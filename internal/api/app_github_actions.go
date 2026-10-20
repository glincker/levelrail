package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const maxGitHubEnvironmentLen = 255

// githubEnvironmentPattern keeps the name safe to pass to GitHub and to paste into a workflow.
var githubEnvironmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/-]*$`)

// AppGitHubActionsStore is the store surface for the GitHub Actions report settings.
type AppGitHubActionsStore interface {
	GetAppGitHubActions(ctx context.Context, serviceName string) (store.AppGitHubActions, error)
	SetAppGitHubActions(ctx context.Context, s store.AppGitHubActions) error
}

type appGitHubActionsResource struct {
	Environment       string `json:"environment"`
	ReportDeployments bool   `json:"report_deployments"`
	ReportChecks      bool   `json:"report_checks"`
}

func toAppGitHubActionsResource(s store.AppGitHubActions) appGitHubActionsResource {
	return appGitHubActionsResource{Environment: s.Environment, ReportDeployments: s.ReportDeployments, ReportChecks: s.ReportChecks}
}

func (rt *Router) requireAppForGitHubActions(w http.ResponseWriter, r *http.Request, name string) bool {
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
		} else {
			rt.internalError(w, "get app for github actions settings", err)
		}
		return false
	}
	return true
}

// handleGetAppGitHubActions handles GET /api/v1/apps/{name}/github-actions.
func (rt *Router) handleGetAppGitHubActions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.requireAppForGitHubActions(w, r, name) {
		return
	}
	s, err := rt.githubActions.GetAppGitHubActions(r.Context(), name)
	if err != nil {
		rt.internalError(w, "get github actions settings", err)
		return
	}
	writeJSON(w, http.StatusOK, toAppGitHubActionsResource(s))
}

// handleSetAppGitHubActions handles PUT /api/v1/apps/{name}/github-actions.
func (rt *Router) handleSetAppGitHubActions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req appGitHubActionsResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Environment = strings.TrimSpace(req.Environment)
	if req.Environment == "" {
		req.Environment = store.DefaultGitHubActionsEnvironment
	}
	if len(req.Environment) > maxGitHubEnvironmentLen || !githubEnvironmentPattern.MatchString(req.Environment) {
		writeError(w, http.StatusBadRequest, "environment must be a plain GitHub environment name")
		return
	}
	if !rt.requireAppForGitHubActions(w, r, name) {
		return
	}
	s := store.AppGitHubActions{
		ServiceName: name, Environment: req.Environment,
		ReportDeployments: req.ReportDeployments, ReportChecks: req.ReportChecks,
	}
	if err := rt.githubActions.SetAppGitHubActions(r.Context(), s); err != nil {
		rt.internalError(w, "set github actions settings", err)
		return
	}
	writeJSON(w, http.StatusOK, toAppGitHubActionsResource(s))
}
