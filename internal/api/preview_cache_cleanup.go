package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/build"
)

// dropPreviewBuildCache removes the BuildKit cache mounts of a torn down preview.
// Shared mode owns nothing of the preview, so it is left alone. Redis keys under the
// preview prefix are not touched here: they expire by TTL.
func (rt *Router) dropPreviewBuildCache(ctx context.Context, previewAppID string) {
	if rt.appBuildCache == nil || rt.apps == nil {
		return
	}
	mode := build.PreviewCacheMode(ctx)
	if mode == build.PreviewCacheSharedWithApp {
		return
	}
	id := build.FitCacheID(previewAppID)
	if id == "" {
		return
	}
	var siblings []string
	if all, err := rt.apps.ListDesiredServices(ctx); err == nil {
		for _, s := range all {
			if sid := build.SanitizeCacheID(s.Name, ""); sid != "" && sid != id {
				siblings = append(siblings, sid)
			}
		}
	}
	if _, err := rt.appBuildCache.PruneAppCache(ctx, id, siblings); err != nil {
		rt.logger.Warn("api: preview teardown: prune build cache failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewAppID))
	}
}
