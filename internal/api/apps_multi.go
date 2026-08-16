package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ensureAppLinked creates-or-reuses a store.App named appName (App.ID ==
// App.Name, the same convention migrations/0039_apps.sql's own backfill
// and internal/deploy.Pipeline.DeploySpec's own ensureApp establish) and
// links serviceName to it. Called from both handleCreateApp (an
// ordinary single-service app, appName == serviceName) and
// handleDeploySpec (one call per fanned-out service, same appName,
// different serviceName each time), so every app this control plane
// creates, single- or multi-service alike, ends up with a real AppID:
// internal/reconcile/application's Controller only attaches a
// per-service container to a Docker network when AppID is set, and
// skipping that for the common single-service case would mean
// networking only ever works after an app is upgraded to multi-service,
// which apps_group.go's own doc comment (and CLAUDE.md's dispatch for
// this feature) both call out as the wrong shape.
func (rt *Router) ensureAppLinked(ctx context.Context, appName, serviceName string) (string, error) {
	app, err := rt.appGroups.GetAppByName(ctx, appName)
	if errors.Is(err, store.ErrAppNotFound) {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		app = store.App{ID: appName, Name: appName, CreatedAt: now, UpdatedAt: now}
		if saveErr := rt.appGroups.SaveApp(ctx, app); saveErr != nil {
			return "", fmt.Errorf("save app: %w", saveErr)
		}
	} else if err != nil {
		return "", fmt.Errorf("look up app: %w", err)
	}

	if err := rt.apps.UpdateServiceApp(ctx, serviceName, app.ID); err != nil {
		return "", fmt.Errorf("link service to app: %w", err)
	}
	return app.ID, nil
}

// deploySpecRequest is POST /api/v1/apps/{name}/deploy-spec's body: a
// git source (repo_url, ref) plus a full app.yaml services: map,
// mirroring triggerBuildRequest's own repo_url/ref shape
// (handleTriggerBuild, builds.go) but for N services fanned out under
// one app instead of one already-existing app's own rebuild.
type deploySpecRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref"`
	// ImageRepoBase defaults to the app name (the {name} path segment)
	// when empty, the same default triggerBuildRequest.ImageRepo already
	// uses for a single service.
	ImageRepoBase string                  `json:"image_repo_base,omitempty"`
	Services      map[string]spec.Service `json:"services"`
}

// deploySpecServiceResult is one service key's own outcome:
// deploy.ServiceOutcome's wire shape, Error as a plain string (never a
// Go error value) so a partial failure is still valid JSON.
type deploySpecServiceResult struct {
	ServiceKey  string `json:"service_key"`
	ServiceName string `json:"service_name"`
	Image       string `json:"image,omitempty"`
	Error       string `json:"error,omitempty"`
}

// deploySpecResponse is POST /api/v1/apps/{name}/deploy-spec's success
// body: one result per service key, in the same deterministic order
// DeploySpec itself returns them. A per-key Error does not fail the
// whole HTTP request (still 200/202): the request as a whole succeeded
// at fanning out, even if one service's own build failed, matching
// DeploySpec's own "one broken resource must not block the others" doc
// comment. AllSucceeded lets a caller check the common case (every
// service deployed) without walking the Services slice itself.
type deploySpecResponse struct {
	AppID        string                    `json:"app_id"`
	Services     []deploySpecServiceResult `json:"services"`
	AllSucceeded bool                      `json:"all_succeeded"`
}

