package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// KindVolumeUsage fires when a volume's used percent meets the configured threshold.
const KindVolumeUsage Kind = "volume_usage"

// Volume size samples are stored under "volume:<id>" so a size history is a plain metrics query.
const (
	MetricVolumeUsedBytes  = "volume_used_bytes"
	MetricVolumeTotalBytes = "volume_total_bytes"
	volumeUsageLookback    = 10 * time.Minute
)

// VolumeRef names one volume the evaluator checks.
type VolumeRef struct {
	ID   string
	Name string
	App  string
}

// VolumeSource lists the volumes to check.
type VolumeSource interface {
	ListVolumes(ctx context.Context) ([]VolumeRef, error)
}

// VolumeResourceID is the metrics resource ID for a volume.
func VolumeResourceID(volumeID string) string { return "volume:" + volumeID }

// EvaluateVolumeUsage fires while any volume is at or above thresholdPercent and resolves once all are below.
// A volume without a recent sample or a positive total is skipped; a failing query is logged and skipped.
func EvaluateVolumeUsage(ctx context.Context, vols VolumeSource, metrics MetricsSource, r Rule, thresholdPercent float64, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if logger == nil {
		logger = slog.Default()
	}
	all, err := vols.ListVolumes(ctx)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: list volumes: %w", r.ID, err)
	}
	next := r
	next.LastEvaluatedAt = &now
	from := now.Add(-volumeUsageLookback)

	var (
		notices []string
		maxPct  float64
		have    bool
	)
	for _, v := range all {
		pct, ok := volumePercent(ctx, metrics, v, from, now, logger)
		if !ok {
			continue
		}
		if !have || pct > maxPct {
			maxPct, have = pct, true
		}
		if pct >= thresholdPercent {
			notices = append(notices, fmt.Sprintf("%s: volume %.1f%% used (threshold %.1f%%)", volumeLabel(v), pct, thresholdPercent))
		}
	}
	if have {
		next.LastValue = &maxPct
	}
	return advanceState(next, r, len(notices) > 0, 0, now), notices, nil
}

func volumePercent(ctx context.Context, metrics MetricsSource, v VolumeRef, from, to time.Time, logger *slog.Logger) (float64, bool) {
	id := VolumeResourceID(v.ID)
	used, err := metrics.QueryMetrics(ctx, id, MetricVolumeUsedBytes, from, to)
	if err != nil {
		logger.Warn("alerting: volume usage query failed, skipping volume", slog.String("volume_id", v.ID), slog.String("error", err.Error()))
		return 0, false
	}
	total, err := metrics.QueryMetrics(ctx, id, MetricVolumeTotalBytes, from, to)
	if err != nil {
		logger.Warn("alerting: volume capacity query failed, skipping volume", slog.String("volume_id", v.ID), slog.String("error", err.Error()))
		return 0, false
	}
	if len(used) == 0 || len(total) == 0 || total[len(total)-1].Value <= 0 {
		return 0, false
	}
	return used[len(used)-1].Value / total[len(total)-1].Value * 100, true
}

func volumeLabel(v VolumeRef) string {
	name := v.Name
	if name == "" {
		name = v.ID
	}
	if v.App != "" {
		return v.App + "/" + name
	}
	return name
}

// VolumeSizeHistory returns used-byte samples for one volume, oldest first, for the size history view.
func VolumeSizeHistory(ctx context.Context, metrics MetricsSource, volumeID string, from, to time.Time) ([]telemetry.Sample, error) {
	return metrics.QueryMetrics(ctx, VolumeResourceID(volumeID), MetricVolumeUsedBytes, from, to)
}
