package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// SecretsResolver is the one internal/secrets.Manager method Runner
// needs, narrowed to keep this package's tests from needing a real
// Manager (Manager itself has no interface today; a struct-shaped fake
// satisfying this one-method interface is enough).
type SecretsResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// HistoryStore is the store.DB surface Runner needs, narrowed the same
// way SecretsResolver is: a fake satisfying six methods is a lot less
// to stub than the full *store.DB.
type HistoryStore interface {
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	StartBackupHistory(ctx context.Context, h store.BackupHistory) error
	FinishBackupHistory(ctx context.Context, id, status string, sizeBytes int64, checksum, errMsg, finishedAt string) error
}

// Runner ties a Dumper and an Uploader to store and internal/secrets:
// the only piece in this package that knows how a backup attempt gets
// recorded and how a target's live credentials get resolved. internal/api
// constructs one real Runner at startup (the same "wire concrete
// implementations once, in cmd/levelrail or internal/api's own
// constructor" pattern every other cross-package dependency in this
// codebase already follows) and calls RunBackup per triggered backup.
type Runner struct {
	Store    HistoryStore
	Secrets  SecretsResolver
	Dumper   Dumper
	Uploader Uploader
	// VolumeArchiver backs RunVolumeBackup the same way Dumper backs
	// RunBackup. nil is valid: a control plane with no volume-backup
	// callers configured (today, none do this by default) simply never
	// calls RunVolumeBackup, the same "optional capability" shape this
	// codebase already uses for Scheduler.Deleter/Verifier.
	VolumeArchiver VolumeArchiver
	// Now returns the current time. A field, not time.Now called
	// directly, so tests get deterministic timestamps without a real
	// clock dependency; production code leaves it nil and RunBackup
	// falls back to time.Now.
	Now func() time.Time
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// RunBackup dumps databaseName (running in containerName under engine)
// and uploads the result to targetID, recording the attempt in
// store.BackupHistory throughout: a running row before any work starts,
// then succeeded or failed once it's done. historyID is minted by the
// caller (internal/api, the same "generate the ID before the first
// write" convention store.BackupTarget's own doc comment describes) so
// the caller can return it in an HTTP response immediately without
// waiting for the backup to finish; RunBackup itself is expected to run
// in a goroutine the caller has already detached; see internal/api's
// handleTriggerBackup for how it starts, and this file's own tests for
// the exact happy-path and every-failure-point sequencing this describes.
//
// Every failure path (target not found, secret resolution, dump, upload)
// finishes the history row as BackupStatusFailed with a real error
// message rather than leaving it stuck at BackupStatusRunning: the one
// exception is a failure to even write FinishBackupHistory itself, which
// RunBackup can't recover from and returns as-is, the same "give up
// honestly rather than mask it" the reconcilers elsewhere in this
// codebase already follow when their own condition-write fails.
func (r *Runner) RunBackup(ctx context.Context, historyID, databaseName, engine, containerName, targetID string) error {
	startedAt := r.now()
	objectKey := fmt.Sprintf("%s/%s-%s.dump", databaseName, databaseName, startedAt.UTC().Format("20060102T150405Z"))

	if err := r.Store.StartBackupHistory(ctx, store.BackupHistory{
		ID:           historyID,
		DatabaseName: databaseName,
		TargetID:     targetID,
		ObjectKey:    objectKey,
		StartedAt:    startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start history %q: %w", historyID, err)
	}

	size, checksum, runErr := r.runDumpAndUpload(ctx, databaseName, engine, containerName, targetID, objectKey)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishBackupHistory(ctx, historyID, status, size, checksum, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish history %q: %w", historyID, err)
	}
	return runErr
}

// runDumpAndUpload is RunBackup's actual work, split out so RunBackup's
// own body stays a flat "start, run, finish" sequence instead of nesting
// three levels of error handling around the history bookkeeping above
// and below it. The returned checksum is the dump's SHA-256, hex-encoded,
// computed while it streams through to the uploader rather than in a
// second pass over the object: countingReader already sits between the
// dump and the uploader to track size, so hashing there too costs nothing
// extra in I/O, only a running hash.Hash.
func (r *Runner) runDumpAndUpload(ctx context.Context, databaseName, engine, containerName, targetID, objectKey string) (size int64, checksum string, err error) {
	dest, err := r.ResolveDestination(ctx, targetID)
	if err != nil {
		return 0, "", err
	}

	dump, err := r.Dumper.Dump(ctx, engine, containerName)
	if err != nil {
		return 0, "", fmt.Errorf("dump database %q: %w", databaseName, err)
	}
	defer func() {
		_ = dump.Close()
	}()

	counted := &countingReader{r: dump, hash: sha256.New()}
	if err := r.Uploader.Upload(ctx, dest, objectKey, counted, -1); err != nil {
		return counted.n, "", fmt.Errorf("upload backup for %q to target %q: %w", databaseName, targetID, err)
	}
	return counted.n, hex.EncodeToString(counted.hash.Sum(nil)), nil
}

// RunVolumeBackup archives dockerVolumeName (serviceName's Docker volume
// backing its volumeName-named app.yaml volume) and uploads the result
// to targetID, recording the attempt in store.BackupHistory exactly the
// way RunBackup does for a database: the same running-then-succeeded-or-
// failed row, the same historyID-minted-by-the-caller contract, the same
// expectation of running in a goroutine the caller has already detached.
// See RunBackup's own doc comment for the reasoning this mirrors in
// full; this only differs in what it dumps (VolumeArchiver.Archive
// instead of Dumper.Dump) and what identifies the row
// (ServiceName/VolumeName/ResourceKind instead of DatabaseName).
func (r *Runner) RunVolumeBackup(ctx context.Context, historyID, serviceName, volumeName, dockerVolumeName, targetID string) error {
	startedAt := r.now()
	objectKey := fmt.Sprintf("volumes/%s/%s/%s.tar", serviceName, volumeName, startedAt.UTC().Format("20060102T150405Z"))

	if err := r.Store.StartBackupHistory(ctx, store.BackupHistory{
		ID:           historyID,
		ResourceKind: store.BackupResourceKindVolume,
		ServiceName:  serviceName,
		VolumeName:   volumeName,
		TargetID:     targetID,
		ObjectKey:    objectKey,
		StartedAt:    startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start history %q: %w", historyID, err)
	}

	size, checksum, runErr := r.runArchiveAndUpload(ctx, dockerVolumeName, targetID, objectKey)

	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
	}
	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishBackupHistory(ctx, historyID, status, size, checksum, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish history %q: %w", historyID, err)
	}
	return runErr
}

// runArchiveAndUpload is RunVolumeBackup's actual work, the volume
// counterpart of runDumpAndUpload: identical shape, VolumeArchiver.
// Archive standing in for Dumper.Dump.
func (r *Runner) runArchiveAndUpload(ctx context.Context, dockerVolumeName, targetID, objectKey string) (size int64, checksum string, err error) {
	dest, err := r.ResolveDestination(ctx, targetID)
	if err != nil {
		return 0, "", err
	}

	archive, err := r.VolumeArchiver.Archive(ctx, dockerVolumeName)
	if err != nil {
		return 0, "", fmt.Errorf("archive volume %q: %w", dockerVolumeName, err)
	}
	defer func() {
		_ = archive.Close()
	}()

	counted := &countingReader{r: archive, hash: sha256.New()}
	if err := r.Uploader.Upload(ctx, dest, objectKey, counted, -1); err != nil {
		return counted.n, "", fmt.Errorf("upload volume backup for %q to target %q: %w", dockerVolumeName, targetID, err)
	}
	return counted.n, hex.EncodeToString(counted.hash.Sum(nil)), nil
}

// ResolveDestination resolves targetID's stored config and live secrets
// into a Destination, the same lookup runDumpAndUpload does before every
// Upload call. Exported so Scheduler can resolve the identical
// Destination for a Deleter when pruning old backups for the same
// target.
func (r *Runner) ResolveDestination(ctx context.Context, targetID string) (Destination, error) {
	target, err := r.Store.GetBackupTarget(ctx, targetID)
	if err != nil {
		return Destination{}, fmt.Errorf("get backup target %q: %w", targetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(targetID)
	accessKeyID, err := r.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return Destination{}, fmt.Errorf("resolve access key id for target %q: %w", targetID, err)
	}
	secretAccessKey, err := r.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
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

// countingReader wraps a Dumper's stream so RunBackup can report
// SizeBytes on both success and a mid-upload failure (the size that
// actually left the dump, not a size the Uploader happens to report,
// which not every S3-compatible implementation reliably surfaces on
// error) without the Dumper or Uploader needing to know about
// store.BackupHistory at all. hash accumulates the same bytes, giving
// RunBackup a checksum alongside the size for free once the Uploader has
// read the stream to completion.
type countingReader struct {
	r    io.Reader
	hash hash.Hash
	n    int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		c.hash.Write(p[:n]) //nolint:errcheck // hash.Hash.Write never returns an error, per its own documented contract
	}
	return n, err
}
