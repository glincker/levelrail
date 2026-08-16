package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file is per-app deploy-outcome notification targets, distinct
// from alert-rule CRUD (alerts.go). Mirrors its handler shape.

// DeployNotifyTargets is the surface the deploy-notify-target handlers
// need from internal/alerting.DB. *alerting.DB satisfies this
// structurally, the same convention AlertRules already establishes.
type DeployNotifyTargets interface {
	SaveDeployTarget(ctx context.Context, t alerting.DeployTarget) error
	GetDeployTarget(ctx context.Context, id string) (*alerting.DeployTarget, error)
	ListDeployTargetsForResource(ctx context.Context, resourceID string) ([]alerting.DeployTarget, error)
	DeleteDeployTarget(ctx context.Context, id string) error
}

// deployTargetResource is the wire shape for a deploy notify target.
// ID/ResourceID are always server-assigned, never taken from the body.
// NotifyURL/NotifyKind are always the *resolved* values, response-only.
type deployTargetResource struct {
	ID         string `json:"id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	ChannelID  string `json:"channel_id,omitempty"`
	NotifyURL  string `json:"notify_url,omitempty"`
	NotifyKind string `json:"notify_kind,omitempty"`
	Enabled    bool   `json:"enabled"`
}

func toDeployTargetResource(t alerting.DeployTarget) deployTargetResource {
	return deployTargetResource{
		ID:         t.ID,
		ResourceID: t.ResourceID,
		ChannelID:  t.ChannelID,
		NotifyURL:  t.NotifyURL,
		NotifyKind: string(t.NotifyKind),
		Enabled:    t.Enabled,
	}
}

// createDeployTargetRequest is POST .../deploy-notify-targets' request
// body: attach an already-connected channel by ID, not a fresh URL.
type createDeployTargetRequest struct {
	ChannelID string `json:"channel_id"`
	Enabled   bool   `json:"enabled"`
}

func (req createDeployTargetRequest) toDeployTarget(id, resourceID string) (alerting.DeployTarget, error) {
	if req.ChannelID == "" {
		return alerting.DeployTarget{}, errors.New("channel_id is required")
	}
	return alerting.DeployTarget{
		ID:         id,
		ResourceID: resourceID,
		ChannelID:  req.ChannelID,
		Enabled:    req.Enabled,
	}, nil
}

// handleCreateDeployNotifyTarget handles
// POST /api/v1/apps/{name}/deploy-notify-targets. resource_id is always
// resourceIDForApp(name), the same "the app's own URL is the only thing
// that gets to say which resource this belongs to" reasoning
// handleCreateAlertRule already applies.
func (rt *Router) handleCreateDeployNotifyTarget(w http.ResponseWriter, r *http.Request) {
	if rt.deployNotifyTargets == nil {
		writeError(w, http.StatusNotImplemented, "deploy notifications are not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create deploy notify target: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req createDeployTargetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	id, err := alerting.NewDeployTargetID()
	if err != nil {
		rt.logger.Error("api: create deploy notify target: generate id failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	target, err := req.toDeployTarget(id, resourceIDForApp(name))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Validated against the real registry first, same as
	// handleSetAppStorage does for storage_target_id.
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return
	}
	if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), target.ChannelID); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusBadRequest, "unknown channel_id")
		return
	} else if err != nil {
		rt.logger.Error("api: create deploy notify target: look up channel failed", slog.String("error", err.Error()), slog.String("channel_id", target.ChannelID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.deployNotifyTargets.SaveDeployTarget(r.Context(), target); err != nil {
		rt.logger.Error("api: create deploy notify target failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("target_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Re-fetched so the response carries resolved notify_url/notify_kind,
	// which target itself doesn't have until read back through the join.
	saved, err := rt.deployNotifyTargets.GetDeployTarget(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: create deploy notify target: reload after save failed", slog.String("error", err.Error()), slog.String("target_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toDeployTargetResource(*saved))
}

// handleListDeployNotifyTargets handles
// GET /api/v1/apps/{name}/deploy-notify-targets: every target scoped to
// this app, including disabled ones, the same "let the UI show and
// re-enable a paused one" reasoning handleListAlertRules already
// applies.
func (rt *Router) handleListDeployNotifyTargets(w http.ResponseWriter, r *http.Request) {
	if rt.deployNotifyTargets == nil {
		writeError(w, http.StatusNotImplemented, "deploy notifications are not configured on this control plane")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list deploy notify targets: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	targets, err := rt.deployNotifyTargets.ListDeployTargetsForResource(r.Context(), resourceIDForApp(name))
	if err != nil {
		rt.logger.Error("api: list deploy notify targets failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]deployTargetResource, 0, len(targets))
	for _, t := range targets {
		out = append(out, toDeployTargetResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteDeployNotifyTarget handles
// DELETE /api/v1/apps/{name}/deploy-notify-targets/{id}. Verifies the
// target's own ResourceID actually matches resourceIDForApp(name)
// before deleting it, the identical cross-app-leak protection
// handleDeleteAlertRule's own doc comment explains for rule IDs.
func (rt *Router) handleDeleteDeployNotifyTarget(w http.ResponseWriter, r *http.Request) {
	if rt.deployNotifyTargets == nil {
		writeError(w, http.StatusNotImplemented, "deploy notifications are not configured on this control plane")
		return
	}

	name := r.PathValue("name")
	id := r.PathValue("id")

	target, err := rt.deployNotifyTargets.GetDeployTarget(r.Context(), id)
	if errors.Is(err, alerting.ErrDeployTargetNotFound) {
		writeError(w, http.StatusNotFound, "deploy notify target not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete deploy notify target: load target failed", slog.String("error", err.Error()), slog.String("target_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if target.ResourceID != resourceIDForApp(name) {
		writeError(w, http.StatusNotFound, "deploy notify target not found")
		return
	}

	if err := rt.deployNotifyTargets.DeleteDeployTarget(r.Context(), id); err != nil {
		rt.logger.Error("api: delete deploy notify target failed", slog.String("error", err.Error()), slog.String("target_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
