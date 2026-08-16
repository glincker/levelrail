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

// ensureAppLinked creates-or-reuses a store.App named appName and links
// serviceName to it. Called for both single- and multi-service apps so
// every service has a real AppID for network attachment.
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

	if err := rt.appGroups.UpdateServiceApp(ctx, serviceName, app.ID); err != nil {
		return "", fmt.Errorf("link service to app: %w", err)
	}
	return app.ID, nil
}

// deploySpecRequest is POST /api/v1/apps/{name}/deploy-spec's body.
type deploySpecRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref"`
	// ImageRepoBase defaults to the app name when empty.
	ImageRepoBase string                  `json:"image_repo_base,omitempty"`
	Services      map[string]spec.Service `json:"services"`
}

// deploySpecServiceResult is one service key's outcome.
type deploySpecServiceResult struct {
	ServiceKey  string `json:"service_key"`
	ServiceName string `json:"service_name"`
	Image       string `json:"image,omitempty"`
	Error       string `json:"error,omitempty"`
}

// deploySpecResponse is one result per service key. A per-key Error does
// not fail the whole request: a partial failure still fanned out.
type deploySpecResponse struct {
	AppID        string                    `json:"app_id"`
	Services     []deploySpecServiceResult `json:"services"`
	AllSucceeded bool                      `json:"all_succeeded"`
}

// handleDeploySpec handles POST /api/v1/apps/{name}/deploy-spec: fans
// app.yaml's services: map out into N builds+deploys under one app.
// Synchronous, unlike handleTriggerBuild: the useful response here is
// the per-service partial-failure detail, which an async ack would lose.
// No per-service deploy_attempts history yet (known gap, not improvised).
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

	// Private-repo token requires AbilityReadSensitive, same gate as
	// handleTriggerBuild, since repo_url is caller-controlled.
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
