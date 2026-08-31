package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// VerificationHistoryStore is the store.DB surface VerifyRunner needs:
// GetBackupHistory/GetBackupTarget resolve exactly what
// DownloadRunner.Download's own identical pair already resolve (the
// object a verification attempt re-downloads is the same object a real
// download would fetch), plus Start/FinishBackupVerification to record
// the attempt itself, the same shape RestoreHistoryStore's own doc
// comment describes for the restore direction.
type VerificationHistoryStore interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
	GetBackupTarget(ctx context.Context, id string) (store.BackupTarget, error)
	StartBackupVerification(ctx context.Context, v store.BackupVerification) error
	FinishBackupVerification(ctx context.Context, id, status string, checksumMatch, sizeMatch, formatValid bool, downloadedBytes int64, errMsg, finishedAt string) error
}

// VerifyRunner confirms a previously succeeded backup is still intact
// without ever attempting a live restore: re-download the object, re-hash
// it, and compare against what Runner recorded at backup time, the
// non-destructive counterpart RunRestore's own doc comment explicitly
// rules out for this feature (a live restore against a running system is
// real risk, not a safe thing to run unattended on a schedule or a
// button click).
type VerifyRunner struct {
	Store      VerificationHistoryStore
	Secrets    SecretsResolver
	Downloader Downloader
	// Now returns the current time, matching Runner.Now and
	// RestoreRunner.Now's own "field, not time.Now called directly"
	// reasoning for deterministic tests.
	Now func() time.Time
}