// handleDeploySpec handles POST /api/v1/apps/{name}/deploy-spec: one
// app.yaml's services: map becomes N independent builds+deploys under
// one store.App named {name}. Synchronous and blocking, the same
// tradeoff handleTriggerBuild's own doc comment already accepts for an
// identical reason (no build-progress-streaming infrastructure exists
// yet): every service builds in sequence (DeploySpec's own doc comment:
// deterministic key order, not concurrent) before this handler
// responds, so a large multi-service app.yaml is a genuinely slow
// request, not just a slow build.
//
// A build failure for one service key does not fail the request: see
// deploySpecResponse's own doc comment. This intentionally does not
// record deploy_attempts history per service (unlike
// handleTriggerBuild/webhook's single-service paths): a real per-service
// attempt log for a fanned-out deploy is a store-schema-sized follow-up
// (one attempt row per service key sharing one trigger), left as a
// known, documented gap rather than improvised here.
func (rt *Router) handleDeploySpec(w http.ResponseWriter, r *http.Request) {
	if rt.builder == nil {
		writeError(w, http.StatusNotImplemented, "manual build trigger is not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	var req deploySpecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	if req.Ref == "" {
		writeError(w, http.StatusBadRequest, "ref is required")
		return
	}
	if len(req.Services) == 0 {
		writeError(w, http.StatusBadRequest, "services must declare at least one service")
		return
	}
	for key, svc := range req.Services {
		switch svc.Build.Type {
		case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic:
			// Supported.
		case spec.BuildCompose:
			writeError(w, http.StatusNotImplemented, fmt.Sprintf("service %q: build.type %q is not yet supported for a multi-service deploy", key, svc.Build.Type))
			return
		default:
			writeError(w, http.StatusBadRequest, fmt.Sprintf("service %q: build.type %q is not recognized", key, svc.Build.Type))
			return
		}
	}

	imageRepoBase := req.ImageRepoBase
	if imageRepoBase == "" {
		imageRepoBase = name
	}

	// AbilityDeploy alone (this route's own gate) is not enough to
	// authorize minting a live GitHub App installation token, the same
	// reasoning handleTriggerBuild's own doc comment gives for the
	// identical check there.
	var token string
	if rt.callerHasAbility(r, AbilityReadSensitive) {
		token = rt.tokenForRepo(r.Context(), req.RepoURL)
	}

	sourceDir, cleanup, err := rt.fetch(r.Context(), req.RepoURL, req.Ref, token)
	if err != nil {
		rt.logger.Error("api: deploy spec: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.String("ref", req.Ref))
		writeError(w, http.StatusBadRequest, "fetching source failed: check repo_url and ref")
		return
	}
	defer cleanup()

	progress := func(serviceKey string, ev build.ProgressEvent) {
		rt.logger.Info("api: deploy spec: build progress", slog.String("name", name), slog.String("service_key", serviceKey), slog.String("step", ev.Step), slog.Bool("completed", ev.Completed), slog.String("error", ev.Error))
	}

	outcomes, err := rt.builder.DeploySpec(r.Context(), deploy.MultiRequest{
		AppName:       name,
		Services:      req.Services,
		SourceDir:     sourceDir,
		CommitSHA:     req.Ref,
		ImageRepoBase: imageRepoBase,
	}, progress)
	if err != nil {
		rt.logger.Error("api: deploy spec failed", slog.String("error", err.Error()), slog.String("name", name))
		if errors.Is(err, deploy.ErrNoServices) {
			writeError(w, http.StatusBadRequest, "services must declare at least one service")
			return
		}
		writeError(w, http.StatusInternalServerError, "deploy failed")
		return
	}

	resp := deploySpecResponse{AllSucceeded: true, Services: make([]deploySpecServiceResult, 0, len(outcomes))}
	for _, o := range outcomes {
		result := deploySpecServiceResult{ServiceKey: o.ServiceKey, ServiceName: o.ServiceName, Image: o.Image}
		if o.Err != nil {
			result.Error = o.Err.Error()
			resp.AllSucceeded = false
			rt.logger.Error("api: deploy spec: service failed", slog.String("name", name), slog.String("service_key", o.ServiceKey), slog.String("error", o.Err.Error()))
		}
		resp.Services = append(resp.Services, result)
	}

	if app, err := rt.appGroups.GetAppByName(r.Context(), name); err == nil {
		resp.AppID = app.ID
	}

	rt.logger.Info("api: multi-service deploy triggered", slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.String("ref", req.Ref), slog.Int("services", len(req.Services)), slog.Bool("all_succeeded", resp.AllSucceeded))

	status := http.StatusCreated
	if !resp.AllSucceeded {
		status = http.StatusMultiStatus
	}
	writeJSON(w, status, resp)
}
