package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/branchdeploy"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const (
	envBranchDeployTTL        = "APP_BRANCH_DEPLOY_TTL"
	envBranchDeployMaxPerApp  = "APP_BRANCH_DEPLOY_MAX_PER_APP"
	defaultBranchDeployTTL    = 14 * 24 * time.Hour
	defaultBranchDeployMaxApp = 10
	branchRefPrefix           = "refs/heads/"
	branchMessageMax          = 200
)

// BranchDeployStore is the store surface branch deploys need. *store.DB satisfies it.
type BranchDeployStore interface {
	GetBranchDeploySettings(ctx context.Context, appName string) (store.BranchDeploySettings, error)
	SaveBranchDeploySettings(ctx context.Context, s store.BranchDeploySettings) error
	SaveBranchDeploy(ctx context.Context, b store.BranchDeploy) error
	UpdateBranchDeploy(ctx context.Context, b store.BranchDeploy) error
	GetBranchDeployBySlug(ctx context.Context, appName, slug string) (*store.BranchDeploy, error)
	ListBranchDeploysByApp(ctx context.Context, appName string) ([]store.BranchDeploy, error)
	ListBranchDeploys(ctx context.Context) ([]store.BranchDeploy, error)
	DeleteBranchDeploy(ctx context.Context, id string) error
}

// branchDeployTTL is how long a branch deploy may go without a push before the sweep removes it.
func branchDeployTTL() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envBranchDeployTTL)); err == nil && d > 0 {
		return d
	}
	return defaultBranchDeployTTL
}

func branchDeployMaxPerApp() int {
	if n, err := strconv.Atoi(os.Getenv(envBranchDeployMaxPerApp)); err == nil && n > 0 {
		return n
	}
	return defaultBranchDeployMaxApp
}

func branchSlug(appName, branch string) string {
	return branchdeploy.Slug(branch, branchdeploy.MaxSlugLen(appName))
}

// branchDeployHost is the stable hostname for a branch: one label under the
// apps base domain so its wildcard record covers it, else br-<slug>.<app>.<primary>.
func (rt *Router) branchDeployHost(ctx context.Context, appName, slug string) string {
	if base := rt.appsBaseDomain(ctx); base != "" {
		return appBaseHost(branchdeploy.AppName(appName, slug), base)
	}
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil || settings.PrimaryDomain == "" {
		return ""
	}
	return fmt.Sprintf("br-%s.%s.%s", slug, ingress.SanitizeDNSLabel(appName), settings.PrimaryDomain)
}

func branchDeployScope(slug string) string { return forgeEnvPreview + ":branch:" + slug }

func productionBranch(gs store.GitSource) string {
	if gs.Branch == "" {
		return webhook.DefaultBranch
	}
	return gs.Branch
}