func (r *VerifyRunner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// VerifyBackup re-downloads backupHistoryID's stored object and checks it
// for corruption, recording the attempt in store.BackupVerification
// throughout: a running row before any work starts, then passed or failed
// once it's done. verificationID is minted by the caller
// (internal/api's handleVerifyBackup), the same "generate the ID before
// the first write" convention RunBackup's own doc comment describes;
// VerifyBackup is expected to run in a goroutine the caller has already
// detached, for the same reason RunBackup is.
//
// Refuses (the same as RunRestore) to proceed against a backup whose
// Status is not store.BackupStatusSucceeded: a running or failed attempt
// has no complete object at its recorded key worth verifying.
//
// engine is passed by the caller (internal/api resolves it from the
// database's own store.DesiredDatabase, the same "caller resolves
// engine, this package only consumes it" convention RunBackup's own
// engine parameter already establishes) rather than looked up here: it
// only sharpens the format check below (validateFormat), it is never
// required for the checksum/size checks that carry most of this
// function's real value, so an empty engine (e.g. the parent database
// was since deleted) still runs a meaningful, if slightly less specific,
// verification rather than failing outright.
func (r *VerifyRunner) VerifyBackup(ctx context.Context, verificationID, backupHistoryID, engine, checkedBy string) error {
	startedAt := r.now()

	if err := r.Store.StartBackupVerification(ctx, store.BackupVerification{
		ID:              verificationID,
		BackupHistoryID: backupHistoryID,
		CheckedBy:       checkedBy,
		StartedAt:       startedAt.UTC().Format(time.RFC3339),
	}); err != nil {
		return fmt.Errorf("backup: start verification %q: %w", verificationID, err)
	}

	result, runErr := r.runVerify(ctx, backupHistoryID, engine)

	status := store.BackupVerificationStatusPassed
	errMsg := ""
	switch {
	case runErr != nil:
		status = store.BackupVerificationStatusFailed
		errMsg = runErr.Error()
	case !result.checksumMatch:
		status = store.BackupVerificationStatusFailed
		errMsg = "downloaded checksum does not match the checksum recorded at backup time"
	case !result.sizeMatch:
		status = store.BackupVerificationStatusFailed
		errMsg = "downloaded size does not match the size recorded at backup time"
	case !result.formatValid:
		status = store.BackupVerificationStatusFailed
		errMsg = result.formatError
	}

	finishedAt := r.now().UTC().Format(time.RFC3339)
	if err := r.Store.FinishBackupVerification(ctx, verificationID, status, result.checksumMatch, result.sizeMatch, result.formatValid, result.downloadedBytes, errMsg, finishedAt); err != nil {
		return fmt.Errorf("backup: finish verification %q: %w", verificationID, err)
	}
	if status == store.BackupVerificationStatusFailed {
		return fmt.Errorf("backup verification %q failed: %s", verificationID, errMsg)
	}
	return nil
}

// verifyResult is runVerify's outcome: every individual check it ran,
// independent of one another, so VerifyBackup can report which one
// actually failed rather than a single opaque "verification failed."
type verifyResult struct {
	checksumMatch   bool
	sizeMatch       bool
	formatValid     bool
	formatError     string
	downloadedBytes int64
}

// runVerify is VerifyBackup's actual work, split out the same way
// runDumpAndUpload is split out of RunBackup. Streams the downloaded
// object through a SHA-256 hash and a small header sniffer in one pass
// (io.MultiWriter), never buffering the whole object in memory: a
// database dump can be large, and this function's only job is to confirm
// it is intact, not to hold a second copy of it anywhere.
func (r *VerifyRunner) runVerify(ctx context.Context, backupHistoryID, engine string) (verifyResult, error) {
	h, err := r.Store.GetBackupHistory(ctx, backupHistoryID)
	if err != nil {
		return verifyResult{}, fmt.Errorf("get backup history %q: %w", backupHistoryID, err)
	}
	if h.Status != store.BackupStatusSucceeded {
		return verifyResult{}, fmt.Errorf("backup %q has status %q, not %q: refusing to verify an attempt that did not succeed", backupHistoryID, h.Status, store.BackupStatusSucceeded)
	}

	target, err := r.Store.GetBackupTarget(ctx, h.TargetID)
	if err != nil {
		return verifyResult{}, fmt.Errorf("get backup target %q: %w", h.TargetID, err)
	}

	secretsKey := store.BackupTargetSecretsKey(h.TargetID)
	accessKeyID, err := r.Secrets.Resolve(ctx, secretsKey, "access_key_id")
	if err != nil {
		return verifyResult{}, fmt.Errorf("resolve access key id for target %q: %w", h.TargetID, err)
	}
	secretAccessKey, err := r.Secrets.Resolve(ctx, secretsKey, "secret_access_key")
	if err != nil {
		return verifyResult{}, fmt.Errorf("resolve secret access key for target %q: %w", h.TargetID, err)
	}

	dest := Destination{
		Provider:        target.Provider,
		Endpoint:        target.Endpoint,
		Region:          target.Region,
		Bucket:          target.Bucket,
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
	}

	stream, err := r.Downloader.Download(ctx, dest, h.ObjectKey)
	if err != nil {
		return verifyResult{}, fmt.Errorf("download backup %q object %q: %w", backupHistoryID, h.ObjectKey, err)
	}
	defer func() {
		_ = stream.Close()
	}()

	hasher := sha256.New()
	header := newHeaderCapture(headerCaptureBytes)
	n, err := io.Copy(io.MultiWriter(hasher, header), stream)
	if err != nil {
		return verifyResult{}, fmt.Errorf("read backup %q object %q: %w", backupHistoryID, h.ObjectKey, err)
	}

	checksum := hex.EncodeToString(hasher.Sum(nil))
	formatValid, formatError := validateFormat(engine, header.bytes, n)

	return verifyResult{
		// h.ChecksumSHA256 is empty for a backup taken before
		// migrations/0069_backup_verification.sql added checksum
		// recording: nothing to compare against, so this check cannot
		// meaningfully fail for that row and is reported as a match
		// rather than a false alarm.
		checksumMatch: h.ChecksumSHA256 == "" || checksum == h.ChecksumSHA256,
		// h.SizeBytes of 0 is the equivalent legacy gap for size.
		sizeMatch:       h.SizeBytes == 0 || n == h.SizeBytes,
		formatValid:     formatValid,
		formatError:     formatError,
		downloadedBytes: n,
	}, nil
}

// headerCaptureBytes only needs to cover the longest magic sequence
// validateFormat checks (Redis RDB's "REDIS" plus its 4-digit version,
// 9 bytes): a round number comfortably larger than that.
const headerCaptureBytes = 16

// headerCapture is an io.Writer that retains only the first max bytes
// ever written to it and silently discards the rest, letting it sit
// alongside a hash.Hash in an io.MultiWriter over a stream of unknown,
// potentially large size without growing unbounded itself.
type headerCapture struct {
	bytes    []byte
	maxBytes int
}

func newHeaderCapture(maxBytes int) *headerCapture {
	return &headerCapture{maxBytes: maxBytes}
}

func (c *headerCapture) Write(p []byte) (int, error) {
	if need := c.maxBytes - len(c.bytes); need > 0 {
		if need > len(p) {
			need = len(p)
		}
		c.bytes = append(c.bytes, p[:need]...)
	}
	return len(p), nil
}

// validateFormat runs the one structural check this codebase can make
// honestly cheaply for a given engine, without decompressing or parsing
// the full dump: Redis/KeyDB/Dragonfly all produce an RDB snapshot
// (dump.go's own redisDumpCmd/keydbDumpCmd/dragonflyDumpCmd), and the RDB
// format's magic header, the ASCII bytes "REDIS" followed by a 4-digit
// version, is stable and well documented, safe to check without risking a
// false failure on a genuinely valid file. Every other engine in this
// codebase's dump.go (Postgres/MySQL/MariaDB/ClickHouse's plain SQL text,
// MongoDB's mongodump archive) has no equally simple, reliably-documented
// magic-byte check available, so for those this only confirms the
// downloaded object is non-empty: a real, if smaller, signal, not a
// fabricated one.
func validateFormat(engine string, header []byte, totalBytes int64) (bool, string) {
	if totalBytes == 0 {
		return false, "downloaded object is empty"
	}
	switch engine {
	case store.EngineRedis, store.EngineKeyDB, store.EngineDragonfly:
		if len(header) < 5 || string(header[:5]) != "REDIS" {
			return false, "RDB header magic (\"REDIS\") not found at the start of the downloaded object"
		}
	}
	return true, ""
}
