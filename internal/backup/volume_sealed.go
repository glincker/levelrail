package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrBackupTooLarge means a volume archive exceeded the configured size budget.
var ErrBackupTooLarge = errors.New("volume archive exceeds the backup size budget")

// SealRecorder persists the codec and manifest of a sealed backup.
type SealRecorder interface {
	SetBackupSealInfo(ctx context.Context, id, codec, manifestDigest string, files, bytes int64) error
}

// VolumePolicyLookup reads a volume's backup policy.
type VolumePolicyLookup interface {
	GetVolumeBackupPolicy(ctx context.Context, serviceName, volumeName string) (store.VolumeBackupPolicy, error)
}

// Quiescer makes a volume consistent for the duration of its archive. It runs
// the policy's pre-hook and optionally pauses the app. The returned end
// function undoes the pause and runs the post-hook.
type Quiescer interface {
	Begin(ctx context.Context, serviceName string, p store.VolumeBackupPolicy) (end func(context.Context) error, err error)
}

// VolumeBackupOptions configures sealed volume backups on a Runner.
type VolumeBackupOptions struct {
	Sealer   *Sealer
	Policies VolumePolicyLookup
	Quiesce  Quiescer
	// MaxBytes aborts an archive larger than this many raw bytes; 0 is no limit.
	MaxBytes int64
	// Timeout bounds one whole backup; 0 is no limit.
	Timeout time.Duration
	// Attempts is how many times a failed archive-and-upload is retried
	// from a fresh archive; values below 1 mean one attempt.
	Attempts int
	Logger   *slog.Logger
}

// OptionsFromEnv fills the budget fields from the environment.
func (o *VolumeBackupOptions) OptionsFromEnv(lookup func(string) (string, bool)) {
	if v, ok := lookup(EnvBackupMaxSizeMB); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
			o.MaxBytes = n << 20
		}
	}
	if v, ok := lookup(EnvBackupTimeoutMin); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			o.Timeout = time.Duration(n) * time.Minute
		}
	}
	if v, ok := lookup(EnvBackupUploadTries); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			o.Attempts = n
		}
	}
}

type sealedResult struct {
	codec    string
	manifest TreeManifest
	hasMan   bool
}

func sealedObjectKey(serviceName, volumeName string, at time.Time, historyID, ext, codec string) string {
	id := historyID
	if len(id) > 12 {
		id = id[:12]
	}
	return fmt.Sprintf("volumes/%s/%s/%s-%s.%s%s", serviceName, volumeName, at.UTC().Format("20060102T150405Z"), id, ext, CodecSuffix(codec))
}

func (r *Runner) recordSeal(ctx context.Context, historyID string, s sealedResult) error {
	rec, ok := r.Store.(SealRecorder)
	if !ok {
		return errors.New("history store cannot record the backup codec, so this backup could not be restored")
	}
	if err := rec.SetBackupSealInfo(ctx, historyID, s.codec, s.manifest.Digest, s.manifest.Files, s.manifest.Bytes); err != nil {
		return fmt.Errorf("record backup codec: %w", err)
	}
	return nil
}

