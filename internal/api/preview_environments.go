package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// PreviewEnvironmentStore is the store surface preview environment
// webhook handling and its HTTP routes (preview_environments_handlers.go)
// need. *store.DB satisfies this structurally.
type PreviewEnvironmentStore interface {
	SavePreviewEnvironment(ctx context.Context, p store.PreviewEnvironment) error
	UpdatePreviewEnvironment(ctx context.Context, p store.PreviewEnvironment) error
	GetPreviewEnvironmentByAppAndPR(ctx context.Context, appName string, prNumber int) (*store.PreviewEnvironment, error)
	ListPreviewEnvironmentsByApp(ctx context.Context, appName string) ([]store.PreviewEnvironment, error)
	DeletePreviewEnvironment(ctx context.Context, id string) error
	ListStalePreviewEnvironments(ctx context.Context, cutoff time.Time) ([]store.PreviewEnvironment, error)
	ListPreviewEnvironments(ctx context.Context) ([]store.PreviewEnvironment, error)
	SetPreviewEnvironmentCommentID(ctx context.Context, id string, commentID int64) error
	GetPreviewAppSettings(ctx context.Context, appName string) (store.PreviewAppSettings, error)
	ListPreviewAppSettings(ctx context.Context) (map[string]store.PreviewAppSettings, error)
	SavePreviewAppSettings(ctx context.Context, s store.PreviewAppSettings) error

	// SavePreviewEphemeralDatabase through DeletePreviewEphemeralDatabase
	// back the ephemeralInPreviews lifecycle
	// (preview_environments_databases.go): provisioning a preview-scoped
	// database instance, listing them for the API's own status surface,
	// and tearing them down alongside the preview that owns them.
	SavePreviewEphemeralDatabase(ctx context.Context, p store.PreviewEphemeralDatabase) error
	GetPreviewEphemeralDatabaseByPreviewAndKey(ctx context.Context, previewEnvironmentID, sourceKey string) (*store.PreviewEphemeralDatabase, error)
	ListPreviewEphemeralDatabasesByPreview(ctx context.Context, previewEnvironmentID string) ([]store.PreviewEphemeralDatabase, error)
	UpdatePreviewEphemeralDatabaseStatus(ctx context.Context, id, status, statusReason, updatedAt string) error
	DeletePreviewEphemeralDatabase(ctx context.Context, id string) error
}

// previewAppName is the naming scheme every preview deploys under. Every
// service inside it (a plain single service, or a "<previewAppName>-<key>"
// fan-out per DeploySpec's own convention) derives from this one name, so
// it's collision-free the same way store.App.Name's own UNIQUE
// constraint already guarantees for every other app.
func previewAppName(appName string, prNumber int) string {
	return fmt.Sprintf("%s-pr-%d", appName, prNumber)
}

// handlePullRequestWebhookEvent is handleGitPushWebhook's pull-request
// counterpart: routes an opened/synchronize event to
// deployPreviewEnvironment and a closed event (merged or not, same
// teardown either way) to teardownPullRequestPreview. gs.PreviewEnabled
// gates the whole thing, off by default: a GitLab, Bitbucket, or Gitea
// source's webhook is registered with merge-request/pull-request events
// unconditionally (gitlabapp.CreateProjectWebhook,
// bitbucketapp.CreateRepoWebhook, giteaapp.CreateRepoWebhook), regardless
// of whether the app has opted into previews, so this check is what
// actually makes the feature opt-in for those providers.
func (rt *Router) handlePullRequestWebhookEvent(ctx context.Context, appName string, gs store.GitSource, ev webhook.PullRequestEvent) (status int, message string) {
	if !gs.PreviewEnabled {
		return http.StatusOK, fmt.Sprintf("ignored: preview environments are not enabled for %q\n", appName)
	}

	targetBranch := gs.Branch
	if targetBranch == "" {
		targetBranch = webhook.DefaultBranch
	}
	if ev.BaseRef != "" && ev.BaseRef != targetBranch {
		return http.StatusOK, fmt.Sprintf("ignored: pull request #%d targets %q, previews trigger on %q only\n", ev.Number, ev.BaseRef, targetBranch)
	}

	if ev.Action == webhook.PullRequestClosed {
		return rt.teardownPullRequestPreview(ctx, appName, ev.Number)
	}

	if rt.builder == nil {
		return http.StatusNotImplemented, "git push deploys are not configured on this control plane\n"
	}
	return rt.deployPreviewEnvironment(ctx, appName, gs, ev, false)
}