// handleBranchDeployPush routes a push to branch deploy handling. handled is
// false when the push is not a branch deploy matter and normal handling applies.
func (rt *Router) handleBranchDeployPush(ctx context.Context, appName string, gs store.GitSource, ev webhook.PushEvent) (status int, message string, handled bool) {
	if rt.branchDeploys == nil || !strings.HasPrefix(ev.Ref, branchRefPrefix) {
		return 0, "", false
	}
	branch := strings.TrimPrefix(ev.Ref, branchRefPrefix)
	if ev.Deleted {
		status, message = rt.teardownBranchDeployForDelete(ctx, appName, gs, branch)
		return status, message, true
	}
	if effectiveGitSourceTriggerMode(gs.TriggerMode) == spec.TriggerModeRelease {
		return 0, "", false
	}
	cfg, err := rt.branchDeploys.GetBranchDeploySettings(ctx, appName)
	if err != nil {
		rt.logger.Error("api: branch deploy: load settings failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return http.StatusInternalServerError, "internal error\n", true
	}
	if !(branchdeploy.Settings{Mode: cfg.Mode, Patterns: cfg.Patterns}).Eligible(branch, productionBranch(gs)) {
		return 0, "", false
	}
	if msg := skipPushForMarker(ev.HeadMessage); msg != "" && !isWebhookReplay(ctx) {
		return http.StatusOK, msg, true
	}
	if rt.builder == nil {
		return http.StatusNotImplemented, "git push deploys are not configured on this control plane\n", true
	}
	status, message = rt.deployBranch(ctx, appName, gs, branch, ev)
	return status, message, true
}

func (rt *Router) teardownBranchDeployForDelete(ctx context.Context, appName string, gs store.GitSource, branch string) (int, string) {
	if branch == productionBranch(gs) {
		return http.StatusOK, "ignored: the production branch was deleted, nothing was changed\n"
	}
	slug := branchSlug(appName, branch)
	row, err := rt.branchDeploys.GetBranchDeployBySlug(ctx, appName, slug)
	if errors.Is(err, store.ErrBranchDeployNotFound) {
		return http.StatusOK, fmt.Sprintf("ignored: no branch deploy for deleted branch %q\n", branch)
	}
	if err != nil {
		rt.logger.Error("api: branch deploy: load for delete failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return http.StatusInternalServerError, "internal error\n"
	}
	status, msg := rt.teardownBranchDeploy(ctx, *row)
	if status < http.StatusBadRequest {
		rt.deactivateForgeDeployments(ctx, appName, gs, branchDeployScope(slug))
	}
	return status, msg
}

// beginBranchDeploy creates or reuses the row for branch and marks it deploying.
func (rt *Router) beginBranchDeploy(ctx context.Context, appName, branch, slug string, ev webhook.PushEvent) (*store.BranchDeploy, int, string) {
	rt.branchDeployMu.Lock()
	defer rt.branchDeployMu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	row, err := rt.branchDeploys.GetBranchDeployBySlug(ctx, appName, slug)
	switch {
	case err == nil:
		row.Branch, row.HeadSHA, row.HeadMessage = branch, ev.After, firstLine(ev.HeadMessage, branchMessageMax)
		row.Status, row.StatusReason, row.UpdatedAt = store.BranchDeployDeploying, "", now
		if uerr := rt.branchDeploys.UpdateBranchDeploy(ctx, *row); uerr != nil {
			rt.logger.Error("api: branch deploy: update row failed", slog.String("error", uerr.Error()), slog.String("app_name", appName))
			return nil, http.StatusInternalServerError, "internal error\n"
		}
		return row, 0, ""
	case !errors.Is(err, store.ErrBranchDeployNotFound):
		rt.logger.Error("api: branch deploy: load row failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, http.StatusInternalServerError, "internal error\n"
	}

	existing, err := rt.branchDeploys.ListBranchDeploysByApp(ctx, appName)
	if err != nil {
		rt.logger.Error("api: branch deploy: list rows failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, http.StatusInternalServerError, "internal error\n"
	}
	if limit := branchDeployMaxPerApp(); len(existing) >= limit {
		return nil, http.StatusOK, fmt.Sprintf("ignored: %q already has %d branch deploys (limit %d, set %s)\n", appName, len(existing), limit, envBranchDeployMaxPerApp)
	}
	id, err := store.NewBranchDeployID()
	if err != nil {
		rt.logger.Error("api: branch deploy: mint id failed", slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "internal error\n"
	}
	row = &store.BranchDeploy{
		ID: id, AppName: appName, Branch: branch, Slug: slug, DeployAppID: branchdeploy.AppName(appName, slug),
		HeadSHA: ev.After, HeadMessage: firstLine(ev.HeadMessage, branchMessageMax), Status: store.BranchDeployDeploying, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.branchDeploys.SaveBranchDeploy(ctx, *row); err != nil {
		rt.logger.Error("api: branch deploy: save row failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, http.StatusInternalServerError, "internal error\n"
	}
	return row, 0, ""
}

func (rt *Router) failBranchDeploy(ctx context.Context, row *store.BranchDeploy, reason string) {
	row.Status, row.StatusReason = store.BranchDeployFailed, reason
	row.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.branchDeploys.UpdateBranchDeploy(ctx, *row); err != nil {
		rt.logger.Error("api: branch deploy: mark failed failed", slog.String("error", err.Error()), slog.String("branch_deploy_id", row.ID))
	}
}

// deployBranch builds the pushed commit under the branch's stable app name,
// reusing the preview deploy path so env, policy and exposure rules match.
func (rt *Router) deployBranch(ctx context.Context, appName string, gs store.GitSource, branch string, ev webhook.PushEvent) (int, string) {
	slug := branchSlug(appName, branch)
	wantDomain := rt.branchDeployHost(ctx, appName, slug)
	dep := rt.beginForgeDeployment(ctx, appName, gs, ev.After, forgeEnvPreview, branchDeployScope(slug), rt.previewURLFor(ctx, wantDomain))
	status, message := rt.deployBranchInner(ctx, appName, gs, branch, slug, wantDomain, ev)
	dep.finish(ctx, deploymentStateFor(status, message), strings.TrimSpace(message))
	return status, message
}

func (rt *Router) deployBranchInner(ctx context.Context, appName string, gs store.GitSource, branch, slug, wantDomain string, ev webhook.PushEvent) (int, string) {
	row, status, message := rt.beginBranchDeploy(ctx, appName, branch, slug, ev)
	if row == nil {
		return status, message
	}
	token, err := rt.resolveGitSourceDeployToken(ctx, appName)
	if err != nil {
		rt.failBranchDeploy(ctx, row, "resolve deploy token: "+err.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}
	sourceDir, cleanup, err := rt.gitSourceFetch(ctx, gs.RepoURL, ev.After, token)
	if err != nil {
		rt.logger.Error("api: branch deploy: fetch source failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.String("branch", branch))
		rt.failBranchDeploy(ctx, row, "fetch source: "+err.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}
	defer cleanup()

	policy := rt.previewSettings(ctx, appName)
	vars := previewEnvVars{Branch: branch, Policy: policy}
	var (
		usedDomain     string
		domainConflict bool
		deployErr      error
	)
	if len(gs.Services) > 0 {
		usedDomain, domainConflict, deployErr = rt.deployPreviewMulti(ctx, appName, row.DeployAppID, gs, sourceDir, ev.After, wantDomain, vars)
	} else {
		usedDomain, domainConflict, deployErr = rt.deployPreviewSingle(ctx, appName, row.DeployAppID, gs, sourceDir, ev.After, wantDomain, vars)
	}
	if deployErr != nil {
		rt.logger.Error("api: branch deploy: deploy failed", slog.String("error", deployErr.Error()), slog.String("app_name", appName), slog.String("branch", branch))
		rt.failBranchDeploy(ctx, row, deployErr.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}

	if _, envID, envErr := rt.ensurePreviewEnvironmentTier(ctx, appName); envErr != nil {
		rt.logger.Error("api: branch deploy: ensure environment tier failed", slog.String("error", envErr.Error()), slog.String("app_name", appName))
	} else if tagErr := rt.tagPreviewWithEnvironment(ctx, row.DeployAppID, gs, envID); tagErr != nil {
		rt.logger.Error("api: branch deploy: tag environment failed", slog.String("error", tagErr.Error()), slog.String("app_name", appName))
	}

	notes := rt.finishPreviewExposure(ctx, appName, row.DeployAppID, usedDomain, policy)
	row.Status, row.Domain, row.StatusReason = store.BranchDeployActive, usedDomain, strings.Join(notes, "; ")
	statusCode := http.StatusOK
	if domainConflict {
		row.StatusReason = fmt.Sprintf("domain %q was already in use, deployed without a domain", wantDomain)
		statusCode = http.StatusMultiStatus
	}
	row.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.branchDeploys.UpdateBranchDeploy(ctx, *row); err != nil {
		rt.logger.Error("api: branch deploy: finalize failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return http.StatusInternalServerError, "internal error\n"
	}
	rt.logger.Info("api: branch deploy ready", slog.String("app_name", appName), slog.String("branch", branch), slog.String("deploy_app", row.DeployAppID))
	return statusCode, fmt.Sprintf("branch deployed: %s\n", row.DeployAppID)
}

// teardownBranchDeploy removes the app, DNS and row. A partial failure keeps
// the row, marked failed with what is left, so a retry or the sweep finds it.
func (rt *Router) teardownBranchDeploy(ctx context.Context, row store.BranchDeploy) (int, string) {
	failed := rt.removePreviewDNS(ctx, store.PreviewEnvironment{AppName: row.AppName, PreviewAppID: row.DeployAppID, Domain: row.Domain})
	failed = append(failed, rt.teardownPreviewApp(ctx, row.DeployAppID)...)
	if len(failed) > 0 {
		row.Status = store.BranchDeployFailed
		row.StatusReason = fmt.Sprintf("teardown left %d resource(s) undeleted: %s", len(failed), strings.Join(failed, ", "))
		row.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := rt.branchDeploys.UpdateBranchDeploy(ctx, row); err != nil {
			rt.logger.Error("api: branch deploy teardown: record failure failed", slog.String("error", err.Error()), slog.String("branch_deploy_id", row.ID))
		}
		return http.StatusMultiStatus, fmt.Sprintf("branch deploy teardown partially failed: %d resource(s) undeleted\n", len(failed))
	}
	if err := rt.branchDeploys.DeleteBranchDeploy(ctx, row.ID); err != nil && !errors.Is(err, store.ErrBranchDeployNotFound) {
		rt.logger.Error("api: branch deploy teardown: delete row failed", slog.String("error", err.Error()), slog.String("branch_deploy_id", row.ID))
		return http.StatusInternalServerError, "internal error\n"
	}
	rt.logger.Info("api: branch deploy removed", slog.String("app_name", row.AppName), slog.String("branch", row.Branch))
	return http.StatusOK, fmt.Sprintf("branch deploy %q removed\n", row.DeployAppID)
}

// SweepStaleBranchDeploys removes branch deploys with no push for the idle TTL.
// One failing teardown never blocks the rest; a partial teardown counts as
// handled because it bumps the row's update time.
func (rt *Router) SweepStaleBranchDeploys(ctx context.Context) (swept int, err error) {
	if rt.branchDeploys == nil {
		return 0, nil
	}
	all, listErr := rt.branchDeploys.ListBranchDeploys(ctx)
	if listErr != nil {
		return 0, fmt.Errorf("api: sweep branch deploys: list: %w", listErr)
	}
	now := time.Now().UTC()
	ttl := branchDeployTTL()
	var errs []error
	for _, b := range all {
		updated, perr := time.Parse(time.RFC3339Nano, b.UpdatedAt)
		if perr != nil || !now.After(updated.Add(ttl)) {
			continue
		}
		status, message := rt.teardownBranchDeploy(ctx, b)
		if status >= http.StatusInternalServerError {
			errs = append(errs, fmt.Errorf("branch deploy %q (app %q): %s", b.ID, b.AppName, message))
			continue
		}
		swept++
	}
	return swept, errors.Join(errs...)
}