// runSealedVolume archives (or snapshots) a volume, compresses and encrypts
// the stream, uploads it, and returns the stored object's size and SHA-256
// plus the content manifest of the raw archive. A failed attempt is retried
// from a fresh archive: a live volume cannot be re-read from an offset, so a
// half-written object is aborted and rebuilt rather than resumed.
func (r *Runner) runSealedVolume(ctx context.Context, serviceName, volumeName, dockerVolumeName, sqlitePath, targetID, objectKey string) (int64, string, sealedResult, error) {
	opts := r.Volume
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	dest, err := r.ResolveDestination(ctx, targetID)
	if err != nil {
		return 0, "", sealedResult{}, err
	}

	policy := store.VolumeBackupPolicy{}
	if opts.Policies != nil {
		p, perr := opts.Policies.GetVolumeBackupPolicy(ctx, serviceName, volumeName)
		if perr != nil && !errors.Is(perr, store.ErrVolumeBackupPolicyNotFound) {
			return 0, "", sealedResult{}, fmt.Errorf("load volume backup policy: %w", perr)
		}
		policy = p
	}

	attempts := opts.Attempts
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 1; i <= attempts; i++ {
		size, sum, res, err := r.sealedAttempt(ctx, serviceName, dockerVolumeName, sqlitePath, dest, objectKey, policy)
		if err == nil {
			return size, sum, res, nil
		}
		lastErr = err
		if errors.Is(err, ErrBackupTooLarge) || ctx.Err() != nil || i == attempts {
			return size, "", res, err
		}
		if opts.Logger != nil {
			opts.Logger.Warn("backup: volume upload attempt failed, retrying from a fresh archive",
				slog.String("service", serviceName), slog.String("volume", volumeName),
				slog.Int("attempt", i), slog.String("error", err.Error()))
		}
	}
	return 0, "", sealedResult{}, lastErr
}

func (r *Runner) sealedAttempt(ctx context.Context, serviceName, dockerVolumeName, sqlitePath string, dest Destination, objectKey string, policy store.VolumeBackupPolicy) (size int64, checksum string, res sealedResult, err error) {
	opts := r.Volume
	res.codec = opts.Sealer.Codec()

	if opts.Quiesce != nil && (policy.PreHook != "" || policy.PostHook != "" || policy.Quiesce != "") {
		end, qerr := opts.Quiesce.Begin(ctx, serviceName, policy)
		if qerr != nil {
			return 0, "", res, fmt.Errorf("prepare volume for backup: %w", qerr)
		}
		defer func() {
			if eerr := end(context.WithoutCancel(ctx)); eerr != nil && err == nil {
				err = fmt.Errorf("release volume after backup: %w", eerr)
			}
		}()
	}

	var src io.ReadCloser
	isTar := sqlitePath == ""
	if isTar {
		if r.VolumeArchiver == nil {
			return 0, "", res, errors.New("volume archiving is not configured")
		}
		src, err = r.VolumeArchiver.Archive(ctx, dockerVolumeName)
	} else {
		if r.SqliteSnapshotter == nil {
			return 0, "", res, errors.New("sqlite snapshots are not configured")
		}
		src, err = r.SqliteSnapshotter.Snapshot(ctx, dockerVolumeName, sqlitePath)
	}
	if err != nil {
		return 0, "", res, fmt.Errorf("archive volume %q: %w", dockerVolumeName, err)
	}
	defer func() { _ = src.Close() }()

	var in io.Reader = src
	if opts.MaxBytes > 0 {
		in = &budgetReader{r: in, max: opts.MaxBytes}
	}
	var wait func() (TreeManifest, error)
	if isTar {
		in, wait = manifestTee(in)
	}

	sealedStream := opts.Sealer.Seal(in)
	counted := &countingReader{r: sealedStream, hash: sha256.New()}
	upErr := r.Uploader.Upload(ctx, dest, objectKey, counted, -1)
	_ = sealedStream.Close()

	var man TreeManifest
	var manErr error
	if wait != nil {
		man, manErr = wait()
	}
	if upErr != nil {
		return counted.n, "", res, fmt.Errorf("upload volume backup for %q: %w", dockerVolumeName, upErr)
	}
	if isTar {
		if manErr != nil {
			return counted.n, "", res, fmt.Errorf("archive of volume %q is not a readable tar: %w", dockerVolumeName, manErr)
		}
		res.manifest, res.hasMan = man, true
	}
	return counted.n, hex.EncodeToString(counted.hash.Sum(nil)), res, nil
}

type budgetReader struct {
	r   io.Reader
	n   int64
	max int64
}

func (b *budgetReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.n += int64(n)
	if b.n > b.max {
		return n, fmt.Errorf("%w (limit %d MB, set by %s)", ErrBackupTooLarge, b.max>>20, EnvBackupMaxSizeMB)
	}
	return n, err
}
