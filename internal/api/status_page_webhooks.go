package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/statuswebhook"
	"github.com/GLINCKER/levelrail/internal/store"
)

type statusWebhookResource struct {
	ID                  string    `json:"id"`
	URL                 string    `json:"url"`
	Label               string    `json:"label"`
	Events              []string  `json:"events"`
	Paused              bool      `json:"paused"`
	PausedReason        string    `json:"paused_reason,omitempty"`
	ConsecutiveFailures int       `json:"consecutive_failures"`
	CreatedAt           time.Time `json:"created_at"`
}

// statusWebhookSecretResource is returned only by create and rotate, the one time the secret is shown.
type statusWebhookSecretResource struct {
	statusWebhookResource
	SigningSecret string `json:"signing_secret"`
}

type statusWebhookRequest struct {
	URL    string   `json:"url"`
	Label  string   `json:"label"`
	Events []string `json:"events"`
}

type statusWebhookDeliveryResource struct {
	ID          string     `json:"id"`
	EventID     string     `json:"event_id"`
	EventType   string     `json:"event_type"`
	State       string     `json:"state"`
	Attempts    int        `json:"attempts"`
	StatusCode  int        `json:"status_code"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at,omitempty"`
}

func toStatusWebhookResource(w store.StatusWebhook) statusWebhookResource {
	ev := w.Events
	if ev == nil {
		ev = []string{}
	}
	return statusWebhookResource{ID: w.ID, URL: w.URL, Label: w.Label, Events: ev, Paused: w.Paused, PausedReason: w.PausedReason,
		ConsecutiveFailures: w.ConsecutiveFailures, CreatedAt: w.CreatedAt.UTC()}
}

func (rt *Router) statusWebhooksReady(w http.ResponseWriter) bool {
	if !rt.statusPageReady(w) {
		return false
	}
	if rt.statusHooks == nil {
		writeError(w, http.StatusNotImplemented, "status page webhooks are not available on this control plane")
		return false
	}
	return true
}

// statusWebhookSealer reads rt.secrets lazily because the secrets manager is wired after the status page.
func (rt *Router) statusWebhookSealer() statuswebhook.Sealer {
	s, _ := rt.secrets.(statuswebhook.Sealer)
	return s
}

func (rt *Router) webhookError(w http.ResponseWriter, what string, err error) {
	var ve statuswebhook.ValidationError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error())
	case errors.Is(err, store.ErrStatusWebhookNotFound):
		writeError(w, http.StatusNotFound, "webhook not found")
	case errors.Is(err, store.ErrStatusWebhookLimit):
		writeError(w, http.StatusConflict, "too many webhook subscribers")
	default:
		rt.internalError(w, "api: status webhook "+what+" failed", err)
	}
}

