package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Environments a deployment is reported under.
const (
	forgeEnvProduction = "production"
	forgeEnvPreview    = "preview"
)

// previewScope keys one pull request's preview deployments, so a newer
// preview only supersedes deployments of the same pull request.
func previewScope(prNumber int) string { return fmt.Sprintf("%s:%d", forgeEnvPreview, prNumber) }

// ForgeDeploymentStore persists the deployments created on a git forge.
// *store.DB satisfies it.
type ForgeDeploymentStore interface {
	CreateForgeDeployment(ctx context.Context, d store.ForgeDeployment) error
	SetForgeDeploymentState(ctx context.Context, id, state, warning string, at time.Time) error
	ListForgeDeploymentsByState(ctx context.Context, app, environment, state, excludeID string) ([]store.ForgeDeployment, error)
}

// SetForgeDeployments wires the store that tracks forge deployments. Unset,
// deployments are not reported.
func (rt *Router) SetForgeDeployments(s ForgeDeploymentStore) { rt.forgeDeployments = s }

// forgeDeployment reports one app deploy to the forge's Deployments API. A
// nil *forgeDeployment is valid and does nothing, so callers never branch on
// whether reporting is on.
type forgeDeployment struct {
	rt     *Router
	f      *forge
	rec    store.ForgeDeployment
	logURL string
	envURL string
}

