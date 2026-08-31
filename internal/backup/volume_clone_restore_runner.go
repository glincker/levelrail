package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// VolumeCloneRestoreStore is the narrow store surface
// VolumeCloneRestoreRunner needs: backupResolver's two methods to resolve
// the source backup, plus the clone-restore bookkeeping pair, the app
// service volume counterpart of CloneRestoreStore.
type VolumeCloneRestoreStore interface {
	backupResolver
	StartVolumeCloneRestore(ctx context.Context, h store.VolumeCloneRestore) error
	FinishVolumeCloneRestore(ctx context.Context, id, status, errMsg, finishedAt string) error
}

// VolumeCreator is the narrow docker.Runtime surface
// VolumeCloneRestoreRunner needs to bring a brand-new, empty volume into
// existence before restoring into it: unlike CloneRestoreRunner's wait for
// a new database's own reconciler to report Ready, a bare Docker volume
// has no reconcile loop and exists the moment this call returns, so there
// is nothing to poll for here.
type VolumeCreator interface {
	EnsureVolume(ctx context.Context, name string) error
}

// VolumeCloneRestoreRunner restores a previously succeeded volume backup
// into a brand-new, standalone Docker volume rather than overwriting the
// one it came from: the volume counterpart of CloneRestoreRunner. Simpler
// than CloneRestoreRunner because there is no reconciler to wait on (see
// VolumeCreator's own doc comment); creating the volume and restoring into
// it both happen synchronously, one after the other, inside
// RunVolumeCloneRestore itself.
type VolumeCloneRestoreRunner struct {
	Store          VolumeCloneRestoreStore
	Secrets        SecretsResolver
	Downloader     Downloader
	VolumeRestorer VolumeRestorer
	Volumes        VolumeCreator
	// Now returns the current time, the same "field, not time.Now called
	// directly" reasoning CloneRestoreRunner.Now's own doc comment gives.
	Now func() time.Time
}

func (r *VolumeCloneRestoreRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// RunVolumeCloneRestore creates newVolumeName as a brand-new, empty Docker
// volume, then downloads and restores backupHistoryID into it, recording
// the attempt as store.VolumeCloneRestore throughout: a running row before
// any work starts, then succeeded or failed once it's done.
// sourceServiceName/sourceVolumeName are recorded for history only;
// RunVolumeCloneRestore never reads from or writes to the volume they
// name. Expected to run in a goroutine the caller has already detached,
// the same convention RunCloneRestore's own doc comment describes.
func (r *VolumeCloneRestoreRunner) RunVolumeCloneRestore(ctx context.Context, historyID, sourceServiceName, sourceVolumeName, newVolumeName, backupHistoryID string) error {
	startedAt := r.now()

	if err := r.Store.StartVolumeCloneRestore(ctx, store.VolumeCloneRestore{
		ID:                historyID,
		SourceServiceName: sourceServiceName,
		SourceVolumeName:  sourceVolumeName,
		NewVolumeName:     newVolumeName,
		BackupHistoryID:   backupHistoryID,
		StartedAt:         startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start volume clone restore %q: %w", historyID, err)
	}

	runErr := r.createAndRestore(ctx, newVolumeName, backupHistoryID)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishVolumeCloneRestore(ctx, historyID, status, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish volume clone restore %q: %w", historyID, err)
	}
	return runErr
}

// createAndRestore is RunVolumeCloneRestore's actual work, the same
// "keep RunX's own body a flat start/run/finish sequence" split
// CloneRestoreRunner.waitAndRestore's own doc comment describes.
func (r *VolumeCloneRestoreRunner) createAndRestore(ctx context.Context, newVolumeName, backupHistoryID string) error {
	if err := r.Volumes.EnsureVolume(ctx, newVolumeName); err != nil {
		return fmt.Errorf("create volume %q: %w", newVolumeName, err)
	}
	return downloadAndRestoreVolume(ctx, r.Store, r.Secrets, r.Downloader, r.VolumeRestorer, newVolumeName, backupHistoryID)
}