// deployPreviewEnvironment gates a pull request event (fork approval,
// concurrency cap), then deploys the PR's head commit under previewAppName
// and records the outcome. A failure at any step marks the row Failed with a
// reason rather than leaving it stuck Deploying; a domain collision
// specifically does not fail the whole preview (see deployPreviewSingle),
// since a reachable preview with no vanity domain is more useful than no
// preview at all. approved lets one operator-approved fork PR through the
// fork gate.
func (rt *Router) deployPreviewEnvironment(ctx context.Context, appName string, gs store.GitSource, ev webhook.PullRequestEvent, approved bool) (int, string) {
	previewName := previewAppName(appName, ev.Number)

	preview, done, status, message := rt.beginPreview(ctx, appName, gs, ev, approved)
	if done {
		return status, message
	}

	rt.notifyPreviewPending(ctx, appName, gs, ev.HeadSHA)
	rt.upsertPreviewComment(ctx, gs, preview, previewCommentBuilding, "")

	token, err := rt.resolveGitSourceDeployToken(ctx, appName)
	if err != nil {
		rt.logger.Error("api: pull request webhook: resolve deploy token failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		rt.finishPreviewFailed(ctx, gs, preview, "resolve deploy token: "+err.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}

	sourceDir, cleanup, err := rt.gitSourceFetch(ctx, gs.RepoURL, ev.HeadSHA, token)
	if err != nil {
		rt.logger.Error("api: pull request webhook: fetch source failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", ev.Number))
		rt.finishPreviewFailed(ctx, gs, preview, "fetch source: "+err.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}
	defer cleanup()

	wantDomain := rt.previewDomain(ctx, appName, ev.Number)
	vars := previewEnvVars{PRNumber: ev.Number, Branch: ev.HeadRef}

	var (
		usedDomain     string
		domainConflict bool
		deployErr      error
	)
	if len(gs.Services) > 0 {
		usedDomain, domainConflict, deployErr = rt.deployPreviewMulti(ctx, previewName, gs, sourceDir, ev.HeadSHA, wantDomain, vars)
	} else {
		usedDomain, domainConflict, deployErr = rt.deployPreviewSingle(ctx, appName, previewName, gs, sourceDir, ev.HeadSHA, wantDomain, vars)
	}
	if deployErr != nil {
		rt.logger.Error("api: pull request webhook: deploy failed", slog.String("error", deployErr.Error()), slog.String("app_name", appName), slog.Int("pr_number", ev.Number))
		rt.finishPreviewFailed(ctx, gs, preview, deployErr.Error())
		return http.StatusInternalServerError, "deploy failed\n"
	}

	if len(gs.Databases) > 0 {
		provisioned := rt.provisionEphemeralDatabases(ctx, preview.ID, previewName, gs.Databases)
		if len(gs.Services) == 0 && len(provisioned) == 1 {
			rt.attachEphemeralDatabase(ctx, previewName, provisioned[0])
		}
	}

	if _, environmentID, envErr := rt.ensurePreviewEnvironmentTier(ctx, appName); envErr != nil {
		rt.logger.Error("api: pull request webhook: ensure preview environment tier failed", slog.String("error", envErr.Error()), slog.String("app_name", appName))
	} else if tagErr := rt.tagPreviewWithEnvironment(ctx, previewName, gs, environmentID); tagErr != nil {
		rt.logger.Error("api: pull request webhook: tag preview environment failed", slog.String("error", tagErr.Error()), slog.String("app_name", appName), slog.Int("pr_number", ev.Number))
	} else {
		preview.EnvironmentID = environmentID
	}

	preview.Status, preview.Domain, preview.StatusReason = store.PreviewStatusActive, usedDomain, ""
	statusCode := http.StatusOK
	if domainConflict {
		preview.StatusReason = fmt.Sprintf("domain %q was already in use, preview deployed without a domain", wantDomain)
		statusCode = http.StatusMultiStatus
	}
	preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		rt.logger.Error("api: pull request webhook: finalize preview failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", ev.Number))
		return http.StatusInternalServerError, "internal error\n"
	}

	rt.notifyPreviewSuccess(ctx, appName, gs, ev.HeadSHA, usedDomain)
	rt.upsertPreviewComment(ctx, gs, preview, previewCommentReady, preview.StatusReason)

	rt.logger.Info("api: pull request webhook: preview deployed", slog.String("app_name", appName), slog.Int("pr_number", ev.Number), slog.String("preview_app", previewName))
	return statusCode, fmt.Sprintf("preview deployed: %s\n", previewName)
}

