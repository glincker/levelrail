package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"syscall"
	"time"
)

// diskSpaceFromStatfs is the pure part of a disk-space reading, split
// out from diskSpaceBytes below so the arithmetic is table-testable
// without a real filesystem. used is total minus the blocks an
// unprivileged process can actually write into (Bavail), the same
// "honest free number" internal/api/status.go's DataDirFreeBytes
// already commits to over Bfree's root-reserved-inclusive count.
func diskSpaceFromStatfs(blocks, bavail uint64, bsize int64) (total, used int64) {
	total = int64(blocks) * bsize  //nolint:gosec // statfs fields are always non-negative in practice
	avail := int64(bavail) * bsize //nolint:gosec // statfs fields are always non-negative in practice
	return total, total - avail
}

// diskSpaceBytes reads path's filesystem capacity via statfs.
func diskSpaceBytes(path string) (total, used int64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("telemetry: statfs %q: %w", path, err)
	}
	total, used = diskSpaceFromStatfs(uint64(stat.Blocks), uint64(stat.Bavail), int64(stat.Bsize)) //nolint:gosec // statfs fields are always non-negative in practice
	return total, used, nil
}

// HostDiskCollector polls one filesystem path's capacity on an interval
// and writes disk_total_bytes/disk_used_bytes samples under one
// resource ID, the host-level counterpart to Collector's per-container
// polling above: a real reading of the volume that actually matters
// operationally (where images, volumes, and the SQLite stores live),
// not a per-container Docker stat.
type HostDiskCollector struct {
	path       string
	resourceID string
	store      *DB
	interval   time.Duration
	logger     *slog.Logger
}

// NewHostDiskCollector builds a HostDiskCollector. logger defaults to
// slog.Default() if nil.
func NewHostDiskCollector(path, resourceID string, store *DB, interval time.Duration, logger *slog.Logger) *HostDiskCollector {
	if logger == nil {
		logger = slog.Default()
	}
	return &HostDiskCollector{path: path, resourceID: resourceID, store: store, interval: interval, logger: logger}
}

// CollectOnce reads path's current capacity once and writes it.
func (c *HostDiskCollector) CollectOnce(ctx context.Context) error {
	total, used, err := diskSpaceBytes(c.path)
	if err != nil {
		return fmt.Errorf("collect host disk space: %w", err)
	}
	now := time.Now()
	return c.store.WriteSamples(ctx, []Sample{
		{ResourceID: c.resourceID, Metric: "disk_total_bytes", Timestamp: now, Value: float64(total)},
		{ResourceID: c.resourceID, Metric: "disk_used_bytes", Timestamp: now, Value: float64(used)},
	})
}

// Run calls CollectOnce every interval until ctx is done, the same "log
// and keep going, one bad tick must not stop the collector" shape
// Collector.Run above already establishes.
func (c *HostDiskCollector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.CollectOnce(ctx); err != nil {
				c.logger.Warn("telemetry: host disk collection tick failed", slog.String("error", err.Error()))
			}
		}
	}
}
