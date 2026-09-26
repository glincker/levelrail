package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// teardownPullRequestPreview deletes a closed pull request's preview
// app and its member service(s), then its preview_environments row.
// Idempotent: a PR with no known preview (already torn down, or one
// that was opened before previews were enabled) is reported as ignored,
// not an error, the same "closing a PR twice" tolerance a real webhook
// delivery retry needs.
func (rt *Router) teardownPullRequestPreview(ctx context.Context, appName string, prNumber int) (int, string) {
	preview, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, appName, prNumber)
	if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		return http.StatusOK, fmt.Sprintf("ignored: no preview environment found for pr #%d\n", prNumber)
	}
	if err != nil {
		rt.logger.Error("api: pull request webhook: load preview for teardown failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", prNumber))
		return http.StatusInternalServerError, "internal error\n"
	}

	return rt.teardownPreviewRecordReason(ctx, *preview, "The pull request was closed or merged, so this preview was removed.")
}

// teardownPreviewRecord is teardownPullRequestPreview's and
// handleTeardownPreview's (preview_environments_handlers.go, the manual
// teardown action) shared implementation: delete every member service,
// then the app row, then the preview_environments row itself, in that
// order, so a crash or a failed step always leaves something concrete
// (the row, or the still-linked app) for the next attempt to find and
// retry rather than an orphan with no record at all.
func (rt *Router) teardownPreviewRecord(ctx context.Context, preview store.PreviewEnvironment) (int, string) {
	return rt.teardownPreviewRecordReason(ctx, preview, "This preview was removed.")
}

// teardownPreviewRecordReason is teardownPreviewRecord with the reason shown
// in the pull request's status comment once the preview is gone.
func (rt *Router) teardownPreviewRecordReason(ctx context.Context, preview store.PreviewEnvironment, reason string) (int, string) {
	failed := rt.teardownPreviewApp(ctx, preview.PreviewAppID)
	failed = append(failed, rt.teardownPreviewEphemeralDatabases(ctx, preview.ID)...)
	if len(failed) > 0 {
		preview.Status = store.PreviewStatusFailed
		preview.StatusReason = fmt.Sprintf("teardown left %d resource(s) undeleted: %s", len(failed), strings.Join(failed, ", "))
		preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, preview); err != nil {
			rt.logger.Error("api: preview teardown: record failure failed", slog.String("error", err.Error()), slog.String("preview_id", preview.ID))
		}
		rt.logger.Error("api: preview teardown partially failed", slog.String("app_name", preview.AppName), slog.Int("pr_number", preview.PRNumber), slog.Any("failed_resources", failed))
		return http.StatusMultiStatus, fmt.Sprintf("preview teardown partially failed: %d resource(s) undeleted\n", len(failed))
	}

	if err := rt.previewEnvironments.DeletePreviewEnvironment(ctx, preview.ID); err != nil {
		rt.logger.Error("api: preview teardown: delete record failed", slog.String("error", err.Error()), slog.String("preview_id", preview.ID))
		return http.StatusInternalServerError, "internal error\n"
	}

	rt.notifyPreviewRemoved(ctx, preview, reason)

	rt.logger.Info("api: preview torn down", slog.String("app_name", preview.AppName), slog.Int("pr_number", preview.PRNumber))
	return http.StatusOK, fmt.Sprintf("preview %q torn down\n", preview.PreviewAppID)
}

// teardownPreviewApp deletes every DesiredService belonging to
// previewAppID and, once none remain, the store.App row itself,
// mirroring handleDeleteApp/deleteAppIfOrphaned's own two-step deletion
// exactly. Returns the names of any services that failed to delete;
// nil means teardown fully succeeded. A previewAppID with no matching
// store.App (already deleted, e.g. a retried teardown) is treated as
// already torn down, not a failure.
func (rt *Router) teardownPreviewApp(ctx context.Context, previewAppID string) []string {
	app, err := rt.appGroups.GetAppByName(ctx, previewAppID)
	if errors.Is(err, store.ErrAppNotFound) {
		return nil
	}
	if err != nil {
		rt.logger.Error("api: teardown preview app: load app failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewAppID))
		return []string{previewAppID}
	}

	services, err := rt.appGroups.ListServicesByApp(ctx, app.ID)
	if err != nil {
		rt.logger.Error("api: teardown preview app: list services failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewAppID))
		return []string{previewAppID}
	}

	var failed []string
	for _, svc := range services {
		if err := rt.apps.DeleteDesiredService(ctx, svc.Name); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
			rt.logger.Error("api: teardown preview app: delete service failed", slog.String("error", err.Error()), slog.String("service", svc.Name))
			failed = append(failed, svc.Name)
			continue
		}
		rt.teardownServiceContainers(svc.Name, svc.NodeID)
	}
	if len(failed) > 0 {
		return failed
	}

	if err := rt.appGroups.DeleteApp(ctx, app.ID); err != nil && !errors.Is(err, store.ErrAppNotFound) {
		rt.logger.Error("api: teardown preview app: delete app row failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewAppID))
	}
	return nil
}