// finishPreviewFailed marks preview Failed with reason, logged but not
// itself returned as an error: called only from paths that are already
// about to report a failure to the webhook caller. Also posts a failure
// commit status and updates the pull request's status comment, gs being the
// same connected git source every caller already has in scope.
func (rt *Router) finishPreviewFailed(ctx context.Context, gs store.GitSource, preview *store.PreviewEnvironment, reason string) {
	preview.Status, preview.StatusReason = store.PreviewStatusFailed, reason
	preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		rt.logger.Error("api: pull request webhook: mark preview failed failed", slog.String("error", err.Error()), slog.String("preview_id", preview.ID))
	}
	rt.notifyPreviewFailure(ctx, preview.AppName, gs, preview.HeadSHA, reason)
	rt.upsertPreviewComment(ctx, gs, preview, previewCommentFailed, reason)
}

// previewEnvVars is what a preview deploy injects into every container it
// starts, so the app can know where it is running.
type previewEnvVars struct {
	PRNumber int
	Branch   string
}

// injectPreviewEnv sets PREVIEW_PR_NUMBER and PREVIEW_BRANCH, plus
// PREVIEW_URL when domain is known, on a copy of env.
func injectPreviewEnv(env map[string]spec.EnvVar, vars previewEnvVars, domain string) map[string]spec.EnvVar {
	out := make(map[string]spec.EnvVar, len(env)+3)
	for k, v := range env {
		out[k] = v
	}
	out["PREVIEW_PR_NUMBER"] = spec.EnvVar{Value: strconv.Itoa(vars.PRNumber)}
	out["PREVIEW_BRANCH"] = spec.EnvVar{Value: vars.Branch}
	if domain != "" {
		out["PREVIEW_URL"] = spec.EnvVar{Value: "https://" + domain}
	} else {
		delete(out, "PREVIEW_URL")
	}
	return out
}

// domainSlice returns nil for an empty domain, rather than a
// one-element slice holding "": spec.Service.Domains treats absence and
// an empty string differently downstream (store.DesiredService's own
// domain-uniqueness enforcement), so this keeps "no domain" genuinely
// absent instead of an empty-string domain no host could ever match.
func domainSlice(domain string) []string {
	if domain == "" {
		return nil
	}
	return []string{domain}
}

// applyPreviewEnvOverrides replaces svcSpec.Env, for every key present in
// overrides, with a plain literal value: a preview-specific override
// (PUT /api/v1/apps/{name}/preview-env/{key} on the parent app) always
// wins over whatever the parent's own Env/SecretEnv/VaultEnv held for
// that key, since the whole point is a preview-only value the parent
// itself never sees. Every other env var not named in overrides still
// inherits from the parent exactly as specServiceFromDesired already
// copied it.
func applyPreviewEnvOverrides(svcSpec *spec.Service, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	if svcSpec.Env == nil {
		svcSpec.Env = make(map[string]spec.EnvVar, len(overrides))
	}
	for k, v := range overrides {
		svcSpec.Env[k] = spec.EnvVar{Value: v}
	}
}

