package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

// previewEnvironmentResource is the wire shape for one preview
// environment, GET /api/v1/apps/{name}/previews's own list element.
type previewEnvironmentResource struct {
	PRNumber     int    `json:"pr_number"`
	PreviewAppID string `json:"preview_app_id"`
	Branch       string `json:"branch"`
	HeadSHA      string `json:"head_sha"`
	Domain       string `json:"domain,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func toPreviewEnvironmentResource(p store.PreviewEnvironment) previewEnvironmentResource {
	return previewEnvironmentResource{
		PRNumber: p.PRNumber, PreviewAppID: p.PreviewAppID, Branch: p.Branch, HeadSHA: p.HeadSHA,
		Domain: p.Domain, Status: p.Status, StatusReason: p.StatusReason,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// handleListPreviewEnvironments handles GET /api/v1/apps/{name}/previews.
// Always 200 with an empty array for an app with no previews (never
// connected a git source, previews not enabled, or none open), matching
// GET .../secrets and every other list-shaped route in this package:
// there is no "not found" case for a list, only an empty one.
func (rt *Router) handleListPreviewEnvironments(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	previews, err := rt.previewEnvironments.ListPreviewEnvironmentsByApp(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list preview environments failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]previewEnvironmentResource, 0, len(previews))
	for _, p := range previews {
		out = append(out, toPreviewEnvironmentResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleTeardownPreviewEnvironment handles POST
// /api/v1/apps/{name}/previews/{number}/teardown: the manual safety net
// alongside the pull-request-closed webhook's own automatic teardown,
// for a preview an operator wants gone right now (a stuck build, a
// mistakenly-left-open PR, or the automatic teardown's own partial
// failure needing a retry). Same teardownPreviewRecord implementation
// the webhook path uses, so a manual and an automatic teardown can never
// diverge in behavior.
func (rt *Router) handleTeardownPreviewEnvironment(w http.ResponseWriter, r *http.Request) {
	// teardownPreviewRecord's message embeds preview.PreviewAppID, built
	// from a webhook-sourced PR number (previewAppName in
	// preview_environments.go): an explicit text/plain content type stops
	// a client from ever MIME-sniffing this body as HTML, the same
	// mitigation handleGitPushWebhook applies for its own webhook-derived
	// response text.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	name := r.PathValue("name")
	prNumber, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "pr number must be an integer")
		return
	}

	preview, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(r.Context(), name, prNumber)
	if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "no preview environment found for this pull request")
		return
	}
	if err != nil {
		rt.logger.Error("api: teardown preview environment: load failed", slog.String("error", err.Error()), slog.String("name", name), slog.Int("pr_number", prNumber))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status, message := rt.teardownPreviewRecord(r.Context(), *preview)
	w.WriteHeader(status)
	if _, err := w.Write([]byte(message)); err != nil {
		rt.logger.Warn("api: teardown preview environment: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// setPreviewEnabledRequest is PUT /api/v1/apps/{name}/preview-settings's
// body: a single boolean toggle, deliberately its own tiny endpoint
// rather than one more field on PUT .../git-source's already-large
// connect/edit form (GitSourceCard.tsx already sits well past this
// codebase's own per-file line budget), so enabling previews needs no
// round-trip through every other git-source field.
type setPreviewEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// handleSetPreviewEnabled handles PUT
// /api/v1/apps/{name}/preview-settings. Requires a git source to
// already be connected: a preview has nothing to build from otherwise.
func (rt *Router) handleSetPreviewEnabled(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setPreviewEnabledRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.gitSources.SetGitSourcePreviewEnabled(r.Context(), name, req.Enabled); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	} else if err != nil {
		rt.logger.Error("api: set preview enabled failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.logger.Info("api: preview environments toggled", slog.String("name", name), slog.Bool("enabled", req.Enabled))
	writeJSON(w, http.StatusOK, setPreviewEnabledRequest{Enabled: req.Enabled})
}
