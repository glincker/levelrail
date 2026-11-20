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
	SaveDeployURL(ctx context.Context, u store.DeployURL) error
}

// retainedSiblingRows lists the retained previous-deploy copies of name. It must run before
// the app's row is deleted, because that delete clears the deploy-url rows that name them.
func (rt *Router) retainedSiblingRows(ctx context.Context, name string) []store.DeployURL {
	rs, ok := rt.deployAttempts.(deployURLRowStore)
	if !ok {
		return nil
	}
	rows, err := rs.ListDeployURLs(ctx, name)
	if err != nil {
		rt.logger.Warn("api: delete app: list retained copies failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil
	}
	return rows
}

// removeRetainedSiblings deletes the given retained copies of name in the same operation as
// the app, so none outlives it with its own host and certificate. A copy whose delete fails
// gets its row written back, and the deploy-URL syncer retries it.
func (rt *Router) removeRetainedSiblings(ctx context.Context, name string, rows []store.DeployURL) {
	rs, ok := rt.deployAttempts.(deployURLRowStore)
	if !ok {
		return
	}
	for _, row := range rows {
		if _, err := rt.deleteApp(ctx, row.ServiceName); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.logger.Warn("api: delete app: remove retained copy failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("copy", row.ServiceName))
			if serr := rs.SaveDeployURL(ctx, row); serr != nil {
				rt.logger.Warn("api: delete app: keep retained copy row failed", slog.String("error", serr.Error()), slog.String("copy", row.ServiceName))
			}
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
