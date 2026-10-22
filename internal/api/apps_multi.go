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
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Every app gets an AppID, even single-service ones: the reconciler attaches containers to a network only when it is set.
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

// A git source plus the full app.yaml services map, fanned out under one app.
type deploySpecRequest struct {
	RepoURL string `json:"repo_url"`
	Ref     string `json:"ref"`
	// ImageRepoBase defaults to the app name (the {name} path segment)
	// when empty, the same default triggerBuildRequest.ImageRepo already
	// uses for a single service.
	ImageRepoBase string                  `json:"image_repo_base,omitempty"`
	Services      map[string]spec.Service `json:"services"`
	// SingleServiceName overrides the default "<name>-<serviceKey>"
	// naming for the sole entry in Services, see deploy.MultiRequest's
	// own field of the same name.
	SingleServiceName string `json:"single_service_name,omitempty"`
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

// A per-service Error does not fail the request: the fan-out itself succeeded.
type deploySpecResponse struct {
	AppID        string                    `json:"app_id"`
	Services     []deploySpecServiceResult `json:"services"`
	AllSucceeded bool                      `json:"all_succeeded"`
}

// Builds run in sequence and block the response, since no build-progress stream exists yet.
// Per-service deploy_attempts history is not recorded; this is a known gap.
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
	if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil {
		writeError(w, http.StatusBadRequest, "repo_url must use http or https")
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
	if req.SingleServiceName != "" && len(req.Services) != 1 {
		writeError(w, http.StatusBadRequest, "single_service_name requires exactly one service")
		return
	}
	if err := validateDeploySpecServiceTypes(req.Services); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for key, svc := range req.Services {
		if err := svc.Health.Validate(key); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if deploySpecHasBindMount(req.Services) && !rt.callerHasAbility(r, AbilityRoot) {
		writeError(w, http.StatusForbidden, "one or more services declare a bind mount, which requires the root ability")
		return
	}

	if !rt.authorizeSpecDatabaseRefs(w, r, req.Services) {
		return
	}
	if !rt.authorizeServiceNames(w, r, name, specServiceNames(name, req.SingleServiceName, req.Services)) {
		return
	}
	inlineSecrets := deploySpecInlineSecrets(req.Services)
	if len(inlineSecrets) > 0 && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
		return
	}
	// Stored before DeploySpec runs, since validateEnv checks required secrets up front.
	for key, values := range inlineSecrets {
		svcName := name + "-" + key
		if req.SingleServiceName != "" {
			svcName = req.SingleServiceName
		}
		for envKey, value := range values {
			if err := rt.secrets.SetValueGuarded(r.Context(), svcName, envKey, value, false); err != nil {
				if errors.Is(err, secrets.ErrSecretLocked) {
					writeError(w, http.StatusConflict, fmt.Sprintf("service %q secret %q is locked", key, envKey))
					return
				}
				rt.internalError(w, "api: deploy spec: set secret failed", err, slog.String("name", name), slog.String("service_key", key), slog.String("env_key", envKey))
				return
			}
		}
	}

	imageRepoBase := req.ImageRepoBase
	if imageRepoBase == "" {
		imageRepoBase = name
	}

	// AbilityDeploy alone does not authorize minting a GitHub App installation token.
	token := rt.tokenForRepo(r.Context(), name, req.RepoURL, rt.callerHasAbility(r, AbilityReadSensitive))

	sourceDir, commit, cleanup, err := rt.fetch(r.Context(), req.RepoURL, req.Ref, token)
	if err != nil {
		rt.logger.Error("api: deploy spec: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("repo_url", redactURLCredentials(req.RepoURL)), slog.String("ref", req.Ref))
		writeError(w, http.StatusBadRequest, "fetching source failed: check repo_url and ref")
		return
	}
	defer cleanup()

	// Tag by the resolved commit, not req.Ref: see fetchFunc (builds.go).
	commitLabel := req.Ref
	if commit != "" {
		commitLabel = commit
	}

	progress := func(serviceKey string, ev build.ProgressEvent) {
		rt.logger.Info("api: deploy spec: build progress", slog.String("name", name), slog.String("service_key", serviceKey), slog.String("step", ev.Step), slog.Bool("completed", ev.Completed), slog.String("error", ev.Error))
	}

	outcomes, err := rt.builder.DeploySpec(r.Context(), deploy.MultiRequest{
		AppName:           name,
		Services:          req.Services,
		SourceDir:         sourceDir,
		CommitSHA:         commitLabel,
		ImageRepoBase:     imageRepoBase,
		SingleServiceName: req.SingleServiceName,
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

	rt.logger.Info("api: multi-service deploy triggered", slog.String("name", name), slog.String("repo_url", redactURLCredentials(req.RepoURL)), slog.String("ref", req.Ref), slog.Int("services", len(req.Services)), slog.Bool("all_succeeded", resp.AllSucceeded))

	status := http.StatusCreated
	if !resp.AllSucceeded {
		status = http.StatusMultiStatus
	}
	rt.nudgeReconciler()
	writeJSON(w, status, resp)
}

// Shared with handleSetGitSource so the two never validate build types differently.
// A fan-out still needs repo_url and ref even when every service is build.type: image.
func validateDeploySpecServiceTypes(services map[string]spec.Service) error {
	for key, svc := range services {
		switch svc.Build.Type {
		case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic, spec.BuildImage, spec.BuildCompose:
		default:
			return fmt.Errorf("service %q: build.type %q is not recognized", key, svc.Build.Type)
		}
	}
	return nil
}

// app.yaml cannot carry a secret value, so this only fires for JSON requests such as the CLI's --secret.
func deploySpecInlineSecrets(services map[string]spec.Service) map[string]map[string]string {
	var out map[string]map[string]string
	for key, svc := range services {
		for envKey, v := range svc.Env {
			if !v.Secret || v.Value == "" {
				continue
			}
			if out == nil {
				out = make(map[string]map[string]string)
			}
			if out[key] == nil {
				out[key] = make(map[string]string)
			}
			out[key][envKey] = v.Value
		}
	}
	return out
}

// deploySpecHasBindMount reports whether any service declares a bind
// mount, the same signal hasBindMount (apps_compose.go) uses to require
// AbilityRoot on top of this route's own AbilityDeploy gate.
func deploySpecHasBindMount(services map[string]spec.Service) bool {
	for _, svc := range services {
		for _, v := range svc.Volumes {
			if v.HostPath != "" {
				return true
			}
		}
	}
	return false
}
