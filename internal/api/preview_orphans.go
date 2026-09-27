package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultPreviewStuckAfter = 30 * time.Minute
	previewEnvironmentPrefix = "preview-env-"
)

// WithPreviewStuckAfter sets how long a preview may stay in deploying before
// the startup sweep marks it failed. Zero keeps the default.
func WithPreviewStuckAfter(d time.Duration) Option {
	return func(rt *Router) { rt.previewStuckAfter = d }
}

func (rt *Router) effectivePreviewStuckAfter() time.Duration {
	if rt.previewStuckAfter > 0 {
		return rt.previewStuckAfter
	}
	return defaultPreviewStuckAfter
}

// OrphanSweepResult counts what SweepOrphanPreviews repaired.
type OrphanSweepResult struct {
	StuckDeploys     int
	OrphanedServices int
}

// SweepOrphanPreviews repairs state a crash can leave behind: previews stuck
// in deploying (the control plane died mid-build) are marked failed, and
// services tagged as preview workloads that no preview row owns are removed
// with their containers. Owned resources are never touched.
func (rt *Router) SweepOrphanPreviews(ctx context.Context) (OrphanSweepResult, error) {
	var res OrphanSweepResult
	all, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		return res, fmt.Errorf("api: orphan preview sweep: list previews: %w", err)
	}

	var errs []error
	now := time.Now().UTC()
	for _, p := range all {
		if p.Status != store.PreviewStatusDeploying {
			continue
		}
		updatedAt, parseErr := time.Parse(time.RFC3339Nano, p.UpdatedAt)
		if parseErr != nil || now.Sub(updatedAt) <= rt.effectivePreviewStuckAfter() {
			continue
		}
		p.Status = store.PreviewStatusFailed
		p.StatusReason = "The control plane stopped while this preview was deploying. Push a new commit to retry."
		p.UpdatedAt = now.Format(time.RFC3339Nano)
		if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, p); err != nil {
			errs = append(errs, fmt.Errorf("mark stuck preview %q failed: %w", p.ID, err))
			continue
		}
		res.StuckDeploys++
	}

	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return res, errors.Join(append(errs, fmt.Errorf("api: orphan preview sweep: list services: %w", err))...)
	}
	for _, svc := range services {
		if !looksLikePreviewService(svc) || previewOwnsService(all, svc.Name) {
			continue
		}
		if err := rt.apps.DeleteDesiredService(ctx, svc.Name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			errs = append(errs, fmt.Errorf("delete orphaned preview service %q: %w", svc.Name, err))
			continue
		}
		rt.teardownServiceContainers(svc.Name, svc.NodeID)
		if svc.AppID != "" {
			rt.deleteAppIfOrphaned(ctx, svc.AppID)
		}
		rt.logger.Info("api: removed orphaned preview service", slog.String("service", svc.Name))
		res.OrphanedServices++
	}
	return res, errors.Join(errs...)
}

// previewOwnsService reports whether name is a preview's own service: the
// preview app itself, or a "<preview app>-<key>" fan-out sibling.
func previewOwnsService(previews []store.PreviewEnvironment, name string) bool {
	for _, p := range previews {
		if name == p.PreviewAppID || strings.HasPrefix(name, p.PreviewAppID+"-") {
			return true
		}
	}
	return false
}

// looksLikePreviewService requires both the preview environment tag and the
// "<app>-pr-<number>" name shape, so a service an operator moved into a
// preview environment by hand is never mistaken for an orphan.
func looksLikePreviewService(svc store.DesiredService) bool {
	appName, ok := strings.CutPrefix(svc.EnvironmentID, previewEnvironmentPrefix)
	if !ok || appName == "" {
		return false
	}
	rest, ok := strings.CutPrefix(svc.Name, appName+"-pr-")
	return ok && rest != "" && rest[0] >= '0' && rest[0] <= '9'
}
