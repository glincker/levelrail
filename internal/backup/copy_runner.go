package backup

import (
	"context"
	"errors"
	"fmt"
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

func (c *CopyRunner) copyOne(ctx context.Context, item store.BackupCopy) (int64, error) {
	src, err := resolveTargetDestination(ctx, c.Store, c.Secrets, item.SourceTargetID)
	if err != nil {
		return 0, err
	}
	dst, err := resolveTargetDestination(ctx, c.Store, c.Secrets, item.TargetID)
	if err != nil {
		return 0, err
	}
	body, err := c.Downloader.Download(ctx, src, item.ObjectKey)
	if err != nil {
		return 0, fmt.Errorf("read from primary: %w", err)
	}
	defer func() { _ = body.Close() }()
	if err := c.Uploader.Upload(ctx, dst, item.ObjectKey, body, item.BackupSize); err != nil {
		return 0, fmt.Errorf("write to second destination: %w", err)
	}
	return item.BackupSize, nil
}
