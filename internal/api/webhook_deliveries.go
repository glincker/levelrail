package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultWebhookDeliveryLimit = 50
	maxWebhookDeliveryLimit     = 200
)

// webhookDeliveryResource is the wire shape for one recorded inbound
// webhook delivery, real visibility into what a git provider actually
// sent and what this control plane did about it: the operator-facing
// counterpart to the GitHub-style "recent deliveries" panel this feature
// implements.
type webhookDeliveryResource struct {
	ID               string    `json:"id"`
	ServiceName      string    `json:"service_name"`
	Provider         string    `json:"provider"`
	EventType        string    `json:"event_type"`
	SignatureValid   bool      `json:"signature_valid"`
	Matched          bool      `json:"matched"`
	StatusCode       int       `json:"status_code"`
	Payload          string    `json:"payload"`
	PayloadTruncated bool      `json:"payload_truncated"`
	Error            string    `json:"error,omitempty"`
	Note             string    `json:"note,omitempty"`
	ReceivedAt       time.Time `json:"received_at"`
	DeliveryID       string    `json:"delivery_id,omitempty"`
	AttemptID        string    `json:"attempt_id,omitempty"`
}

func toWebhookDeliveryResource(d store.WebhookDelivery) webhookDeliveryResource {
	return webhookDeliveryResource{
		ID:               d.ID,
		ServiceName:      d.ServiceName,
		Provider:         d.Provider,
		EventType:        d.EventType,
		SignatureValid:   d.SignatureValid,
		Matched:          d.Matched,
		StatusCode:       d.StatusCode,
		Payload:          string(d.Payload),
		PayloadTruncated: d.PayloadTruncated,
		Error:            d.Error,
		Note:             d.Note,
		ReceivedAt:       d.ReceivedAt,
		DeliveryID:       d.DeliveryID,
		AttemptID:        d.AttemptID,
	}
}

// handleListWebhookDeliveries handles
// GET /api/v1/apps/{name}/webhook-deliveries. Cursor-paginated by
// ?before/?limit, the same contract handleListBackupHistory already
// establishes: an app under active development sees steady webhook
// traffic, so this list is unbounded over the app's lifetime.
func (rt *Router) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list webhook deliveries: load app failed", err, slog.String("name", name))
		return
	}

	limit := defaultWebhookDeliveryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxWebhookDeliveryLimit {
		limit = maxWebhookDeliveryLimit
	}

	var before *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC3339 timestamp")
			return
		}
		before = &t
	}

	deliveries, err := rt.webhookDeliveries.ListWebhookDeliveries(r.Context(), name, limit, before)
	if err != nil {
		rt.internalError(w, "api: list webhook deliveries failed", err, slog.String("name", name))
		return
	}

	out := make([]webhookDeliveryResource, 0, len(deliveries))
	for _, d := range deliveries {
		out = append(out, toWebhookDeliveryResource(d))
	}
	writeJSON(w, http.StatusOK, out)
}

// replayWebhookDeliveryResult is what POST
// .../webhook-deliveries/{id}/replay returns: the same status/message a
// live webhook delivery's own HTTP response carries, so an operator (or
// the CLI/dashboard rendering this) sees exactly what re-running the
// stored payload actually did.
type replayWebhookDeliveryResult struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// handleReplayWebhookDelivery handles
// POST /api/v1/apps/{name}/webhook-deliveries/{id}/replay: re-runs
// processGitPushWebhookPayload (git_webhook.go) against a stored
// delivery's exact payload and header fields, the same processing path a
// live webhook takes, rather than a second, forkable copy of it.
// AbilityDeploy-gated, matching POST .../deploys: a replay can trigger a
// real build and deploy, the identical class of side effect.
//
// Deliberately does not re-verify the original request's signature: the
// caller already authenticated at AbilityDeploy to reach this route, and
// re-checking a signature against a secret that may have since rotated
// would incorrectly block a legitimate replay of an old delivery.
// Deliberately does not write a new webhook_deliveries row either: a
// replay is an operator-initiated action already visible in the audit
// log requireAbility writes for every AbilityDeploy call, not a second
// inbound delivery.
func (rt *Router) handleReplayWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: replay webhook delivery: load app failed", err, slog.String("name", name))
		return
	}

	delivery, err := rt.webhookDeliveries.GetWebhookDelivery(r.Context(), id)
	if errors.Is(err, store.ErrWebhookDeliveryNotFound) {
		writeError(w, http.StatusNotFound, "webhook delivery not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: replay webhook delivery: load delivery failed", err, slog.String("id", id))
		return
	}
	// Delivery IDs are globally unique but the route is app-scoped: a
	// mismatch here is treated as not-found, the same cross-app-leak
	// guard handleDeployLogStream's own doc comment establishes for
	// deploy attempt IDs.
	if delivery.ServiceName != name {
		writeError(w, http.StatusNotFound, "webhook delivery not found")
		return
	}

	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: replay webhook delivery: load git source failed", err, slog.String("name", name))
		return
	}

	status, message := rt.replayDelivery(r.Context(), name, *gs, delivery)
	rt.logger.Info("api: replayed webhook delivery", slog.String("name", name), slog.String("delivery_id", id), slog.Int("status", status))
	writeJSON(w, http.StatusOK, replayWebhookDeliveryResult{Status: status, Message: message})
}