func (rt *Router) handleListStatusWebhooks(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	list, err := rt.statusHooksStore.ListStatusWebhooks(r.Context())
	if err != nil {
		rt.webhookError(w, "list", err)
		return
	}
	full := rt.callerHasAbility(r, AbilityWrite)
	out := make([]statusWebhookResource, 0, len(list))
	for _, x := range list {
		res := toStatusWebhookResource(x)
		if !full {
			res.URL = redactWebhookURL(res.URL)
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// redactWebhookURL keeps scheme and host and masks the path, which often carries a bearer token.
func redactWebhookURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "***"
	}
	if p := strings.Trim(u.Path, "/"); p != "" {
		tail := ""
		if len(p) > 8 {
			tail = p[len(p)-4:]
		}
		return u.Scheme + "://" + u.Host + "/***" + tail
	}
	return u.Scheme + "://" + u.Host
}

func (rt *Router) handleCreateStatusWebhook(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	var req statusWebhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hook, secret, err := rt.statusHooks.Create(r.Context(), req.URL, req.Label, req.Events)
	if err != nil {
		rt.webhookError(w, "create", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, statusWebhookSecretResource{statusWebhookResource: toStatusWebhookResource(hook), SigningSecret: secret})
}

func (rt *Router) handleRotateStatusWebhook(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	secret, err := rt.statusHooks.Rotate(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.webhookError(w, "rotate", err)
		return
	}
	hook, err := rt.statusHooksStore.GetStatusWebhook(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.webhookError(w, "rotate", err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, statusWebhookSecretResource{statusWebhookResource: toStatusWebhookResource(hook), SigningSecret: secret})
}

func (rt *Router) setStatusWebhookPaused(w http.ResponseWriter, r *http.Request, paused bool) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	reason := ""
	if paused {
		reason = "paused by an operator"
	}
	if err := rt.statusHooksStore.SetStatusWebhookPaused(r.Context(), r.PathValue("id"), paused, reason); err != nil {
		rt.webhookError(w, "pause", err)
		return
	}
	hook, err := rt.statusHooksStore.GetStatusWebhook(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.webhookError(w, "pause", err)
		return
	}
	writeJSON(w, http.StatusOK, toStatusWebhookResource(hook))
}

func (rt *Router) handlePauseStatusWebhook(w http.ResponseWriter, r *http.Request) {
	rt.setStatusWebhookPaused(w, r, true)
}

func (rt *Router) handleResumeStatusWebhook(w http.ResponseWriter, r *http.Request) {
	rt.setStatusWebhookPaused(w, r, false)
}

func (rt *Router) handleDeleteStatusWebhook(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	if err := rt.statusHooks.Delete(r.Context(), r.PathValue("id")); err != nil {
		rt.webhookError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleTestStatusWebhook(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	res, err := rt.statusHooks.Test(r.Context(), r.PathValue("id"), rt.statusWebhookPage(r.Context()))
	if err != nil {
		rt.webhookError(w, "test", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleListStatusWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	if !rt.statusWebhooksReady(w) {
		return
	}
	list, err := rt.statusHooks.Deliveries(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.webhookError(w, "deliveries", err)
		return
	}
	out := make([]statusWebhookDeliveryResource, 0, len(list))
	for _, d := range list {
		out = append(out, statusWebhookDeliveryResource{ID: d.ID, EventID: d.EventID, EventType: d.EventType, State: d.State, Attempts: d.Attempts,
			StatusCode: d.LastStatus, LastError: d.LastError, CreatedAt: d.CreatedAt.UTC(), DeliveredAt: d.DeliveredAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) statusWebhookPage(ctx context.Context) statuswebhook.PageRef {
	ref := statuswebhook.PageRef{Path: statuspage.PagePath}
	if s, err := rt.statusPage.GetStatusPageSettings(ctx); err == nil {
		ref.Title = s.Title
	}
	return ref
}

// statusEventsOn reports whether public events may leave the control plane: the page must be switched on.
func (rt *Router) statusEventsOn(ctx context.Context) bool {
	if rt.statusHooks == nil || rt.statusPage == nil {
		return false
	}
	s, err := rt.statusPage.GetStatusPageSettings(ctx)
	return err == nil && s.Enabled
}

// emitStatusIncidentEvent queues a webhook for a published incident. An unpublished incident
// is not found by the public read and so can never produce an event.
func (rt *Router) emitStatusIncidentEvent(ctx context.Context, id string, opened bool) {
	if !rt.statusEventsOn(ctx) {
		return
	}
	inc, err := rt.statusPage.GetPublicStatusIncident(ctx, id)
	if err != nil {
		return
	}
	comps, err := rt.statusPage.ListStatusComponents(ctx)
	if err != nil {
		rt.logger.Warn("api: status webhook: list components failed", slog.String("error", err.Error()))
		return
	}
	names := make(map[string]string, len(comps))
	for _, c := range comps {
		names[c.ID] = c.DisplayName
	}
	all, err := rt.statusPage.ListPublicStatusIncidentUpdates(ctx)
	if err != nil {
		rt.logger.Warn("api: status webhook: list updates failed", slog.String("error", err.Error()))
		return
	}
	var ups []store.StatusIncidentUpdate
	for _, u := range all {
		if u.IncidentID == inc.ID {
			ups = append(ups, u)
		}
	}
	view := statuspage.NewIncidentView(inc, names, ups)
	ev := statuswebhook.Event{Type: statusIncidentEventType(inc, opened), ObjectID: inc.ID, Stamp: inc.UpdatedAt.UTC().Format(time.RFC3339Nano),
		Page: rt.statusWebhookPage(ctx), Incident: &view}
	if err := rt.statusHooks.Emit(ctx, ev); err != nil {
		rt.logger.Warn("api: status webhook: queue event failed", slog.String("error", err.Error()))
	}
}

func statusIncidentEventType(inc store.StatusIncident, opened bool) string {
	if inc.Kind == statuspage.MaintenanceKind {
		switch {
		case inc.Status == statuspage.StatusCompleted:
			return statuswebhook.MaintenanceCompleted
		case opened:
			return statuswebhook.MaintenanceScheduled
		}
		return statuswebhook.MaintenanceUpdated
	}
	switch {
	case inc.Status == statuspage.StatusResolved:
		return statuswebhook.IncidentResolved
	case opened:
		return statuswebhook.IncidentOpened
	}
	return statuswebhook.IncidentUpdated
}

// emitStatusComponentEvent queues component_changed with the public name and description only.
func (rt *Router) emitStatusComponentEvent(ctx context.Context, c store.StatusComponent, change string) {
	if !rt.statusEventsOn(ctx) {
		return
	}
	ev := statuswebhook.Event{Type: statuswebhook.ComponentChanged, ObjectID: c.ID, Stamp: change + time.Now().UTC().Format(time.RFC3339Nano),
		Page: rt.statusWebhookPage(ctx), Component: &statuswebhook.ComponentRef{ID: c.ID, Name: c.DisplayName, Description: c.Description, Change: change}}
	if err := rt.statusHooks.Emit(ctx, ev); err != nil {
		rt.logger.Warn("api: status webhook: queue event failed", slog.String("error", err.Error()))
	}
}