// beginForgeDeployment creates a deployment for sha and marks it in_progress.
// environment is the GitHub environment name and scope keys which earlier
// deployments this one supersedes. It returns nil when reporting is off, the
// app is not on GitHub, or the forge rejects the call; a rejection is logged,
// recorded as an error row with its warning, and never blocks the deploy.
func (rt *Router) beginForgeDeployment(ctx context.Context, app string, gs store.GitSource, sha, environment, scope, envURL string) *forgeDeployment {
	if !gitStatusEnabled() || !gs.ReportStatus || rt.forgeDeployments == nil || sha == "" {
		return nil
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if err != nil {
		return nil
	}
	switch f.kind {
	case forgeGitHub:
		return rt.beginOnForge(ctx, f, app, sha, environment, scope, envURL)
	case forgeGitLab:
		return rt.beginOnGitLab(ctx, f, gs, app, sha, environment, scope, envURL)
	}
	return nil
}

func (rt *Router) beginOnForge(ctx context.Context, f *forge, app, sha, environment, scope, envURL string) *forgeDeployment {
	owner, repo := f.ownerName()
	production := environment == forgeEnvProduction
	id, err := f.github.CreateDeployment(ctx, f.instanceURL, f.token, owner, repo, sha, environment, "Deploy "+app, production)
	now := time.Now().UTC()
	if err != nil {
		warning := "deployment not created: " + describeForgeError(err)
		rt.logger.Warn("api: create forge deployment failed", slog.String("app_name", app), slog.String("environment", environment), slog.String("error", describeForgeError(err)))
		failed := store.ForgeDeployment{ID: newForgeDeploymentID(), AppName: app, Environment: scope, Provider: f.kind, CommitSHA: sha, State: "error", Warning: warning, CreatedAt: now, UpdatedAt: now}
		if serr := rt.forgeDeployments.CreateForgeDeployment(ctx, failed); serr != nil {
			rt.logger.Warn("api: record failed forge deployment failed", slog.String("app_name", app), slog.String("error", serr.Error()))
		}
		return nil
	}
	d := &forgeDeployment{rt: rt, f: f, envURL: envURL, logURL: rt.dashboardLink(ctx, "/apps/"+app+"/deployments"), rec: store.ForgeDeployment{
		ID: newForgeDeploymentID(), AppName: app, Environment: scope, Provider: f.kind, ExternalID: id,
		CommitSHA: sha, State: string(githubapp.DeploymentInProgress), CreatedAt: now, UpdatedAt: now,
	}}
	if err := rt.forgeDeployments.CreateForgeDeployment(ctx, d.rec); err != nil {
		rt.logger.Warn("api: record forge deployment failed", slog.String("app_name", app), slog.String("error", err.Error()))
	}
	d.post(ctx, githubapp.DeploymentInProgress, "Deploying "+app)
	return d
}

// deploymentStateFor maps a webhook deploy outcome to a deployment state: a
// superseded deploy never went live, so it is reported inactive, and a
// multi-status (some services failed) is not a successful release.
func deploymentStateFor(status int, message string) githubapp.DeploymentState {
	switch {
	case strings.HasPrefix(message, "not deployed"):
		return githubapp.DeploymentInactive
	case status >= http.StatusBadRequest || status == http.StatusMultiStatus:
		return githubapp.DeploymentFailure
	}
	return githubapp.DeploymentSuccess
}

func newForgeDeploymentID() string {
	buf := make([]byte, 9)
	_, _ = rand.Read(buf)
	return "fdep_" + hex.EncodeToString(buf)
}

func (d *forgeDeployment) post(ctx context.Context, state githubapp.DeploymentState, description string) string {
	if d.f.kind == forgeGitLab {
		return d.postGitLab(ctx, state, description)
	}
	owner, repo := d.f.ownerName()
	url := ""
	if state == githubapp.DeploymentSuccess {
		url = d.envURL
	}
	if err := d.f.github.CreateDeploymentStatus(ctx, d.f.instanceURL, d.f.token, owner, repo, d.rec.ExternalID, state, url, d.logURL, truncateStatusDescription(description, githubStatusDescriptionMax)); err != nil {
		warning := fmt.Sprintf("deployment status %s not reported: %s", state, describeForgeError(err))
		d.rt.logger.Warn("api: post forge deployment status failed", slog.String("app_name", d.rec.AppName), slog.String("state", string(state)), slog.String("error", describeForgeError(err)))
		return warning
	}
	return ""
}

// supersedeSelf marks this deployment inactive because a newer one of the same
// scope already went live, so finishing late never displaces the newer one.
func (d *forgeDeployment) supersedeSelf(ctx context.Context, at time.Time) {
	if d.rec.State == string(githubapp.DeploymentInactive) {
		return
	}
	warning := ""
	if err := d.f.retireDeployment(ctx, d.rec.ExternalID, "Superseded"); err != nil {
		warning = fmt.Sprintf("deployment status inactive not reported: %s", describeForgeError(err))
	}
	d.rec.State = string(githubapp.DeploymentInactive)
	if err := d.rt.forgeDeployments.SetForgeDeploymentState(ctx, d.rec.ID, d.rec.State, warning, at); err != nil {
		d.rt.logger.Warn("api: update superseded forge deployment failed", slog.String("app_name", d.rec.AppName), slog.String("error", err.Error()))
	}
}

// deactivateForgeDeployments marks every live deployment of scope inactive,
// used when a pull request closes and its preview is torn down.
func (rt *Router) deactivateForgeDeployments(ctx context.Context, app string, gs store.GitSource, scope string) {
	if !gitStatusEnabled() || !gs.ReportStatus || rt.forgeDeployments == nil {
		return
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if err != nil {
		return
	}
	switch f.kind {
	case forgeGitHub:
		rt.deactivateLive(ctx, f, app, scope)
	case forgeGitLab:
		rt.stopGitLabEnvironment(ctx, f, gs, app, scope)
	}
}

func (rt *Router) deactivateLive(ctx context.Context, f *forge, app, scope string) {
	live, err := rt.forgeDeployments.ListForgeDeploymentsByState(ctx, app, scope, string(githubapp.DeploymentSuccess), "")
	if err != nil {
		rt.logger.Warn("api: list forge deployments to deactivate failed", slog.String("app_name", app), slog.String("error", err.Error()))
		return
	}
	now := time.Now().UTC()
	for _, o := range live {
		w := ""
		if err := f.retireDeployment(ctx, o.ExternalID, "Preview removed"); err != nil {
			w = fmt.Sprintf("deployment status inactive not reported: %s", describeForgeError(err))
			rt.logger.Warn("api: mark removed preview deployment inactive failed", slog.String("app_name", app), slog.String("error", describeForgeError(err)))
		}
		if err := rt.forgeDeployments.SetForgeDeploymentState(ctx, o.ID, string(githubapp.DeploymentInactive), w, now); err != nil {
			rt.logger.Warn("api: update removed preview deployment failed", slog.String("app_name", app), slog.String("error", err.Error()))
		}
	}
}

// finish moves the deployment to its final state. On success every earlier
// successful deployment of the same scope is marked inactive.
func (d *forgeDeployment) finish(ctx context.Context, state githubapp.DeploymentState, description string) {
	if d == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	warning := d.post(ctx, state, description)
	now := time.Now().UTC()
	if err := d.rt.forgeDeployments.SetForgeDeploymentState(ctx, d.rec.ID, string(state), warning, now); err != nil {
		d.rt.logger.Warn("api: update forge deployment failed", slog.String("app_name", d.rec.AppName), slog.String("error", err.Error()))
	}
	if state != githubapp.DeploymentSuccess {
		return
	}
	older, err := d.rt.forgeDeployments.ListForgeDeploymentsByState(ctx, d.rec.AppName, d.rec.Environment, string(githubapp.DeploymentSuccess), d.rec.ID)
	if err != nil {
		d.rt.logger.Warn("api: list superseded forge deployments failed", slog.String("app_name", d.rec.AppName), slog.String("error", err.Error()))
		return
	}
	for _, o := range older {
		if !o.CreatedAt.Before(d.rec.CreatedAt) {
			d.supersedeSelf(ctx, now)
			continue
		}
		w := ""
		if err := d.f.retireDeployment(ctx, o.ExternalID, "Superseded"); err != nil {
			w = fmt.Sprintf("deployment status inactive not reported: %s", describeForgeError(err))
			d.rt.logger.Warn("api: mark superseded forge deployment inactive failed", slog.String("app_name", o.AppName), slog.String("error", describeForgeError(err)))
		}
		if err := d.rt.forgeDeployments.SetForgeDeploymentState(ctx, o.ID, string(githubapp.DeploymentInactive), w, now); err != nil && !errors.Is(err, context.Canceled) {
			d.rt.logger.Warn("api: update superseded forge deployment failed", slog.String("app_name", o.AppName), slog.String("error", err.Error()))
		}
	}
}

// retireDeployment marks an earlier deployment inactive on the forge. GitLab
// keeps only the newest deployment current by itself, so it needs no call.
func (f *forge) retireDeployment(ctx context.Context, externalID int64, description string) error {
	if f.kind == forgeGitLab {
		return nil
	}
	owner, repo := f.ownerName()
	return f.github.CreateDeploymentStatus(ctx, f.instanceURL, f.token, owner, repo, externalID, githubapp.DeploymentInactive, "", "", description)
}

// forgeStatusSourceDeployment tags the status rows written for deployment posts.
const forgeStatusSourceDeployment = "deployment"

// gitlabDeploymentContext names the forge status rows of deployment posts.
const gitlabDeploymentContext = "levelrail/deployment"

// gitlabDeploymentClient is the optional GitLab surface deployment reporting
// needs beyond commit statuses.
type gitlabDeploymentClient interface {
	EnsureEnvironment(ctx context.Context, instanceURL, accessToken, projectPath, name, externalURL, tier string) (gitlabapp.Environment, error)
	SetEnvironmentURL(ctx context.Context, instanceURL, accessToken, projectPath string, envID int64, externalURL string) error
	StopEnvironment(ctx context.Context, instanceURL, accessToken, projectPath, name string) error
	CreateDeployment(ctx context.Context, instanceURL, accessToken, projectPath, environment, sha, ref string, tag bool, status gitlabapp.DeploymentStatus) (int64, error)
	UpdateDeploymentStatus(ctx context.Context, instanceURL, accessToken, projectPath string, id int64, status gitlabapp.DeploymentStatus) error
}

// gitlabEnvironmentName gives each app environment its own GitLab environment,
// grouped under the app so two apps in one project never share one.
func gitlabEnvironmentName(app, scope string) string {
	return app + "/" + strings.ReplaceAll(scope, ":", "-")
}

func gitlabEnvironmentTier(scope string) string {
	if scope == forgeEnvProduction {
		return "production"
	}
	return "development"
}

func gitlabDeploymentStatus(state githubapp.DeploymentState) gitlabapp.DeploymentStatus {
	switch state {
	case githubapp.DeploymentSuccess:
		return gitlabapp.DeploymentSuccess
	case githubapp.DeploymentFailure:
		return gitlabapp.DeploymentFailed
	case githubapp.DeploymentInactive:
		return gitlabapp.DeploymentCanceled
	}
	return gitlabapp.DeploymentRunning
}

// gitlabDeploymentsAllowed is the live off switch: reporting is off by
// default, can be limited to chosen connections, and never runs when the
// connection's stored scopes show the deployments_report capability needs a
// scope the token lacks.
func (rt *Router) gitlabDeploymentsAllowed(ctx context.Context, f *forge, gs store.GitSource) (gitlabDeploymentClient, bool) {
	dc, ok := f.gitlab.(gitlabDeploymentClient)
	if !ok || rt.runtimeSettings == nil || !rt.runtimeSettings.Bool(ctx, runtimesettings.KeyGitLabDeployments) {
		return nil, false
	}
	conn, err := rt.gitConnForRepo(ctx, gitProviderGitLab, gs.RepoURL, gs.ConnectionID)
	if err != nil {
		return nil, false
	}
	if only := rt.runtimeSettings.List(ctx, runtimesettings.KeyGitLabDeploymentsConnections); len(only) > 0 && !slices.Contains(only, conn.ID) {
		return nil, false
	}
	if scopes := strings.Fields(strings.ReplaceAll(conn.Scopes, ",", " ")); len(scopes) > 0 {
		for _, c := range gitlabCapabilities(scopes, false) {
			if c.ID == capDeploymentsReprt && c.State == gitCapNeedsScope {
				return nil, false
			}
		}
	}
	return dc, true
}

// recordGitLabDeploymentPost keeps one deployment post in the forge status
// storage, with the failure text when the call did not go through.
func (rt *Router) recordGitLabDeploymentPost(ctx context.Context, app, sha, state, target, description string, postErr error) {
	rt.recordForgeStatus(ctx, store.ForgeStatusReport{
		AppName: app, Provider: forgeGitLab, SHA: sha, Context: gitlabDeploymentContext, State: state,
		Description: description, TargetURL: target, Source: forgeStatusSourceDeployment,
	}, postErr)
}

func reportStateFor(state githubapp.DeploymentState) string {
	switch state {
	case githubapp.DeploymentSuccess:
		return pipeline.ReportSuccess
	case githubapp.DeploymentFailure:
		return pipeline.ReportFailure
	case githubapp.DeploymentInactive:
		return pipeline.ReportError
	}
	return pipeline.ReportPending
}

func (rt *Router) beginOnGitLab(ctx context.Context, f *forge, gs store.GitSource, app, sha, environment, scope, envURL string) *forgeDeployment {
	dc, ok := rt.gitlabDeploymentsAllowed(ctx, f, gs)
	if !ok {
		return nil
	}
	logURL := rt.dashboardLink(ctx, "/apps/"+app+"/deployments")
	target := envURL
	if target == "" {
		target = logURL
	}
	name := gitlabEnvironmentName(app, scope)
	now := time.Now().UTC()
	env, err := dc.EnsureEnvironment(ctx, f.instanceURL, f.token, f.repo, name, target, gitlabEnvironmentTier(environment))
	var id int64
	if err == nil {
		if target != "" && env.ExternalURL != target {
			if uerr := dc.SetEnvironmentURL(ctx, f.instanceURL, f.token, f.repo, env.ID, target); uerr != nil {
				rt.logger.Warn("api: set gitlab environment url failed", slog.String("app_name", app), slog.String("error", describeForgeError(uerr)))
			}
		}
		id, err = dc.CreateDeployment(ctx, f.instanceURL, f.token, f.repo, name, sha, sha, false, gitlabapp.DeploymentRunning)
	}
	if err != nil {
		warning := "deployment not created: " + describeForgeError(err)
		rt.logger.Warn("api: create gitlab deployment failed", slog.String("app_name", app), slog.String("environment", name), slog.String("error", describeForgeError(err)))
		rt.recordGitLabDeploymentPost(ctx, app, sha, pipeline.ReportError, target, "Deployment not created", err)
		failed := store.ForgeDeployment{ID: newForgeDeploymentID(), AppName: app, Environment: scope, Provider: f.kind, CommitSHA: sha, State: "error", Warning: warning, CreatedAt: now, UpdatedAt: now}
		if serr := rt.forgeDeployments.CreateForgeDeployment(ctx, failed); serr != nil {
			rt.logger.Warn("api: record failed forge deployment failed", slog.String("app_name", app), slog.String("error", serr.Error()))
		}
		return nil
	}
	d := &forgeDeployment{rt: rt, f: f, envURL: envURL, logURL: logURL, rec: store.ForgeDeployment{
		ID: newForgeDeploymentID(), AppName: app, Environment: scope, Provider: f.kind, ExternalID: id,
		CommitSHA: sha, State: string(githubapp.DeploymentInProgress), CreatedAt: now, UpdatedAt: now,
	}}
	if err := rt.forgeDeployments.CreateForgeDeployment(ctx, d.rec); err != nil {
		rt.logger.Warn("api: record forge deployment failed", slog.String("app_name", app), slog.String("error", err.Error()))
	}
	rt.recordGitLabDeploymentPost(ctx, app, sha, pipeline.ReportPending, target, "Deploying "+app, nil)
	return d
}

func (d *forgeDeployment) postGitLab(ctx context.Context, state githubapp.DeploymentState, description string) string {
	dc, ok := d.f.gitlab.(gitlabDeploymentClient)
	if !ok {
		return ""
	}
	err := dc.UpdateDeploymentStatus(ctx, d.f.instanceURL, d.f.token, d.f.repo, d.rec.ExternalID, gitlabDeploymentStatus(state))
	target := d.envURL
	if target == "" {
		target = d.logURL
	}
	d.rt.recordGitLabDeploymentPost(ctx, d.rec.AppName, d.rec.CommitSHA, reportStateFor(state), target, truncateStatusDescription(description, gitLabStatusDescriptionMax), err)
	if err != nil {
		d.rt.logger.Warn("api: post gitlab deployment status failed", slog.String("app_name", d.rec.AppName), slog.String("state", string(state)), slog.String("error", describeForgeError(err)))
		return fmt.Sprintf("deployment status %s not reported: %s", state, describeForgeError(err))
	}
	return ""
}

// stopGitLabEnvironment stops the environment of a closed preview and marks
// its live rows inactive. A missing environment is not an error.
func (rt *Router) stopGitLabEnvironment(ctx context.Context, f *forge, gs store.GitSource, app, scope string) {
	dc, ok := rt.gitlabDeploymentsAllowed(ctx, f, gs)
	if !ok {
		return
	}
	warning := ""
	if err := dc.StopEnvironment(ctx, f.instanceURL, f.token, f.repo, gitlabEnvironmentName(app, scope)); err != nil {
		warning = "environment not stopped: " + describeForgeError(err)
		rt.logger.Warn("api: stop gitlab environment failed", slog.String("app_name", app), slog.String("error", describeForgeError(err)))
	}
	live, err := rt.forgeDeployments.ListForgeDeploymentsByState(ctx, app, scope, string(githubapp.DeploymentSuccess), "")
	if err != nil {
		rt.logger.Warn("api: list forge deployments to deactivate failed", slog.String("app_name", app), slog.String("error", err.Error()))
		return
	}
	now := time.Now().UTC()
	for _, o := range live {
		if err := rt.forgeDeployments.SetForgeDeploymentState(ctx, o.ID, string(githubapp.DeploymentInactive), warning, now); err != nil {
			rt.logger.Warn("api: update removed preview deployment failed", slog.String("app_name", app), slog.String("error", err.Error()))
		}
		if warning != "" {
			rt.recordGitLabDeploymentPost(ctx, app, o.CommitSHA, pipeline.ReportError, "", "Environment not stopped", errors.New(warning))
		}
	}
}
