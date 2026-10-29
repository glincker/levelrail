package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envCopyConcurrency = "APP_BACKUP_COPY_CONCURRENCY"
	envCopyLagAlert    = "APP_BACKUP_COPY_LAG_ALERT"

	defaultCopyConcurrency = 2
	defaultCopyLagAlert    = time.Hour
	copyPollInterval       = time.Minute
)

// CopyConcurrencyFromEnv reads APP_BACKUP_COPY_CONCURRENCY, a positive int.
func CopyConcurrencyFromEnv(lookup func(string) (string, bool)) int {
	if v, ok := lookup(envCopyConcurrency); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultCopyConcurrency
}

// CopyLagAlertFromEnv reads APP_BACKUP_COPY_LAG_ALERT, a Go duration such as 90m.
func CopyLagAlertFromEnv(lookup func(string) (string, bool)) time.Duration {
	if v, ok := lookup(envCopyLagAlert); ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultCopyLagAlert
}

// CopyStore is the store surface the copy runner needs.
type CopyStore interface {
	TargetGetter
	GetBackupCopySettings(ctx context.Context) (store.BackupCopySettings, error)
	EnqueueBackupCopies(ctx context.Context, targetID, since, now string) error
	ClaimBackupCopies(ctx context.Context, limit int) ([]store.BackupCopy, error)
	FinishBackupCopy(ctx context.Context, id string, size int64, errMsg, now string) error
	ResetStuckBackupCopies(ctx context.Context) error
}

// CopyRunner mirrors each finished backup from its primary destination to the
// configured second one. Credentials are resolved per copy and never logged.
type CopyRunner struct {
	Store       CopyStore
	Secrets     SecretsResolver
	Downloader  Downloader
	Uploader    Uploader
	Concurrency int
	Interval    time.Duration
	Logger      *slog.Logger
	Now         func() time.Time
}