// deployPreviewSingle rebuilds appName's own production build config
// (specServiceFromDesired, the same reconstruction handleGitPushWebhook
// already uses) against the PR's checkout, under previewName. A domain
// collision (store.ErrDomainTaken) does not fail the deploy: it retries
// once with no domain at all, since a reachable preview under
// host:port is strictly more useful than no preview, and returns
// domainConflict=true so the caller can record why the preview has no
// domain instead of silently swallowing it.
func (rt *Router) deployPreviewSingle(ctx context.Context, appName, previewName string, gs store.GitSource, sourceDir, headSHA, wantDomain string, vars previewEnvVars) (usedDomain string, domainConflict bool, err error) {
	prod, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil {
		return "", false, fmt.Errorf("load production service %q: %w", appName, err)
	}

	svcSpec := specServiceFromDesired(*prod, spec.Build{Type: gs.BuildType, Path: gs.BuildPath})
	applyPreviewEnvOverrides(&svcSpec, prod.PreviewEnvOverrides)
	branchOverrides, err := rt.apps.ListServiceBranchEnvOverrides(ctx, appName)
	if err != nil {
		return "", false, fmt.Errorf("load branch env overrides for %q: %w", appName, err)
	}
	if err := applyBranchEnvOverrides(ctx, &svcSpec, appName, vars.Branch, branchOverrides, rt.secrets); err != nil {
		return "", false, fmt.Errorf("apply branch env overrides for %q: %w", appName, err)
	}
	svcSpec.Domains = domainSlice(wantDomain)
	svcSpec.Env = injectPreviewEnv(svcSpec.Env, vars, wantDomain)
	req := deploy.Request{ServiceName: previewName, Service: svcSpec, SourceDir: sourceDir, CommitSHA: headSHA, ImageRepo: previewName}

	_, deployErr := rt.builder.Deploy(ctx, req, build.SlogProgress(rt.logger))
	usedDomain = wantDomain
	var domainTaken *store.ErrDomainTaken
	if deployErr != nil && wantDomain != "" && errors.As(deployErr, &domainTaken) {
		req.Service.Domains = nil
		req.Service.Env = injectPreviewEnv(req.Service.Env, vars, "")
		if _, retryErr := rt.builder.Deploy(ctx, req, build.SlogProgress(rt.logger)); retryErr != nil {
			return "", false, fmt.Errorf("deploy without domain after domain conflict: %w", retryErr)
		}
		usedDomain, domainConflict, deployErr = "", true, nil
	}
	if deployErr != nil {
		return "", false, deployErr
	}

	if _, linkErr := rt.ensureAppLinked(ctx, previewName, previewName); linkErr != nil {
		return "", false, fmt.Errorf("link preview to app: %w", linkErr)
	}
	return usedDomain, domainConflict, nil
}

// deployPreviewMulti fans out gs.Services (an app.yaml-style map) under
// previewName via the same deploy.Pipeline.DeploySpec fan-out a manual
// multi-service deploy uses. Only one service (previewPrimaryService's
// own pick) ever carries the preview domain, to avoid every fanned-out
// service claiming the same subdomain; the rest deploy with no domain,
// reachable only on the internal network the way a worker/db-adjacent
// service already is in production. Unlike the single-service path,
// this does not retry a domain-carrying service without its domain on a
// collision: DeploySpec's own per-service ServiceOutcome.Err already
// reports that service's failure independently of its siblings, which
// this treats as a domain-conflict-shaped partial success (the rest of
// the preview is still reachable) rather than special-casing the retry.
func (rt *Router) deployPreviewMulti(ctx context.Context, previewName string, gs store.GitSource, sourceDir, headSHA, wantDomain string, vars previewEnvVars) (usedDomain string, domainConflict bool, err error) {
	services, primaryKey := previewServicesSpec(gs.Services, wantDomain)
	for k, svc := range services {
		domain := ""
		if len(svc.Domains) > 0 {
			domain = svc.Domains[0]
		}
		svc.Env = injectPreviewEnv(svc.Env, vars, domain)
		services[k] = svc
	}

	outcomes, err := rt.builder.DeploySpec(ctx, deploy.MultiRequest{
		AppName: previewName, Services: services, SourceDir: sourceDir, CommitSHA: headSHA, ImageRepoBase: previewName,
	}, nil)
	if err != nil {
		return "", false, err
	}

	var failed []string
	primaryFailed := false
	for _, o := range outcomes {
		if o.Err == nil {
			continue
		}
		failed = append(failed, o.ServiceKey)
		if o.ServiceKey == primaryKey {
			primaryFailed = true
		}
	}
	if len(failed) == len(outcomes) {
		return "", false, fmt.Errorf("every service in the preview fan-out failed: %s", strings.Join(failed, ", "))
	}
	if len(failed) == 0 {
		return wantDomain, false, nil
	}
	if primaryKey == "" || primaryFailed {
		return "", true, nil
	}
	return wantDomain, true, nil
}

// previewServicesSpec deep-copies services for a preview deploy: every
// entry's own production Domains are cleared (a preview must never
// silently claim a production domain), and domain, if non-empty, is
// assigned to exactly one service, the first (sorted, for determinism)
// key that had a domain of its own in production. Returns "" for
// primaryKey when no service had a production domain, or domain itself
// is empty: neither case has anything meaningful to assign.
func previewServicesSpec(services map[string]spec.Service, domain string) (map[string]spec.Service, string) {
	keys := make([]string, 0, len(services))
	for k := range services {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]spec.Service, len(services))
	primaryKey := ""
	for _, k := range keys {
		svc := services[k]
		hadDomain := len(svc.Domains) > 0
		svc.Domains = nil
		if hadDomain && primaryKey == "" && domain != "" {
			svc.Domains = []string{domain}
			primaryKey = k
		}
		out[k] = svc
	}
	return out, primaryKey
}

