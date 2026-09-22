package backup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// BaseBackupHistoryStore is the store surface BaseBackupRunner needs,
// narrowed the same way HistoryStore is for the logical-dump direction
// (runner.go).
type BaseBackupHistoryStore interface {
	TargetGetter
	StartBaseBackupHistory(ctx context.Context, h store.BaseBackupHistory) error
	FinishBaseBackupHistory(ctx context.Context, id, status string, sizeBytes int64, lsn, errMsg, finishedAt string) error
}

// BaseBackupRunner ties a BaseBackuper and an Uploader to store and
// internal/secrets, the physical-backup counterpart of Runner
// (runner.go): the only piece in this package that knows how a base
// backup attempt gets recorded and how a target's live credentials get
// resolved.
type BaseBackupRunner struct {
	Store        BaseBackupHistoryStore
	Secrets      SecretsResolver
	BaseBackuper BaseBackuper
	Uploader     Uploader
	// Runtime, if set, backs StartLSN's best-effort diagnostic lookup
	// (base_backup.go). nil is valid: RunBaseBackup simply records an
	// empty LSN, the same "optional capability" shape this package
	// already uses for Runner.VolumeArchiver.
	Runtime Runtime
	// WorkDir/Now match Runner's own identically-named fields exactly
	// (runner.go's own doc comments for each apply here unchanged).
	WorkDir string
	Now     func() time.Time
}

func (r *BaseBackupRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// RunBaseBackup takes a physical base backup of databaseName (running in
// containerName) and uploads it to targetID, recording the attempt in
// store.BaseBackupHistory throughout: the same running-then-succeeded-
// or-failed lifecycle RunBackup (runner.go) already establishes for a
// logical dump, applied to a physical one. historyID is minted by the
// caller, the same "generate the ID before the first write" convention
// RunBackup's own doc comment describes; expected to run in a goroutine
// the caller has already detached.
func (r *BaseBackupRunner) RunBaseBackup(ctx context.Context, historyID, databaseName, containerName, targetID string) error {
	if err := checkDiskSpace(resolveWorkDir(r.WorkDir)); err != nil {
		return err
	}

	startedAt := r.now()
	objectKey := fmt.Sprintf("%s/base-%s.tar", databaseName, startedAt.UTC().Format("20060102T150405Z"))

	if err := r.Store.StartBaseBackupHistory(ctx, store.BaseBackupHistory{
		ID:           historyID,
		DatabaseName: databaseName,
		TargetID:     targetID,
		ObjectKey:    objectKey,
		StartedAt:    startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start base backup history %q: %w", historyID, err)
	}

	lsn := ""
	if r.Runtime != nil {
		lsn = StartLSN(ctx, r.Runtime, containerName)
	}

	size, runErr := r.runBaseBackupAndUpload(ctx, containerName, targetID, objectKey)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishBaseBackupHistory(ctx, historyID, status, size, lsn, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish base backup history %q: %w", historyID, err)
	}
	return runErr
}

func (r *BaseBackupRunner) runBaseBackupAndUpload(ctx context.Context, containerName, targetID, objectKey string) (size int64, err error) {
	dest, err := resolveTargetDestination(ctx, r.Store, r.Secrets, targetID)
	if err != nil {
		return 0, err
	}

	tar, err := r.BaseBackuper.BaseBackup(ctx, containerName)
	if err != nil {
		return 0, fmt.Errorf("base backup container %q: %w", containerName, err)
	}
	defer func() {
		_ = tar.Close()
	}()

	counted := &countingReader{r: tar, hash: sha256.New()}
	if err := r.Uploader.Upload(ctx, dest, objectKey, counted, -1); err != nil {
		return counted.n, fmt.Errorf("upload base backup for container %q to target %q: %w", containerName, targetID, err)
	}
	return counted.n, nil
}
