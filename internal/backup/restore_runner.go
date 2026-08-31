package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// RestoreHistoryStore is the store.DB surface RestoreRunner needs,
// narrowed the same way Runner's own HistoryStore is: GetBackupHistory to
// resolve the source backup a restore names, GetBackupTarget to resolve
// where that backup's object actually lives, and
// StartRestoreHistory/FinishRestoreHistory to record the restore attempt
// itself. Deliberately not HistoryStore plus new methods bolted on: a
// fake satisfying HistoryStore's forward-backup methods has no reason to
// also implement restore's, and the reverse is equally true, so keeping
// them as two separate narrow interfaces (which happen to overlap on
// GetBackupTarget, satisfied by the same *store.DB either way) matches
// this codebase's own "narrow interface per real dependency, not one
// grab-bag interface per package" convention.
type RestoreHistoryStore interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	StartRestoreHistory(ctx context.Context, h store.RestoreHistory) error
	FinishRestoreHistory(ctx context.Context, id, status, errMsg, finishedAt string) error
}

// RestoreRunner ties a Downloader and a Restorer to store and
// internal/secrets, the restore counterpart of Runner: the only piece in
// this package that knows how a restore attempt gets recorded and how a
// target's live credentials get resolved. internal/api constructs one
// real RestoreRunner at startup, the same place it constructs Runner, and
// calls RunRestore per triggered restore.
//
// A separate type from Runner rather than a second method on it: Runner
// is already a coherent "dump this, upload it" unit with its own field
// set (Dumper, Uploader); RestoreRunner's fields (Downloader, Restorer)
// point the opposite direction and share nothing with Runner's except
// Store's underlying *store.DB value and Secrets' underlying
// *secrets.Manager value, both already expressed as separate narrow
// interfaces above and in runner.go rather than a shared struct field,
// so there is no real duplication to unify by merging the two types, only
// two independently-testable units that happen to be wired from the same
// concrete dependencies at startup.
type RestoreRunner struct {
	Store      RestoreHistoryStore
	Secrets    SecretsResolver
	Downloader Downloader
	Restorer   Restorer
	// VolumeRestorer backs RunVolumeRestore the same way Restorer backs
	// RunRestore. nil is valid, the same "optional capability" shape
	// Runner.VolumeArchiver's own doc comment establishes.
	VolumeRestorer VolumeRestorer
	// Now returns the current time, matching Runner.Now's own "field, not
	// time.Now called directly" reasoning for deterministic tests.
	Now func() time.Time
}

