package main

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

// wireModelPreflight enables Hugging Face preflight and the model cache
// manager. client may be nil, which leaves disk facts unknown.
func wireModelPreflight(svc *models.Service, client *docker.Client, prefix string, logger *slog.Logger) {
	svc.SetHuggingFace(models.NewHFClient(models.LoadHFConfig(), nil, logger), models.LoadFitConfig())
	if client != nil {
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
