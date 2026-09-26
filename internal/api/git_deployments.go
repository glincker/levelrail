package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/githubapp"
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

// forgeDeployment reports one app deploy to the GitHub Deployments API. A
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
	f, err := rt.resolveForge(ctx, gs.RepoURL)
	if err != nil || f.kind != forgeGitHub {
		return nil
	}
	return rt.beginOnForge(ctx, f, app, sha, environment, scope, envURL)
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
	owner, repo := d.f.ownerName()
	warning := ""
	if err := d.f.github.CreateDeploymentStatus(ctx, d.f.instanceURL, d.f.token, owner, repo, d.rec.ExternalID, githubapp.DeploymentInactive, "", "", "Superseded"); err != nil {
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
	f, err := rt.resolveForge(ctx, gs.RepoURL)
	if err != nil || f.kind != forgeGitHub {
		return
	}
	rt.deactivateLive(ctx, f, app, scope)
}

func (rt *Router) deactivateLive(ctx context.Context, f *forge, app, scope string) {
	live, err := rt.forgeDeployments.ListForgeDeploymentsByState(ctx, app, scope, string(githubapp.DeploymentSuccess), "")
	if err != nil {
		rt.logger.Warn("api: list forge deployments to deactivate failed", slog.String("app_name", app), slog.String("error", err.Error()))
		return
	}
	owner, repo := f.ownerName()
	now := time.Now().UTC()
	for _, o := range live {
		w := ""
		if err := f.github.CreateDeploymentStatus(ctx, f.instanceURL, f.token, owner, repo, o.ExternalID, githubapp.DeploymentInactive, "", "", "Preview removed"); err != nil {
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
	owner, repo := d.f.ownerName()
	for _, o := range older {
		if !o.CreatedAt.Before(d.rec.CreatedAt) {
			d.supersedeSelf(ctx, now)
			continue
		}
		w := ""
		if err := d.f.github.CreateDeploymentStatus(ctx, d.f.instanceURL, d.f.token, owner, repo, o.ExternalID, githubapp.DeploymentInactive, "", "", "Superseded"); err != nil {
			w = fmt.Sprintf("deployment status inactive not reported: %s", describeForgeError(err))
			d.rt.logger.Warn("api: mark superseded forge deployment inactive failed", slog.String("app_name", o.AppName), slog.String("error", describeForgeError(err)))
		}
		if err := d.rt.forgeDeployments.SetForgeDeploymentState(ctx, o.ID, string(githubapp.DeploymentInactive), w, now); err != nil && !errors.Is(err, context.Canceled) {
			d.rt.logger.Warn("api: update superseded forge deployment failed", slog.String("app_name", o.AppName), slog.String("error", err.Error()))
		}
	}
}
