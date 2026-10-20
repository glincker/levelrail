package main

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

// wireModelPreflight enables Hugging Face preflight and the model cache
// manager. client may be nil, which leaves disk facts unknown.
func wireModelPreflight(svc *models.Service, client *docker.Client, prefix string, logger *slog.Logger) {
	svc.SetHuggingFace(models.NewHFClient(models.LoadHFConfig(), nil, logger), models.LoadFitConfig())
	svc.SetHostFacts(localHostFacts)
	if client != nil {
		svc.SetBenchmark(models.LoadBenchConfig(), nil, benchStats(client, prefix))
		svc.SetCacheBackend(client, prefix)
		svc.SetDiskFacts(func(ctx context.Context, nodeID string) (free, total int64, ok bool) {
			return dockerDiskFacts(ctx, client, diskspace.Usage, nodeID, logger)
		})
	}
}

type rootDirSource interface {
	DockerRootDir(ctx context.Context) (string, error)
}

// dockerDiskFacts measures the filesystem holding Docker's volumes, which is
// where model weights land. Only the local node ("") can be measured from
// here; remote nodes stay unknown rather than borrowing another filesystem.
func dockerDiskFacts(ctx context.Context, src rootDirSource, usage func(string) (int64, int64, error), nodeID string, logger *slog.Logger) (free, total int64, ok bool) {
	if nodeID != "" {
		return 0, 0, false
	}
	root, err := src.DockerRootDir(ctx)
	if err != nil || root == "" {
		logger.Warn("models: docker root dir unavailable for disk check", slog.Any("error", err))
		return 0, 0, false
	}
	free, total, err = usage(root)
	if err != nil || total <= 0 {
		logger.Warn("models: cannot stat docker root dir", slog.String("path", root), slog.Any("error", err))
		return 0, 0, false
	}
	return free, total, true
}

// localHostFacts reports the control plane host's CPUs and memory. Remote
// nodes stay unknown, so CPU models there need explicit limits.
func localHostFacts(_ context.Context, nodeID string) (models.HostFacts, bool) {
	if nodeID != "" {
		return models.HostFacts{}, false
	}
	total, avail, err := telemetry.HostMemoryBytes()
	if err != nil || total <= 0 {
		return models.HostFacts{CPUs: runtime.NumCPU()}, false
	}
	return models.HostFacts{CPUs: runtime.NumCPU(), MemTotalBytes: total, MemAvailBytes: max(avail, 0)}, true
}

// benchStats reads a local model container's memory use for the benchmark.
func benchStats(client *docker.Client, prefix string) models.BenchStatsFunc {
	return func(ctx context.Context, m store.Model) (used, limit uint64, ok bool) {
		if m.NodeID != "" {
			return 0, 0, false
		}
		list, err := client.ListByPrefix(ctx, models.ContainerPrefix(prefix, m.Name))
		if err != nil {
			return 0, 0, false
		}
		for _, c := range list {
			if !c.Running {
				continue
			}
			st, err := client.Stats(ctx, c.ID)
			if err != nil {
				return 0, 0, false
			}
			return st.MemoryUsageBytes, st.MemoryLimitBytes, true
		}
		return 0, 0, false
	}
}
