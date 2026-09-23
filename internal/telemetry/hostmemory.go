package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// MetricMemoryAvailableBytes and MetricMemoryTotalBytes are the sample
// metric names HostMemoryCollector writes, and the names internal/api
// reads back.
const (
	MetricMemoryAvailableBytes = "memory_available_bytes"
	MetricMemoryTotalBytes     = "memory_total_bytes"
)

// parseMemInfo is the pure part of a /proc/meminfo reading, split out for
// table-driven testing. Uses MemAvailable, not MemFree, since MemFree
// ignores reclaimable page cache.
func parseMemInfo(data []byte) (totalBytes, availableBytes int64, err error) {
	var total, available int64
	var haveTotal, haveAvailable bool

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			total, err = parseMemInfoLineKB(line)
			if err != nil {
				return 0, 0, err
			}
			haveTotal = true
		case strings.HasPrefix(line, "MemAvailable:"):
			available, err = parseMemInfoLineKB(line)
			if err != nil {
				return 0, 0, err
			}
			haveAvailable = true
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("telemetry: scan meminfo: %w", err)
	}
	if !haveTotal {
		return 0, 0, fmt.Errorf("telemetry: meminfo has no MemTotal line")
	}
	if !haveAvailable {
		return 0, 0, fmt.Errorf("telemetry: meminfo has no MemAvailable line")
	}
	return total * 1024, available * 1024, nil
}

// parseMemInfoLineKB parses a "Label:    12345 kB" /proc/meminfo line into
// its kB value.
func parseMemInfoLineKB(line string) (int64, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("telemetry: malformed meminfo line %q", line)
	}
	value, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("telemetry: parse meminfo value in %q: %w", line, err)
	}
	return value, nil
}

// HostMemoryBytes reads this host's total and available memory from
// /proc/meminfo, the same read HostMemoryCollector.CollectOnce performs
// on a poll tick. Exported for a one-shot caller (internal/api's ram
// doctor check) that has no running collector to read back from.
func HostMemoryBytes() (totalBytes, availableBytes int64, err error) {
	return hostMemoryBytes("/proc/meminfo")
}

// hostMemoryBytes reads a host's total and available memory from a
// /proc/meminfo-shaped file at path.
func hostMemoryBytes(path string) (totalBytes, availableBytes int64, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is "/proc/meminfo" in production (NewHostMemoryCollector), only overridden by this package's own tests
	if err != nil {
		return 0, 0, fmt.Errorf("telemetry: read %q: %w", path, err)
	}
	return parseMemInfo(data)
}

// HostMemoryCollector polls this host's real memory capacity on an
// interval and writes memory_total_bytes/memory_available_bytes
// samples under one resource ID, HostDiskCollector's own memory
// counterpart. Like HostDiskCollector, it reads the machine the control
// plane process itself runs on, not every node in a multi-node fleet.
type HostMemoryCollector struct {
	memInfoPath string
	resourceID  string
	store       *DB
	interval    time.Duration
	logger      *slog.Logger
}

// NewHostMemoryCollector builds a HostMemoryCollector reading
// /proc/meminfo. logger defaults to slog.Default() if nil.
func NewHostMemoryCollector(resourceID string, store *DB, interval time.Duration, logger *slog.Logger) *HostMemoryCollector {
	if logger == nil {
		logger = slog.Default()
	}
	return &HostMemoryCollector{memInfoPath: "/proc/meminfo", resourceID: resourceID, store: store, interval: interval, logger: logger}
}

// CollectOnce reads the host's current memory capacity once and writes it.
func (c *HostMemoryCollector) CollectOnce(ctx context.Context) error {
	total, available, err := hostMemoryBytes(c.memInfoPath)
	if err != nil {
		return fmt.Errorf("collect host memory: %w", err)
	}
	now := time.Now()
	return c.store.WriteSamples(ctx, []Sample{
		{ResourceID: c.resourceID, Metric: MetricMemoryTotalBytes, Timestamp: now, Value: float64(total)},
		{ResourceID: c.resourceID, Metric: MetricMemoryAvailableBytes, Timestamp: now, Value: float64(available)},
	})
}

// Run calls CollectOnce every interval until ctx is done, the same "log
// and keep going, one bad tick must not stop the collector" shape
// HostDiskCollector.Run already establishes.
func (c *HostMemoryCollector) Run(ctx context.Context) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := c.CollectOnce(ctx); err != nil {
				c.logger.Warn("telemetry: host memory collection tick failed", slog.String("error", err.Error()))
			}
		}
	}
}
