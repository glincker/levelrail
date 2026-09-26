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
// It returns nil when reporting is off, the app is not on GitHub, or the
// forge rejects the call; a failure is logged and never blocks the deploy.
func (rt *Router) beginForgeDeployment(ctx context.Context, app string, gs store.GitSource, sha, environment, envURL string) *forgeDeployment {
	if !gitStatusEnabled() || !gs.ReportStatus || rt.forgeDeployments == nil || sha == "" {
		return nil
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL)
	if err != nil || f.kind != forgeGitHub {
		return nil
	}
	owner, repo := f.ownerName()
	production := environment == forgeEnvProduction
	id, err := f.github.CreateDeployment(ctx, f.instanceURL, f.token, owner, repo, sha, environment, "Deploy "+app, production)
	if err != nil {
		rt.logger.Warn("api: create forge deployment failed", slog.String("app_name", app), slog.String("environment", environment), slog.String("error", describeForgeError(err)))
		return nil
	}
	now := time.Now().UTC()
	d := &forgeDeployment{rt: rt, f: f, envURL: envURL, logURL: rt.dashboardLink(ctx, "/apps/"+app+"/deployments"), rec: store.ForgeDeployment{
		ID: newForgeDeploymentID(), AppName: app, Environment: environment, Provider: f.kind, ExternalID: id,
		CommitSHA: sha, State: string(githubapp.DeploymentInProgress), CreatedAt: now, UpdatedAt: now,
	}}
	if err := rt.forgeDeployments.CreateForgeDeployment(ctx, d.rec); err != nil {
		rt.logger.Warn("api: record forge deployment failed", slog.String("app_name", app), slog.String("error", err.Error()))
	}
	d.post(ctx, githubapp.DeploymentInProgress, "Deploying "+app)
	return d
}

// deploymentStateFor maps a webhook deploy outcome to a deployment state: a
// superseded deploy never went live, so it is reported inactive.
func deploymentStateFor(status int, message string) githubapp.DeploymentState {
	switch {
	case strings.HasPrefix(message, "not deployed"):
		return githubapp.DeploymentInactive
	case status >= http.StatusBadRequest:
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

// finish moves the deployment to its final state. On success every earlier
// successful deployment of the same environment is marked inactive.
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
