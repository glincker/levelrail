package backup

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DownloadHistoryStore is the store.DB surface DownloadRunner needs:
// GetBackupHistory to resolve the attempt a download names, GetBackupTarget
// to resolve where its object actually lives, the same pair
// RestoreHistoryStore's own doc comment describes for the restore
// direction. No Start/Finish pair here: unlike a backup or a restore, a
// download has no attempt of its own worth recording in a history table,
// it only reads an object an earlier, already-recorded backup wrote.
type DownloadHistoryStore interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
}

// DownloadRunner resolves a succeeded backup attempt to its live object
// stream, the read-only counterpart of Runner (dump-and-upload) and
// RestoreRunner (download-and-restore-in-place): given a
// store.BackupHistory ID, it resolves the row's target and credentials the
// identical way Runner.runDumpAndUpload and RestoreRunner.runDownloadAndRestore
// already do, then hands the caller (internal/api's handleDownloadBackup)
// the raw object stream to copy directly into an HTTP response, never
// buffering it here.
type DownloadRunner struct {
	Store      DownloadHistoryStore
	Secrets    SecretsResolver
	Downloader Downloader
}

// Download resolves historyID's backup attempt to its object stream.
// Refuses (without ever resolving credentials or contacting the bucket) a
// backup whose Status is not store.BackupStatusSucceeded, the same safety
// property RunRestore's own doc comment describes for the restore path:
// a running attempt has no complete object at its recorded key yet, and a
// failed one may have none, or a partial one. internal/api's
// handleDownloadBackup does the identical check before ever calling this;
// this is the defense-in-depth backstop for a caller that doesn't.
func (d *DownloadRunner) Download(ctx context.Context, historyID string) (io.ReadCloser, error) {
	h, err := d.Store.GetBackupHistory(ctx, historyID)
	if err != nil {
		return nil, fmt.Errorf("get backup history %q: %w", historyID, err)
	}
	if h.Status != store.BackupStatusSucceeded {
		return nil, fmt.Errorf("backup %q has status %q, not %q: refusing to download an attempt that did not succeed", historyID, h.Status, store.BackupStatusSucceeded)
	}

	target, err := d.Store.GetBackupTarget(ctx, h.TargetID)
	if err != nil {
		return nil, fmt.Errorf("get backup target %q: %w", h.TargetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(h.TargetID)
	accessKeyID, err := d.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return nil, fmt.Errorf("resolve access key id for target %q: %w", h.TargetID, err)
	}
	secretAccessKey, err := d.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return nil, fmt.Errorf("resolve secret access key for target %q: %w", h.TargetID, err)
	}

	dest := Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	stream, err := d.Downloader.Download(ctx, dest, h.ObjectKey)
	if err != nil {
		return nil, fmt.Errorf("download backup %q object %q: %w", historyID, h.ObjectKey, err)
	}
	return stream, nil
}
