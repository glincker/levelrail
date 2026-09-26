package main

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const defaultNodeDiskFactMaxAge = 10 * time.Minute

func nodeDiskFactMaxAge() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_NODE_DISK_FACT_MAX_AGE")); err == nil && d > 0 {
		return d
	}
	return defaultNodeDiskFactMaxAge
}

// wireModelPreflight enables Hugging Face preflight and the model cache
// manager. client and telemetryDB may be nil, which leaves the matching
// facts unknown.
func wireModelPreflight(svc *models.Service, client *docker.Client, telemetryDB *telemetry.DB, prefix string, logger *slog.Logger) {
	svc.SetHuggingFace(models.NewHFClient(models.LoadHFConfig(), nil, logger), models.LoadFitConfig())
	if client != nil {
		svc.SetCacheBackend(client, prefix)
	}
	if telemetryDB != nil {
		maxAge := nodeDiskFactMaxAge()
		svc.SetDiskFacts(func(ctx context.Context, nodeID string) (free, total int64, ok bool) {
			return nodeDiskFacts(ctx, telemetryDB, nodeID, maxAge, time.Now())
		})
	}
}

type latestByMetric interface {
	LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error)
}

func nodeDiskFacts(ctx context.Context, src latestByMetric, nodeID string, maxAge time.Duration, now time.Time) (free, total int64, ok bool) {
	resource := "node:" + nodeID
	read := func(metric string) (float64, bool) {
		samples, err := src.LatestByMetric(ctx, metric)
		if err != nil {
			return 0, false
		}
		for _, s := range samples {
			if strings.EqualFold(s.ResourceID, resource) && now.Sub(s.Timestamp) <= maxAge {
				return s.Value, true
			}
		}
		return 0, false
	}
	used, okUsed := read(telemetry.MetricDiskUsedBytes)
	totalV, okTotal := read(telemetry.MetricDiskTotalBytes)
	if !okUsed || !okTotal || totalV <= 0 {
		return 0, 0, false
	}
	return max(int64(totalV-used), 0), int64(totalV), true
}
