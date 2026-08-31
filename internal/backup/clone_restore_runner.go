package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultCloneRestoreReadyTimeout bounds how long RunCloneRestore waits
// for a freshly created database's own reconciler to report it Ready
// before giving up: long enough for a cold image pull on a slow link,
// short enough that a genuinely stuck database (the credentials gap
// databaseControllerName's own doc comment describes, for example) fails
// the attempt rather than hanging the goroutine forever.
const defaultCloneRestoreReadyTimeout = 10 * time.Minute

// defaultCloneRestoreReadyPollInterval is how often RunCloneRestore
// re-checks the new database's reported conditions while waiting.
const defaultCloneRestoreReadyPollInterval = 2 * time.Second

// CloneRestoreStore is the narrow store surface CloneRestoreRunner needs:
// backupResolver's two methods to resolve the source backup, the clone-
// restore bookkeeping pair, and GetConditions to detect when the newly
// created database has actually come up. A separate interface from
// RestoreHistoryStore, the same "narrow interface per real dependency"
// reasoning that type's own doc comment gives, since the two share no
// method beyond the backupResolver pair.
type CloneRestoreStore interface {
	backupResolver
	StartCloneRestore(ctx context.Context, h store.CloneRestore) error
	FinishCloneRestore(ctx context.Context, id, status, errMsg, finishedAt string) error
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
}

// CloneRestoreRunner restores a previously succeeded backup into a
// brand-new database rather than overwriting the one it came from: the
// non-destructive counterpart to RestoreRunner. internal/api creates the
// new database's desired state itself (reusing handleCreateDatabase's own
// path, see database_clone_restore.go's own doc comment for why) before
// ever calling RunCloneRestore; this type's only job is to wait for that
// database's reconciler to bring it up and then run the identical
// download-and-apply RestoreRunner already implements.
type CloneRestoreRunner struct {
	Store      CloneRestoreStore
	Secrets    SecretsResolver
	Downloader Downloader
	Restorer   Restorer
	// Now returns the current time, the same "field, not time.Now called
	// directly" reasoning RestoreRunner.Now's own doc comment gives.
	Now func() time.Time
	// ReadyTimeout/PollInterval override defaultCloneRestoreReadyTimeout/
	// defaultCloneRestoreReadyPollInterval when non-zero, so tests don't
	// have to wait out the real default.
	ReadyTimeout time.Duration
	PollInterval time.Duration
}

func (r *CloneRestoreRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *CloneRestoreRunner) readyTimeout() time.Duration {
	if r.ReadyTimeout > 0 {
		return r.ReadyTimeout
	}
	return defaultCloneRestoreReadyTimeout
}

func (r *CloneRestoreRunner) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return defaultCloneRestoreReadyPollInterval
}

// RunCloneRestore waits for newDatabaseName (identified to the reconciler
// by controllerName, "database/" + newDatabaseName) to report Ready, then
// downloads and restores backupHistoryID into it, recording the attempt
// as store.CloneRestore throughout: a running row before any work starts,
// then succeeded or failed once it's done. sourceDatabaseName is recorded
// for history only; RunCloneRestore never reads from or writes to it.
// Expected to run in a goroutine the caller has already detached, the same
// convention RestoreRunner.RunRestore's own doc comment describes.
func (r *CloneRestoreRunner) RunCloneRestore(ctx context.Context, historyID, sourceDatabaseName, newDatabaseName, backupHistoryID, engine, containerName, controllerName string) error {
	startedAt := r.now()

	if err := r.Store.StartCloneRestore(ctx, store.CloneRestore{
		ID:                 historyID,
		SourceDatabaseName: sourceDatabaseName,
		NewDatabaseName:    newDatabaseName,
		BackupHistoryID:    backupHistoryID,
		StartedAt:          startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start clone restore %q: %w", historyID, err)
	}

	runErr := r.waitAndRestore(ctx, controllerName, newDatabaseName, backupHistoryID, engine, containerName)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishCloneRestore(ctx, historyID, status, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish clone restore %q: %w", historyID, err)
	}
	return runErr
}

// waitAndRestore is RunCloneRestore's actual work, the same "keep
// RunX's own body a flat start/run/finish sequence" split RunRestore's
// runDownloadAndRestore already establishes.
func (r *CloneRestoreRunner) waitAndRestore(ctx context.Context, controllerName, newDatabaseName, backupHistoryID, engine, containerName string) error {
	if err := r.waitUntilReady(ctx, controllerName); err != nil {
		return fmt.Errorf("wait for database %q to become ready: %w", newDatabaseName, err)
	}
	return downloadAndRestore(ctx, r.Store, r.Secrets, r.Downloader, r.Restorer, newDatabaseName, backupHistoryID, engine, containerName)
}

// waitUntilReady polls controllerName's stored reconcile conditions until
// a "Ready"/ConditionTrue condition appears or readyTimeout elapses,
// mirroring the deadline-loop shape test/e2e's own getBodyWithRetry
// helper uses for an analogous "something is still coming up
// asynchronously" wait, at control-plane scale instead of test scale: a
// freshly created database goes through image pull, container create,
// and start before its own controller ever reports Ready (controller.go's
// ready() result), and none of that happens synchronously inside the
// SaveDesiredDatabase call that created it.
func (r *CloneRestoreRunner) waitUntilReady(ctx context.Context, controllerName string) error {
	// Real wall-clock time, deliberately not r.now(): that field exists
	// so StartCloneRestore/FinishCloneRestore record a deterministic,
	// test-fixed timestamp, not so this wait loop's own deadline can be
	// frozen; a test exercising the timeout path sets a small
	// ReadyTimeout/PollInterval instead and lets a few real milliseconds
	// pass, the same way test/e2e's own getBodyWithRetry helper does.
	deadline := time.Now().Add(r.readyTimeout())
	interval := r.pollInterval()

	for {
		conditions, err := r.Store.GetConditions(ctx, controllerName)
		if err != nil {
			return fmt.Errorf("get conditions for %q: %w", controllerName, err)
		}
		for _, c := range conditions {
			if c.Type == "Ready" && c.Status == reconcile.ConditionTrue {
				return nil
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("database %q did not become ready within %s", controllerName, r.readyTimeout())
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