func (c *CopyRunner) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// Run polls until ctx ends.
func (c *CopyRunner) Run(ctx context.Context) error {
	if err := c.Store.ResetStuckBackupCopies(ctx); err != nil {
		return err
	}
	interval := c.Interval
	if interval <= 0 {
		interval = copyPollInterval
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if _, err := c.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) && c.Logger != nil {
			c.Logger.Warn("backup copy pass failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// RunOnce queues new backups and copies a bounded batch, returning how many copies succeeded.
func (c *CopyRunner) RunOnce(ctx context.Context) (int, error) {
	settings, err := c.Store.GetBackupCopySettings(ctx)
	if err != nil {
		return 0, fmt.Errorf("backup copy: read settings: %w", err)
	}
	if !settings.Enabled || settings.TargetID == "" {
		return 0, nil
	}
	now := c.now().Format(time.RFC3339)
	if err := c.Store.EnqueueBackupCopies(ctx, settings.TargetID, settings.EnabledAt, now); err != nil {
		return 0, err
	}
	workers := c.Concurrency
	if workers <= 0 {
		workers = defaultCopyConcurrency
	}
	batch, err := c.Store.ClaimBackupCopies(ctx, workers)
	if err != nil {
		return 0, err
	}
	var (
		wg sync.WaitGroup
		mu sync.Mutex
		ok int
	)
	for _, item := range batch {
		wg.Add(1)
		go func(item store.BackupCopy) {
			defer wg.Done()
			size, cerr := c.copyOne(ctx, item)
			msg := ""
			if cerr != nil {
				msg = cerr.Error()
			}
			if ferr := c.Store.FinishBackupCopy(context.WithoutCancel(ctx), item.BackupHistoryID, size, msg, c.now().Format(time.RFC3339)); ferr != nil && c.Logger != nil {
				c.Logger.Warn("backup copy: record outcome failed", slog.String("backup", item.BackupHistoryID), slog.String("error", ferr.Error()))
			}
			if cerr == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(item)
	}
	wg.Wait()
	return ok, nil
}

// historyGetter is optional on CopyStore: without it the copy skips checksum
// verification and the manifest sidecar, which only older fakes lack.
type historyGetter interface {
	GetBackupHistory(ctx context.Context, id string) (store.BackupHistory, error)
}

// CopyManifestSuffix names the sidecar written next to every copied object so
// a restore from the second destination alone has the checksum and codec.
const CopyManifestSuffix = ".manifest.json"

// CopyManifest is the sidecar document: what a restore needs besides the dump.
type CopyManifest struct {
	HistoryID      string `json:"history_id"`
	Resource       string `json:"resource"`
	ResourceKind   string `json:"resource_kind"`
	ObjectKey      string `json:"object_key"`
	SizeBytes      int64  `json:"size_bytes"`
	ChecksumSHA256 string `json:"checksum_sha256"`
	Codec          string `json:"codec,omitempty"`
	ManifestDigest string `json:"manifest_digest,omitempty"`
	ManifestFiles  int64  `json:"manifest_files,omitempty"`
	ManifestBytes  int64  `json:"manifest_bytes,omitempty"`
	FinishedAt     string `json:"finished_at"`
}

func (c *CopyRunner) copyOne(ctx context.Context, item store.BackupCopy) (int64, error) {
	src, err := resolveCopyDestination(ctx, c.Store, c.Secrets, item.SourceTargetID)
	if err != nil {
		return 0, err
	}
	dst, err := resolveCopyDestination(ctx, c.Store, c.Secrets, item.TargetID)
	if err != nil {
		return 0, err
	}
	var hist store.BackupHistory
	if hg, ok := c.Store.(historyGetter); ok {
		if hist, err = hg.GetBackupHistory(ctx, item.BackupHistoryID); err != nil {
			return 0, fmt.Errorf("read backup record: %w", err)
		}
	}
	body, err := c.Downloader.Download(ctx, src, item.ObjectKey)
	if err != nil {
		return 0, fmt.Errorf("read from primary: %w", err)
	}
	defer func() { _ = body.Close() }()
	h := sha256.New()
	if err := c.Uploader.Upload(ctx, dst, item.ObjectKey, io.TeeReader(body, h), item.BackupSize); err != nil {
		return 0, fmt.Errorf("write to second destination: %w", err)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if hist.ChecksumSHA256 != "" && sum != hist.ChecksumSHA256 {
		return 0, fmt.Errorf("primary object checksum %s does not match the recorded %s", sum, hist.ChecksumSHA256)
	}
	if err := c.verifyCopy(ctx, dst, item.ObjectKey, sum); err != nil {
		return 0, err
	}
	if hist.ID != "" {
		if err := c.writeManifest(ctx, dst, item, hist, sum); err != nil {
			return 0, err
		}
	}
	return item.BackupSize, nil
}

// verifyCopy reads the copy back so a truncated or corrupted write is a failed copy.
func (c *CopyRunner) verifyCopy(ctx context.Context, dst Destination, key, want string) error {
	rc, err := c.Downloader.Download(ctx, dst, key)
	if err != nil {
		return fmt.Errorf("verify second destination: %w", err)
	}
	defer func() { _ = rc.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, rc); err != nil {
		return fmt.Errorf("verify second destination: %w", err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("copy checksum %s does not match the source %s", got, want)
	}
	return nil
}

func (c *CopyRunner) writeManifest(ctx context.Context, dst Destination, item store.BackupCopy, hist store.BackupHistory, sum string) error {
	raw, err := json.Marshal(CopyManifest{
		HistoryID: hist.ID, Resource: item.ResourceName, ResourceKind: item.ResourceKind, ObjectKey: item.ObjectKey,
		SizeBytes: item.BackupSize, ChecksumSHA256: sum, Codec: hist.Codec, ManifestDigest: hist.ManifestDigest,
		ManifestFiles: hist.ManifestFiles, ManifestBytes: hist.ManifestBytes, FinishedAt: hist.FinishedAt,
	})
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	if err := c.Uploader.Upload(ctx, dst, item.ObjectKey+CopyManifestSuffix, bytes.NewReader(raw), int64(len(raw))); err != nil {
		return fmt.Errorf("write manifest to second destination: %w", err)
	}
	return nil
}
