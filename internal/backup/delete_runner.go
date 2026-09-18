package backup

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DeleteHistoryStore is the store.DB surface DeleteRunner needs:
// GetBackupHistory/GetBackupTarget resolve where a backup attempt's
// object actually lives, the identical pair DownloadHistoryStore's own
// doc comment describes; DeleteBackupHistory removes the row itself once
// the underlying object delete has been attempted.
type DeleteHistoryStore interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	DeleteBackupHistory(ctx context.Context, id string) error
}

// DeleteRunner deletes one backup attempt on demand: an operator-directed
// counterpart to Scheduler's own retention-driven deletePrunedObjects,
// sharing the same "best-effort object delete, the store row is the
// thing that must always go" contract (Scheduler.deletePrunedObjects's
// own doc comment). Unlike that batch path, DeleteBackup always finishes
// by removing the row, regardless of whether the object delete
// succeeded, failed, or couldn't even be attempted: an operator who
// explicitly named this one archive to delete should not be left with it
// still showing up in history because deleting the underlying object hit
// a storage-side problem, on top of Deleter's own contract that a
// missing object is already a no-op success (backup.go's own Deleter
// doc comment).
type DeleteRunner struct {
	Store   DeleteHistoryStore
	Secrets SecretsResolver
	// Deleter may be nil, the same optional-capability shape
	// Scheduler.Deleter's own doc comment establishes: DeleteBackup then
	// only removes the store row, never attempting the object delete.
	Deleter Deleter
	Logger  *slog.Logger
}

func (d *DeleteRunner) log() *slog.Logger {
	if d.Logger != nil {
		return d.Logger
	}
	return slog.Default()
}

// DeleteBackup removes historyID's backup_history row and best-effort
// deletes the stored object behind it first. A target that no longer
// resolves, a secret that fails to resolve, or a Deleter call that itself
// fails is logged and does not stop the row from being deleted: the
// object is already unreachable or already gone either way, and an
// operator asking to delete an archive should not be blocked by a
// storage-side problem with the very thing they're trying to get rid of.
func (d *DeleteRunner) DeleteBackup(ctx context.Context, historyID string) error {
	h, err := d.Store.GetBackupHistory(ctx, historyID)
	if err != nil {
		return fmt.Errorf("get backup history %q: %w", historyID, err)
	}

	if d.Deleter != nil && h.ObjectKey != "" {
		dest, destErr := d.resolveDestination(ctx, h.TargetID)
		switch {
		case destErr != nil:
			d.log().Warn("backup: delete: resolve destination failed, deleting history row without removing the stored object",
				slog.String("backup_id", historyID), slog.String("target_id", h.TargetID), slog.String("error", destErr.Error()))
		default:
			if delErr := d.Deleter.Delete(ctx, dest, h.ObjectKey); delErr != nil {
				d.log().Warn("backup: delete: remove stored object failed, deleting history row anyway",
					slog.String("backup_id", historyID), slog.String("object_key", h.ObjectKey), slog.String("error", delErr.Error()))
			}
		}
	}

	if err := d.Store.DeleteBackupHistory(ctx, historyID); err != nil {
		return fmt.Errorf("delete backup history %q: %w", historyID, err)
	}
	return nil
}

// resolveDestination mirrors DownloadRunner.Download's own identical
// target-plus-secrets resolution (download_runner.go): duplicated rather
// than shared, the same small-independent-struct tradeoff this package's
// other resolvers already make.
func (d *DeleteRunner) resolveDestination(ctx context.Context, targetID string) (Destination, error) {
	target, err := d.Store.GetBackupTarget(ctx, targetID)
	if err != nil {
		return Destination{}, fmt.Errorf("get backup target %q: %w", targetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := d.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve access key id for target %q: %w", targetID, err)
	}
	secretAccessKey, err := d.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve secret access key for target %q: %w", targetID, err)
	}

	return Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}, nil
}
