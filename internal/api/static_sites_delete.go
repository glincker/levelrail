package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const defaultStaticOrphanGrace = 10 * time.Minute

// staticSiteDeleter is the optional store surface that removes a published
// static site. *store.DB satisfies it.
type staticSiteDeleter interface {
	DeleteStaticSite(ctx context.Context, name string) error
}

type staticSiteGetter interface {
	GetStaticSite(ctx context.Context, name string) (*store.StaticSite, error)
}

// staticAttemptLister finds deploys that may still be about to write a site.
type staticAttemptLister interface {
	ListQueuedDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
	ListRunningDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
}

// WithStaticOrphanGrace sets how old an orphaned static site must be before
// the sweep removes it. Zero keeps the default.
func WithStaticOrphanGrace(d time.Duration) Option {
	return func(rt *Router) { rt.staticOrphanGrace = d }
}

// StaticOrphanGrace is the grace period the sweep uses.
func (rt *Router) StaticOrphanGrace() time.Duration {
	if rt.staticOrphanGrace > 0 {
		return rt.staticOrphanGrace
	}
	return defaultStaticOrphanGrace
}

// removeStaticSite drops the site row first, so the ingress route goes on the
// next reconcile even if file cleanup fails, then deletes the published tree.
// A site that does not exist is not an error.
func (rt *Router) removeStaticSite(ctx context.Context, name string) error {
	d, ok := rt.staticSites.(staticSiteDeleter)
	if !ok {
		return nil
	}
	if err := d.DeleteStaticSite(ctx, name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		return fmt.Errorf("delete static site %q: %w", name, err)
	}
	if rt.staticRootDir == "" {
		return nil
	}
	if err := deploy.RemoveStaticTree(rt.staticRootDir, name); err != nil {
		return fmt.Errorf("remove published files of %q: %w", name, err)
	}
	return nil
}

// handleDeleteStaticSite handles DELETE /api/v1/static-sites/{name}. A site
// whose app still exists is refused: deleting the app removes the site with it.
func (rt *Router) handleDeleteStaticSite(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "static site delete is not configured on this control plane")
		return
	}
	if _, err := getter.GetStaticSite(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "static site not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: delete static site: lookup", err, slog.String("name", name))
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err == nil {
		writeError(w, http.StatusConflict, "an app with this name still exists; delete the app instead")
		return
	} else if !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: delete static site: app lookup", err, slog.String("name", name))
		return
	}
	if err := rt.removeStaticSite(r.Context(), name); err != nil {
		rt.internalError(w, "api: delete static site failed", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// SweepOrphanStaticSites removes static sites with no desired service and no
// queued or running deploy, once older than the grace period, so a site whose
// app is still being created is never removed. It returns the removed names.
func (rt *Router) SweepOrphanStaticSites(ctx context.Context) ([]string, error) {
	sites, err := rt.staticSites.ListStaticSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: static orphan sweep: list sites: %w", err)
	}
	if len(sites) == 0 {
		return nil, nil
	}
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: static orphan sweep: list services: %w", err)
	}
	owned := make(map[string]bool, len(services))
	for _, s := range services {
		owned[s.Name] = true
	}
	previews, err := rt.staticPreviewNames(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: static orphan sweep: %w", err)
	}
	for name := range previews {
		owned[name] = true
	}
	pending, err := rt.pendingDeployServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: static orphan sweep: %w", err)
	}

	var (
		removed []string
		errs    []error
		now     = time.Now().UTC()
		grace   = rt.StaticOrphanGrace()
	)
	for _, site := range sites {
		if owned[site.Name] || pending[site.Name] {
			continue
		}
		if site.UpdatedAt.IsZero() || now.Sub(site.UpdatedAt) < grace {
			continue
		}
		if err := rt.removeStaticSite(ctx, site.Name); err != nil {
			errs = append(errs, err)
			continue
		}
		rt.logger.Info("api: removed orphaned static site", slog.String("name", site.Name))
		removed = append(removed, site.Name)
	}
	if len(removed) > 0 {
		rt.nudgeReconciler()
	}
	return removed, errors.Join(errs...)
}

func (rt *Router) pendingDeployServices(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	l, ok := rt.deployAttempts.(staticAttemptLister)
	if !ok {
		return out, nil
	}
	queued, err := l.ListQueuedDeployAttempts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list queued deploys: %w", err)
	}
	running, err := l.ListRunningDeployAttempts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list running deploys: %w", err)
	}
	for _, a := range append(queued, running...) {
		out[a.ServiceName] = true
	}
	return out, nil
}