func (r *RestoreRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// RunRestore downloads the object recorded by backupHistoryID and
// applies it to containerName (running engine) as databaseName's new
// contents, recording the attempt in store.RestoreHistory throughout: a
// running row before any work starts, then succeeded or failed once it's
// done. historyID is minted by the caller (internal/api), the same
// "generate the ID before the first write" convention RunBackup's own
// doc comment describes; RunRestore is expected to run in a goroutine
// the caller has already detached, for the same reason RunBackup is, see
// internal/api's handleTriggerRestore.
//
// RunRestore refuses to proceed, failing the history row rather than
// attempting anything, if the named backup's own Status is not
// store.BackupStatusSucceeded: restoring from a backup that was still
// running (and so has no complete, consistent object at its recorded key
// yet) or that already failed (and so may have no object there at all,
// or a partial one) is not a smaller version of a real restore, it is a
// data-loss-shaped mistake this function refuses to make even if a
// caller's own validation (internal/api's handleTriggerRestore does the
// identical check before ever calling this, so a caller going through
// the ordinary API path never reaches this branch) somehow let it
// through.
func (r *RestoreRunner) RunRestore(ctx context.Context, historyID, databaseName, backupHistoryID, engine, containerName string) error {
	startedAt := r.now()

	if err := r.Store.StartRestoreHistory(ctx, store.RestoreHistory{
		ID:              historyID,
		DatabaseName:    databaseName,
		BackupHistoryID: backupHistoryID,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start restore history %q: %w", historyID, err)
	}

	runErr := r.runDownloadAndRestore(ctx, databaseName, backupHistoryID, engine, containerName)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishRestoreHistory(ctx, historyID, status, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish restore history %q: %w", historyID, err)
	}
	return runErr
}

// runDownloadAndRestore is RunRestore's actual work, split out the same
// way runDumpAndUpload is split out of RunBackup: keeps RunRestore's own
// body a flat "start, run, finish" sequence.
func (r *RestoreRunner) runDownloadAndRestore(ctx context.Context, databaseName, backupHistoryID, engine, containerName string) error {
	return downloadAndRestore(ctx, r.Store, r.Secrets, r.Downloader, r.Restorer, databaseName, backupHistoryID, engine, containerName)
}

// backupResolver is the subset of RestoreHistoryStore downloadAndRestore
// needs to resolve a backup attempt down to a downloadable object:
// narrower than the full interface so CloneRestoreRunner (clone_restore_
// runner.go) can share this function without also implementing the
// restore-history bookkeeping methods it has no use for.
type backupResolver interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
}

// downloadAndRestore resolves backupHistoryID down to a live object via
// resolver and secrets, downloads it, and restores it into containerName,
// the shared work behind both RestoreRunner.runDownloadAndRestore (an
// in-place restore) and CloneRestoreRunner.RunCloneRestore (restore into a
// freshly created database): the download-and-apply mechanics are
// identical either way, only what happens before and after differ.
func downloadAndRestore(ctx context.Context, resolver backupResolver, secrets SecretsResolver, downloader Downloader, restorer Restorer, databaseName, backupHistoryID, engine, containerName string) error {
	bh, err := resolver.GetBackupHistory(ctx, backupHistoryID)
	if err != nil {
		return fmt.Errorf("get backup history %q: %w", backupHistoryID, err)
	}
	if bh.Status != store.BackupStatusSucceeded {
		return fmt.Errorf("backup %q has status %q, not %q: refusing to restore from an attempt that did not succeed", backupHistoryID, bh.Status, store.BackupStatusSucceeded)
	}

	target, err := resolver.GetBackupTarget(ctx, bh.TargetID)
	if err != nil {
		return fmt.Errorf("get backup target %q: %w", bh.TargetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(bh.TargetID)
	accessKeyID, err := secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return fmt.Errorf("resolve access key id for target %q: %w", bh.TargetID, err)
	}
	secretAccessKey, err := secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return fmt.Errorf("resolve secret access key for target %q: %w", bh.TargetID, err)
	}

	dest := Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	dump, err := downloader.Download(ctx, dest, bh.ObjectKey)
	if err != nil {
		return fmt.Errorf("download backup %q object %q: %w", backupHistoryID, bh.ObjectKey, err)
	}
	defer func() {
		_ = dump.Close()
	}()

	if err := restorer.Restore(ctx, engine, containerName, dump); err != nil {
		return fmt.Errorf("restore database %q from backup %q: %w", databaseName, backupHistoryID, err)
	}
	return nil
}

// RunVolumeRestore downloads the object recorded by backupHistoryID and
// applies it to dockerVolumeName (serviceName's Docker volume backing
// its volumeName-named app.yaml volume) as its new, complete contents,
// recording the attempt in store.RestoreHistory exactly the way
// RunRestore does for a database. See RunRestore's own doc comment for
// the full reasoning this mirrors, including the identical refusal to
// proceed against a backup whose Status isn't store.BackupStatusSucceeded.
func (r *RestoreRunner) RunVolumeRestore(ctx context.Context, historyID, serviceName, volumeName, dockerVolumeName, backupHistoryID string) error {
	startedAt := r.now()

	if err := r.Store.StartRestoreHistory(ctx, store.RestoreHistory{
		ID:              historyID,
		ResourceKind:    store.BackupResourceKindVolume,
		ServiceName:     serviceName,
		VolumeName:      volumeName,
		BackupHistoryID: backupHistoryID,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start restore history %q: %w", historyID, err)
	}

	runErr := r.runDownloadAndRestoreVolume(ctx, dockerVolumeName, backupHistoryID)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishRestoreHistory(ctx, historyID, status, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish restore history %q: %w", historyID, err)
	}
	return runErr
}

// runDownloadAndRestoreVolume is RunVolumeRestore's actual work, the
// volume counterpart of runDownloadAndRestore: identical resolution of
// the source backup and its destination credentials, VolumeRestorer.
// Restore standing in for Restorer.Restore.
func (r *RestoreRunner) runDownloadAndRestoreVolume(ctx context.Context, dockerVolumeName, backupHistoryID string) error {
	bh, err := r.Store.GetBackupHistory(ctx, backupHistoryID)
	if err != nil {
		return fmt.Errorf("get backup history %q: %w", backupHistoryID, err)
	}
	if bh.Status != store.BackupStatusSucceeded {
		return fmt.Errorf("backup %q has status %q, not %q: refusing to restore from an attempt that did not succeed", backupHistoryID, bh.Status, store.BackupStatusSucceeded)
	}

	target, err := r.Store.GetBackupTarget(ctx, bh.TargetID)
	if err != nil {
		return fmt.Errorf("get backup target %q: %w", bh.TargetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(bh.TargetID)
	accessKeyID, err := r.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return fmt.Errorf("resolve access key id for target %q: %w", bh.TargetID, err)
	}
	secretAccessKey, err := r.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return fmt.Errorf("resolve secret access key for target %q: %w", bh.TargetID, err)
	}

	dest := Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	archive, err := r.Downloader.Download(ctx, dest, bh.ObjectKey)
	if err != nil {
		return fmt.Errorf("download backup %q object %q: %w", backupHistoryID, bh.ObjectKey, err)
	}
	defer func() {
		_ = archive.Close()
	}()

	if err := r.VolumeRestorer.Restore(ctx, dockerVolumeName, archive); err != nil {
		return fmt.Errorf("restore volume %q from backup %q: %w", dockerVolumeName, backupHistoryID, err)
	}
	return nil
}
