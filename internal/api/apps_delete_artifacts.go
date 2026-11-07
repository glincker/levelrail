package api

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rollbackPinDeleter is the optional pin-row cleanup on the deploy gate store.
type rollbackPinDeleter interface {
	DeleteRollbackPin(ctx context.Context, serviceName, attemptID string) error
}

// removeAppArtifacts drops what a deleted app leaves on the host: rollback
// pin tags, its locally built images and stored upload archives. Unused only
// (Docker refuses to remove an image a container still runs); failures are logged.
func (rt *Router) removeAppArtifacts(ctx context.Context, name, nodeID string) {
	rt.clearDeletedAppBuildCache(ctx, name)
	if st := rt.uploadStore(); st != nil {
		if err := st.RemoveApp(name); err != nil {
			rt.logger.Warn("api: delete app: remove uploads failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	var pins []store.RollbackPin
	if rt.deployGate != nil {
		var err error
		if pins, err = rt.deployGate.ListRollbackPins(ctx, name); err != nil {
			rt.logger.Warn("api: delete app: list rollback pins failed", slog.String("error", err.Error()), slog.String("name", name))
		}
	}
	if rt.imageInventory != nil {
		rt.removeAppImages(ctx, name, nodeID, pins)
	}
	if del, ok := rt.deployGate.(rollbackPinDeleter); ok {
		for _, p := range pins {
			if err := del.DeleteRollbackPin(ctx, name, p.DeployAttemptID); err != nil {
				rt.logger.Warn("api: delete app: remove rollback pin failed", slog.String("error", err.Error()), slog.String("name", name))
			}
		}
	}
}

func (rt *Router) removeAppImages(ctx context.Context, name, nodeID string, pins []store.RollbackPin) {
	inv, err := rt.imageInventory(nodeID)
	if err != nil {
		rt.logger.Warn("api: delete app: image inventory unavailable", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	refs := map[string]bool{}
	for _, p := range pins {
		refs[p.PinRef] = true
	}
	if recs, lerr := inv.ListAllImages(ctx); lerr == nil {
		for _, r := range recs {
			if r.Containers > 0 {
				continue
			}
			for _, tag := range r.Tags {
				if imageRepoIs(tag, name) {
					refs[tag] = true
				}
			}
		}
	}
	for ref := range refs {
		if err := inv.RemoveImage(ctx, ref); err != nil {
			rt.logger.Warn("api: delete app: remove image failed", slog.String("error", err.Error()), slog.String("ref", ref))
		}
	}
}

// imageRepoIs matches a locally built image (repository equals the app name)
// without touching registry images that merely end in that name.
func imageRepoIs(tag, name string) bool {
	repo := tag
	if i := strings.LastIndex(tag, ":"); i > strings.LastIndex(tag, "/") {
		repo = tag[:i]
	}
	return repo == name
}

// clearDeletedAppBuildCache drops the app's BuildKit cache mounts, which
// otherwise outlive it and cannot be cleared by name. Best effort.
func (rt *Router) clearDeletedAppBuildCache(ctx context.Context, name string) {
	if rt.appBuildCache == nil {
		return
	}
	id := build.SanitizeCacheID(name, "")
	if id == "" {
		return
	}
	res, err := rt.appBuildCache.PruneAppCache(ctx, id, rt.siblingCacheIDs(ctx, id))
	if err != nil {
		rt.logger.Warn("api: delete app: clear build cache failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	appCacheUsageMemo.drop(id)
	rt.logger.Info("api: delete app: build cache cleared", slog.String("name", name), slog.Int64("freed_bytes", res.FreedBytes))
}
