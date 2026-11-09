package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	volumeUsageDefaultThreshold = 85.0
	volumeUsageDefaultInterval  = 5 * time.Minute
	volumeUsageDefaultTotalPath = "/var/lib/docker"
)

type volumeUsageDocker interface {
	ListNamedVolumes(ctx context.Context) ([]docker.NamedVolume, error)
}

type volumeUsageServices interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
}

type volumeUsageWriter interface {
	WriteSamples(ctx context.Context, samples []telemetry.Sample) error
}

// volumeUsage samples app volume sizes and lists them for the alert evaluator.
// Capacity is the filesystem under the Docker data root, since volumes have no quota.
type volumeUsage struct {
	docker   volumeUsageDocker
	services volumeUsageServices
	writer   volumeUsageWriter
	total    func() int64
	logger   *slog.Logger
}

// ListVolumes satisfies alerting.VolumeSource with IDs shaped <app>/<volume>.
func (v *volumeUsage) ListVolumes(ctx context.Context) ([]alerting.VolumeRef, error) {
	svcs, err := v.services.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []alerting.VolumeRef
	for _, s := range svcs {
		for _, vol := range s.Volumes {
			out = append(out, alerting.VolumeRef{ID: s.Name + "/" + vol.Name, Name: vol.Name, App: s.Name})
		}
	}
	return out, nil
}

// sample writes used and total bytes for each app volume the daemon reports a size for.
func (v *volumeUsage) sample(ctx context.Context, at time.Time) error {
	named, err := v.docker.ListNamedVolumes(ctx)
	if err != nil {
		return err
	}
	sizes := make(map[string]int64, len(named))
	for _, n := range named {
		if n.SizeBytes >= 0 {
			sizes[n.Name] = n.SizeBytes
		}
	}
	total := float64(v.total())
	if total <= 0 {
		return nil
	}
	refs, err := v.ListVolumes(ctx)
	if err != nil {
		return err
	}
	var samples []telemetry.Sample
	for _, ref := range refs {
		used, ok := sizes[ref.Name]
		if !ok {
			continue
		}
		id := alerting.VolumeResourceID(ref.ID)
		samples = append(samples,
			telemetry.Sample{ResourceID: id, Metric: alerting.MetricVolumeUsedBytes, Timestamp: at, Value: float64(used)},
			telemetry.Sample{ResourceID: id, Metric: alerting.MetricVolumeTotalBytes, Timestamp: at, Value: total},
		)
	}
	if len(samples) == 0 {
		return nil
	}
	return v.writer.WriteSamples(ctx, samples)
}

// run samples once at start and then on every interval until ctx ends.
func (v *volumeUsage) run(ctx context.Context, interval time.Duration) {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if err := v.sample(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			v.logger.Warn("volume usage sample failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// volumeUsageThreshold reads the alert percent from env; unset or invalid uses the default.
func volumeUsageThreshold(logger *slog.Logger) func() float64 {
	raw := os.Getenv("APP_ALERT_VOLUME_USAGE_THRESHOLD_PERCENT")
	val := volumeUsageDefaultThreshold
	if raw != "" {
		p, err := strconv.ParseFloat(raw, 64)
		if err != nil || p <= 0 || p > 100 {
			logger.Warn("invalid APP_ALERT_VOLUME_USAGE_THRESHOLD_PERCENT, using the default", slog.String("value", raw))
		} else {
			val = p
		}
	}
	return func() float64 { return val }
}

// wireVolumeUsage enables volume_usage alert rules and starts the sampler.
func wireVolumeUsage(ctx context.Context, e *alerting.Engine, d volumeUsageDocker, svcs volumeUsageServices, w volumeUsageWriter, logger *slog.Logger) {
	path := os.Getenv("APP_VOLUME_USAGE_TOTAL_PATH")
	if path == "" {
		path = volumeUsageDefaultTotalPath
	}
	v := &volumeUsage{docker: d, services: svcs, writer: w, logger: logger, total: func() int64 { return filesystemTotalBytes(path) }}
	e.SetVolumeUsage(v, volumeUsageThreshold(logger))
	go v.run(ctx, envDurationOrDefault(logger, "APP_VOLUME_USAGE_SAMPLE_INTERVAL", volumeUsageDefaultInterval))
}