// previewDomain returns the subdomain a preview should request:
// "pr-<number>.<appName>.<primary domain>" when a control-plane-wide
// primary domain is configured (ingress_settings.primary_domain, the
// same base-domain concept SetPrimaryDomainPrompt already reuses for
// every git provider's connect flow), or "" otherwise, meaning the
// preview is reachable by host:port only, the same fallback every other
// domain-less app already has.
func (rt *Router) previewDomain(ctx context.Context, appName string, prNumber int) string {
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.logger.Error("api: preview domain: load ingress settings failed", slog.String("error", err.Error()))
		return ""
	}
	if settings.PrimaryDomain == "" {
		return ""
	}
	return fmt.Sprintf("pr-%d.%s.%s", prNumber, appName, settings.PrimaryDomain)
}

// ensurePreviewEnvironmentTier returns appName's dedicated preview
// project/environment pair, creating either or both if this is the
// first preview ever deployed for appName. Deterministic IDs
// ("preview-<appName>"/"preview-env-<appName>"), not randomly minted
// ones: every app has at most one preview project/environment pair by
// construction, so a lookup never needs a separate index, and creating
// it is naturally idempotent (GetProject/GetEnvironment already tell us
// whether it exists).
//
// One dedicated Environment per app, not one per PR: an Environment's
// only real behavior in this codebase is its shared env-var tier
// (SetEnvironmentEnvVars), which is inherently a per-tier concern (a
// shared preview database URL, a shared feature-flag key), not a
// per-instance one; a PR's own identity (branch, SHA, PR number)
// already lives on its own preview_environments row and its own preview
// service name, so a dedicated Environment per PR would only add N
// throwaway rows an operator would never have a reason to configure
// individually.
func (rt *Router) ensurePreviewEnvironmentTier(ctx context.Context, appName string) (projectID, environmentID string, err error) {
	projectID = "preview-" + appName
	if _, getErr := rt.projects.GetProject(ctx, projectID); errors.Is(getErr, store.ErrProjectNotFound) {
		if saveErr := rt.projects.SaveProject(ctx, store.Project{ID: projectID, Name: appName + " previews", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); saveErr != nil {
			return "", "", fmt.Errorf("ensure preview project: %w", saveErr)
		}
	} else if getErr != nil {
		return "", "", fmt.Errorf("load preview project: %w", getErr)
	}

	environmentID = "preview-env-" + appName
	if _, getErr := rt.environments.GetEnvironment(ctx, environmentID); errors.Is(getErr, store.ErrEnvironmentNotFound) {
		if saveErr := rt.environments.SaveEnvironment(ctx, store.Environment{ID: environmentID, ProjectID: projectID, Name: "Preview", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}); saveErr != nil {
			return "", "", fmt.Errorf("ensure preview environment: %w", saveErr)
		}
	} else if getErr != nil {
		return "", "", fmt.Errorf("load preview environment: %w", getErr)
	}
	return projectID, environmentID, nil
}

// tagPreviewWithEnvironment tags every service a preview deployed
// (previewName itself for a single-service preview, or every sibling
// under previewName's own store.App for a fan-out one) with
// environmentID.
func (rt *Router) tagPreviewWithEnvironment(ctx context.Context, previewName string, gs store.GitSource, environmentID string) error {
	if len(gs.Services) == 0 {
		return rt.environments.SetServiceEnvironment(ctx, previewName, environmentID)
	}

	app, err := rt.appGroups.GetAppByName(ctx, previewName)
	if err != nil {
		return fmt.Errorf("load preview app: %w", err)
	}
	services, err := rt.appGroups.ListServicesByApp(ctx, app.ID)
	if err != nil {
		return fmt.Errorf("list preview services: %w", err)
	}
	for _, svc := range services {
		if err := rt.environments.SetServiceEnvironment(ctx, svc.Name, environmentID); err != nil {
			return fmt.Errorf("tag %q: %w", svc.Name, err)
		}
	}
	return nil
}
