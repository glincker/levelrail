package backup

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// PITRHistoryStore is the store surface PITRRunner needs. See ADR 016
// for why UpdateDatabaseSuspended is part of a restore's own store
// surface rather than a separate concern.
type PITRHistoryStore interface {
	TargetGetter
	GetBaseBackupHistory(ctx context.Context, id string) (store.BaseBackupHistory, error)
	GetOldestSucceededBaseBackup(ctx context.Context, databaseName string) (store.BaseBackupHistory, bool, error)
	StartPITRRestoreHistory(ctx context.Context, h store.PITRRestoreHistory) error
	FinishPITRRestoreHistory(ctx context.Context, id, status, errMsg, finishedAt string) error
	UpdateDatabaseSuspended(ctx context.Context, name string, suspended bool) error
}

const (
	defaultPITRTeardownTimeout = 60 * time.Second
	defaultPITRStartupTimeout  = 5 * time.Minute
	defaultPITRPromoteTimeout  = 15 * time.Minute
	defaultPITRPollInterval    = 1 * time.Second
)

// PITRRunner orchestrates one point-in-time restore end to end: download
// the named base backup, hand it to a PITRRestorer to wipe and
// re-populate the database's data volume with recovery configured for
// targetTime, then wait for Postgres to actually promote before calling
// the attempt succeeded. Unlike RestoreRunner, this needs the container
// down for the whole window; see ADR 016 for why Suspended is what
// keeps the reconciler from racing that, and why promotion is polled
// directly rather than trusted to the reconciler's Ready condition.
type PITRRunner struct {
	Store      PITRHistoryStore
	Secrets    SecretsResolver
	Downloader Downloader
	Restorer   PITRRestorer
	// Runtime backs the teardown/startup/promotion polls: Suspended and
	// Nudge only ever request a change, this verifies it happened.
	Runtime docker.Runtime
	// Nudge requests an immediate reconcile pass (*reconcile.Engine's
	// own Nudge); nil is valid, falling back to the resync cadence.
	Nudge func()
	// Now returns the current time for history timestamps; nil falls
	// back to time.Now.
	Now func() time.Time
	// TeardownTimeout/StartupTimeout/PromoteTimeout/PollInterval override
	// the package defaults above when non-zero, for tests.
	TeardownTimeout time.Duration
	StartupTimeout  time.Duration
	PromoteTimeout  time.Duration
	PollInterval    time.Duration
}

func (r *PITRRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *PITRRunner) teardownTimeout() time.Duration {
	if r.TeardownTimeout > 0 {
		return r.TeardownTimeout
	}
	return defaultPITRTeardownTimeout
}

func (r *PITRRunner) startupTimeout() time.Duration {
	if r.StartupTimeout > 0 {
		return r.StartupTimeout
	}
	return defaultPITRStartupTimeout
}

func (r *PITRRunner) promoteTimeout() time.Duration {
	if r.PromoteTimeout > 0 {
		return r.PromoteTimeout
	}
	return defaultPITRPromoteTimeout
}

func (r *PITRRunner) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return defaultPITRPollInterval
}

func (r *PITRRunner) nudge() {
	if r.Nudge != nil {
		r.Nudge()
	}
}

