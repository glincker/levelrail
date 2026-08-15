package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// This file is deploy-outcome notification targets: a Slack/Discord/
// Telegram/generic-webhook/email destination that fires once per deploy
// attempt reaching a terminal state, distinct from this package's
// existing alert-rule CRUD (alerts.go) the same way
// internal/alerting/deploy_notify.go's own package-level doc comment
// distinguishes the two. Mirrors alerts.go's own handler shape closely
// (narrow store interface, wire resource type, create/list/delete
// handlers, app-ownership check on delete) on purpose: an operator who
// already knows how to configure an alert rule's notify channel should
// find this surface immediately familiar.

// DeployNotifyTargets is the surface the deploy-notify-target handlers
// need from internal/alerting.DB. *alerting.DB satisfies this
// structurally, the same "narrow consumer-defined interface" convention
// AlertRules already establishes in this package.
type DeployNotifyTargets interface {
	SaveDeployTarget(ctx context.Context, t alerting.DeployTarget) error
	GetDeployTarget(ctx context.Context, id string) (*alerting.DeployTarget, error)
	ListDeployTargetsForResource(ctx context.Context, resourceID string) ([]alerting.DeployTarget, error)
	DeleteDeployTarget(ctx context.Context, id string) error
}

// deployTargetResource is the wire shape for a deploy notify target.
// ID and ResourceID are never caller-supplied, the same reasoning
// ruleResource's own doc comment gives for its own ID/ResourceID
// fields: ID is always generated server-side
// (alerting.NewDeployTargetID), ResourceID is always derived from the
// app name in the URL (resourceIDForApp), never taken from the body.
type deployTargetResource struct {
	ID         string `json:"id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	NotifyURL  string `json:"notify_url,omitempty"`
	NotifyKind string `json:"notify_kind,omitempty"`
	Enabled    bool   `json:"enabled"`
}

func toDeployTargetResource(t alerting.DeployTarget) deployTargetResource {
	return deployTargetResource{
		ID:         t.ID,
		ResourceID: t.ResourceID,
		NotifyURL:  t.NotifyURL,
		NotifyKind: string(t.NotifyKind),
		Enabled:    t.Enabled,
	}
}

// toDeployTarget converts a request body into an alerting.DeployTarget,
// validating it along the way. id and resourceID are assigned by the
// caller (handleCreateDeployNotifyTarget), never taken from the
// resource itself.
func (d deployTargetResource) toDeployTarget(id, resourceID string) (alerting.DeployTarget, error) {
	if d.NotifyURL == "" {
		return alerting.DeployTarget{}, errors.New("notify_url is required")
	}

	kind := alerting.NotifyKind(d.NotifyKind)
	switch kind {
	case "", alerting.NotifyGeneric, alerting.NotifySlack, alerting.NotifyDiscord, alerting.NotifyTelegram, alerting.NotifyEmail:
		// valid (including empty, which NewNotifier-style dispatch treats
		// as NotifyGeneric); anything else is rejected below.
	default:
		return alerting.DeployTarget{}, fmt.Errorf("notify_kind must be one of %q, %q, %q, %q, %q",
			alerting.NotifyGeneric, alerting.NotifySlack, alerting.NotifyDiscord, alerting.NotifyTelegram, alerting.NotifyEmail)
	}

	return alerting.DeployTarget{
		ID:         id,
		ResourceID: resourceID,
		NotifyURL:  d.NotifyURL,
		NotifyKind: kind,
		Enabled:    d.Enabled,
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

	var req deployTargetResource
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

	if err := rt.deployNotifyTargets.SaveDeployTarget(r.Context(), target); err != nil {
		rt.logger.Error("api: create deploy notify target failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("target_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toDeployTargetResource(target))
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
