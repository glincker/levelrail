package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

type deployURLLister interface {
	ListDeployURLs(ctx context.Context, appName string) ([]store.DeployURL, error)
}

// deployURLsByAttempt maps an attempt id of app to the https URL of its retained copy.
func (rt *Router) deployURLsByAttempt(ctx context.Context, app string) map[string]string {
	l, ok := rt.deployAttempts.(deployURLLister)
	if !ok {
		return nil
	}
	rows, err := l.ListDeployURLs(ctx, app)
	if err != nil {
		rt.logger.Warn("api: list deploy urls failed", slog.String("error", err.Error()), slog.String("app", app))
		return nil
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.AttemptID] = rt.publicHTTPSURL(ctx, r.Host)
	}
	return out
}

// DeployURLHost is the hostname a retained deploy service is served on: one
// label under the apps base domain, else the sslip fallback for the public IP.
func (rt *Router) DeployURLHost(ctx context.Context, serviceName string) string {
	if base := rt.appsBaseDomain(ctx); base != "" {
		return appBaseHost(serviceName, base)
	}
	if rt.publicHost == "" {
		return ""
	}
	host, _ := rt.autoHost(ctx, serviceName)
	return host
}

// RemoveDeployURLService deletes a retained deploy service and tears its
// containers down. A teardown that is still pending is retried by the reconciler.
func (rt *Router) RemoveDeployURLService(ctx context.Context, serviceName string) error {
	_, err := rt.deleteApp(ctx, serviceName)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		return fmt.Errorf("remove deploy url service %q: %w", serviceName, err)
	}
	return nil
}

// NudgeReconcilers asks the reconcilers to run now.
func (rt *Router) NudgeReconcilers() { rt.nudgeReconciler() }

func (rt *Router) retainedDeployServices(ctx context.Context) map[string]bool {
	l, ok := rt.deployAttempts.(deployURLLister)
	if !ok {
		return nil
	}
	rows, err := l.ListDeployURLs(ctx, "")
	if err != nil {
		rt.logger.Warn("api: list retained deploy services failed", slog.String("error", err.Error()))
		return nil
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.ServiceName] = true
	}
	return out
}

// listPublicApps is ListDesiredServices without the retained deploy services,
// which are rollback machinery rather than apps an operator manages.
func (rt *Router) listPublicApps(ctx context.Context) ([]store.DesiredService, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	hidden := rt.retainedDeployServices(ctx)
	if len(hidden) == 0 {
		return svcs, nil
	}
	out := make([]store.DesiredService, 0, len(svcs))
	for _, s := range svcs {
		if !hidden[s.Name] {
			out = append(out, s)
		}
	}
	return out, nil
}