// RunPITRRestore restores databaseName to targetTime from the physical
// base backup baseBackupHistoryID, recording a running-then-succeeded-
// or-failed store.PITRRestoreHistory row. targetTime is assumed already
// validated by the caller (ValidateTarget). Expected to run in a
// goroutine the caller has already detached.
func (r *PITRRunner) RunPITRRestore(ctx context.Context, historyID, databaseName, containerName, dataVolumeName, baseBackupHistoryID string, targetTime time.Time) error {
	startedAt := r.now()

	if err := r.Store.StartPITRRestoreHistory(ctx, store.PITRRestoreHistory{
		ID:                  historyID,
		DatabaseName:        databaseName,
		BaseBackupHistoryID: baseBackupHistoryID,
		TargetTimestamp:     targetTime.UTC().Format(time.RFC3339),
		StartedAt:           startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start pitr restore history %q: %w", historyID, err)
	}

	runErr := r.run(ctx, databaseName, containerName, dataVolumeName, baseBackupHistoryID, targetTime)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishPITRRestoreHistory(ctx, historyID, status, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish pitr restore history %q: %w", historyID, err)
	}
	return runErr
}

// run resolves and downloads the base backup (any failure here never
// touches the live container), then runs the destructive phase.
func (r *PITRRunner) run(ctx context.Context, databaseName, containerName, dataVolumeName, baseBackupHistoryID string, targetTime time.Time) error {
	bh, err := r.Store.GetBaseBackupHistory(ctx, baseBackupHistoryID)
	if err != nil {
		return fmt.Errorf("get base backup history %q: %w", baseBackupHistoryID, err)
	}
	if bh.Status != store.BackupStatusSucceeded {
		return fmt.Errorf("base backup %q has status %q, not %q: refusing to restore from an attempt that did not succeed", baseBackupHistoryID, bh.Status, store.BackupStatusSucceeded)
	}

	dest, err := resolveTargetDestination(ctx, r.Store, r.Secrets, bh.TargetID)
	if err != nil {
		return err
	}

	tar, err := r.Downloader.Download(ctx, dest, bh.ObjectKey)
	if err != nil {
		return fmt.Errorf("download base backup %q object %q: %w", baseBackupHistoryID, bh.ObjectKey, err)
	}
	defer func() {
		_ = tar.Close()
	}()

	return r.destructivePhase(ctx, databaseName, containerName, dataVolumeName, tar, targetTime)
}

// destructivePhase suspends, waits for teardown, restores, then
// unconditionally clears Suspended again (defer): a failed attempt must
// not leave the database suspended forever (ADR 016).
func (r *PITRRunner) destructivePhase(ctx context.Context, databaseName, containerName, dataVolumeName string, tar io.Reader, targetTime time.Time) error {
	if err := r.Store.UpdateDatabaseSuspended(ctx, databaseName, true); err != nil {
		return fmt.Errorf("suspend database %q: %w", databaseName, err)
	}
	r.nudge()
	defer func() {
		if err := r.Store.UpdateDatabaseSuspended(context.Background(), databaseName, false); err != nil {
			return
		}
		r.nudge()
	}()

	if err := r.waitContainerGone(ctx, containerName); err != nil {
		return err
	}

	if err := r.Restorer.Restore(ctx, dataVolumeName, tar, targetTime); err != nil {
		return fmt.Errorf("restore data volume %q: %w", dataVolumeName, err)
	}

	// Clear now, ahead of the deferred clear: the waits below need the
	// container actually coming back up. The deferred clear firing again
	// on return is a harmless, idempotent no-op on top of this one.
	if err := r.Store.UpdateDatabaseSuspended(ctx, databaseName, false); err != nil {
		return fmt.Errorf("unsuspend database %q: %w", databaseName, err)
	}
	r.nudge()

	if err := r.waitContainerRunning(ctx, containerName); err != nil {
		return err
	}
	return r.waitPromoted(ctx, containerName)
}

// waitContainerRunning polls Runtime.InspectByName until containerName
// exists and is running (the reconciler's own fresh-create-and-start
// path, driven by clearing Suspended above) or startupTimeout elapses.
func (r *PITRRunner) waitContainerRunning(ctx context.Context, containerName string) error {
	deadline := time.Now().Add(r.startupTimeout())
	for {
		state, err := r.Runtime.InspectByName(ctx, containerName)
		if err == nil && state != nil && state.Running {
			return nil
		}
		if !time.Now().Before(deadline) {
			if err != nil {
				return fmt.Errorf("container %q did not come back up within %s: %w", containerName, r.startupTimeout(), err)
			}
			return fmt.Errorf("container %q did not come back up within %s", containerName, r.startupTimeout())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.pollInterval()):
		}
	}
}

