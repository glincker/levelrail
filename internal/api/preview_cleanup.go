package api

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

// removeExistingVolumes removes the named volumes that exist on the control
// plane's own node. A volume already gone counts as clean, so a retried
// teardown converges. Returns the names it could not remove.
func (rt *Router) removeExistingVolumes(ctx context.Context, names []string) []string {
	if rt.orphanedVolumes == nil || len(names) == 0 {
		return nil
	}
	existing, err := rt.orphanedVolumes.ListNamedVolumes(ctx)
	if err != nil {
		rt.logger.Error("api: preview cleanup: list volumes failed", slog.String("error", err.Error()))
		return names
	}
	present := make(map[string]bool, len(existing))
	for _, v := range existing {
		present[v.Name] = true
	}
	var failed []string
	for _, n := range names {
		if !present[n] {
			continue
		}
		if err := rt.orphanedVolumes.RemoveVolume(ctx, n); err != nil {
			rt.logger.Error("api: preview cleanup: remove volume failed", slog.String("error", err.Error()), slog.String("volume", n))
			failed = append(failed, "volume:"+n)
			continue
		}
		rt.logger.Info("api: preview cleanup: volume removed", slog.String("volume", n))
	}
	return failed
}

// previewOwnedVolumes filters candidates to volumes no remaining desired
// service mounts, so a preview that reuses a production volume name never
// deletes production data.
func (rt *Router) previewOwnedVolumes(ctx context.Context, candidates []string) ([]string, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	all, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list services: %w", err)
	}
	inUse := make(map[string]bool)
	for _, svc := range all {
		for _, v := range svc.Volumes {
			inUse[v.Name] = true
		}
	}
	var owned []string
	for _, c := range candidates {
		if !inUse[c] {
			owned = append(owned, c)
		}
	}
	return owned, nil
}

// teardownServiceContainersSync is teardownServiceContainers that waits, so a
// volume can be removed right after its container is gone.
func (rt *Router) teardownServiceContainersSync(ctx context.Context, svc store.DesiredService) error {
	finalizer := rt.deleteFinalizer()
	if finalizer == nil {
		return nil
	}
	if err := finalizer.Begin(ctx, svc.Name, svc.NodeID); err != nil {
		return fmt.Errorf("record tombstone: %w", err)
	}
	tctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), rt.deleteTeardownTimeout())
	defer cancel()
	if err := finalizer.Finalize(tctx, svc.Name, svc.NodeID); err != nil {
		return fmt.Errorf("remove containers: %w", err)
	}
	return nil
}
