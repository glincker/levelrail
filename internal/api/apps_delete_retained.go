package api

import (
	"context"
	"errors"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/store"
)

type deployURLRowStore interface {
	ListDeployURLs(ctx context.Context, appName string) ([]store.DeployURL, error)
	DeleteDeployURL(ctx context.Context, serviceName string) error
}

// removeRetainedSiblings deletes the retained previous-deploy copies of name in the same
// operation as the app, so none outlives it with its own host and certificate. A copy whose
// delete fails keeps its row, and the deploy-URL syncer retries it.
func (rt *Router) removeRetainedSiblings(ctx context.Context, name string) {
	rs, ok := rt.deployAttempts.(deployURLRowStore)
	if !ok {
		return
	}
	rows, err := rs.ListDeployURLs(ctx, name)
	if err != nil {
		rt.logger.Warn("api: delete app: list retained copies failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	for _, row := range rows {
		if _, err := rt.deleteApp(ctx, row.ServiceName); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.logger.Warn("api: delete app: remove retained copy failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("copy", row.ServiceName))
			continue
		}
		if err := rs.DeleteDeployURL(ctx, row.ServiceName); err != nil {
			rt.logger.Warn("api: delete app: remove retained copy row failed", slog.String("error", err.Error()), slog.String("copy", row.ServiceName))
		}
	}
}

// retainedWakeBlocked reports a retained copy whose app is gone: it must stay down, never be woken.
func (rt *Router) retainedWakeBlocked(ctx context.Context, name string) bool {
	o, ok := rt.deployAttempts.(deployURLOwner)
	if !ok {
		return false
	}
	app, found, err := o.DeployURLApp(ctx, name)
	if err != nil || !found {
		return false
	}
	_, err = rt.apps.GetDesiredService(ctx, app)
	return errors.Is(err, store.ErrServiceNotFound)
}

// removeAppPushMirror drops the stored git push repo of a deleted app, so a same-name app
// starts from no history.
func (rt *Router) removeAppPushMirror(name string) {
	if rt.sourceCache == nil {
		return
	}
	if err := rt.sourceCache.ResetMirror(gitPushMirrorURL(name)); err != nil {
		rt.logger.Warn("api: delete app: remove push repo failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}