// waitPromoted polls pg_is_in_recovery() directly until it reports false
// or promoteTimeout elapses; see ADR 016 for why this, not the
// reconciler's Ready condition, is what proves a restore succeeded.
func (r *PITRRunner) waitPromoted(ctx context.Context, containerName string) error {
	deadline := time.Now().Add(r.promoteTimeout())
	for {
		if r.promoted(ctx, containerName) {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("database in container %q did not finish recovery and promote within %s", containerName, r.promoteTimeout())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.pollInterval()):
		}
	}
}

func (r *PITRRunner) promoted(ctx context.Context, containerName string) bool {
	rc, err := r.Runtime.Exec(ctx, containerName, pgIsInRecoveryCmd)
	if err != nil {
		return false
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "f"
}

var pgIsInRecoveryCmd = []string{"sh", "-c", `exec psql --no-password -U "$POSTGRES_USER" -Atq -c "SELECT pg_is_in_recovery()"`}

// Window computes databaseName's current PITRWindow: the oldest
// succeeded base backup plus how far WAL archiving has actually,
// provably reached right now (RecoverableWindowEnd). Returns
// ErrNoBaseBackup without ever reaching containerName when there is no
// succeeded base backup yet.
func (r *PITRRunner) Window(ctx context.Context, databaseName, containerName string) (PITRWindow, error) {
	oldest, ok, err := r.Store.GetOldestSucceededBaseBackup(ctx, databaseName)
	if err != nil {
		return PITRWindow{}, fmt.Errorf("get oldest succeeded base backup for %q: %w", databaseName, err)
	}
	if !ok {
		return PITRWindow{}, ErrNoBaseBackup
	}

	startedAt, err := time.Parse(time.RFC3339, oldest.StartedAt)
	if err != nil {
		return PITRWindow{}, fmt.Errorf("parse base backup %q started_at %q: %w", oldest.ID, oldest.StartedAt, err)
	}

	end, err := RecoverableWindowEnd(ctx, r.Runtime, containerName)
	if err != nil {
		return PITRWindow{}, fmt.Errorf("compute recoverable window end for %q: %w", containerName, err)
	}

	return PITRWindow{HasBaseBackup: true, Start: startedAt, End: end}, nil
}

// WindowBounds is Window with its result destructured into plain
// values, since an interface method's return type must be identical
// across the internal/api package boundary (see pitr.go).
func (r *PITRRunner) WindowBounds(ctx context.Context, databaseName, containerName string) (hasBaseBackup bool, start, end time.Time, err error) {
	win, err := r.Window(ctx, databaseName, containerName)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}
	return win.HasBaseBackup, win.Start, win.End, nil
}

// ValidateTarget checks target falls inside databaseName's current
// window, the synchronous check RunPITRRestore itself never repeats.
func (r *PITRRunner) ValidateTarget(ctx context.Context, databaseName, containerName string, target time.Time) error {
	win, err := r.Window(ctx, databaseName, containerName)
	if err != nil {
		return err
	}
	return ValidatePITRTarget(win, target)
}

// waitContainerGone polls Runtime.InspectByName until containerName no
// longer exists (Controller.Teardown's own effect once Suspended takes
// hold, see PITRRunner's own doc comment) or teardownTimeout elapses.
func (r *PITRRunner) waitContainerGone(ctx context.Context, containerName string) error {
	deadline := time.Now().Add(r.teardownTimeout())
	for {
		state, err := r.Runtime.InspectByName(ctx, containerName)
		if err != nil {
			return fmt.Errorf("inspect %q while waiting for teardown: %w", containerName, err)
		}
		if state == nil {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("container %q was not torn down within %s", containerName, r.teardownTimeout())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(r.pollInterval()):
		}
	}
}
