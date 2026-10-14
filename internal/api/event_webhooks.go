package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EventWebhookService is the surface the event webhook routes need from
// internal/eventhook; *eventhook.Service satisfies it.
type EventWebhookService interface {
	Emit(e eventhook.Event)
	List(ctx context.Context) ([]store.EventWebhook, error)
	Get(ctx context.Context, id string) (store.EventWebhook, error)
	Create(ctx context.Context, in eventhook.Input) (store.EventWebhook, string, error)
	Update(ctx context.Context, id string, p eventhook.Patch) (store.EventWebhook, error)
	Delete(ctx context.Context, id string) error
	RotateSecret(ctx context.Context, id string) (string, error)
	Test(ctx context.Context, id string) (int, error)
	Replay(ctx context.Context, deliveryID string) (store.EventWebhookDelivery, error)
	Deliveries(ctx context.Context, webhookID string, limit int) ([]store.EventWebhookDelivery, error)
	Summary(ctx context.Context, id string) (eventhook.Counts, error)
}

// SetEventWebhooks enables the event webhook routes and starts feeding
// audited mutations to subscribers.
func (rt *Router) SetEventWebhooks(s EventWebhookService) { rt.eventWebhooks = s }

func (rt *Router) eventWebhooksOrNotImplemented(w http.ResponseWriter) (EventWebhookService, bool) {
	if rt.eventWebhooks == nil {
		writeError(w, http.StatusNotImplemented, "event webhooks are not configured on this control plane")
		return nil, false
	}
	return rt.eventWebhooks, true
}

// emitAuditEvent hands a successful audited mutation to the webhook queue.
// Emit never blocks, so a slow subscriber cannot slow a request.
func (rt *Router) emitAuditEvent(e store.AuditEntry) {
	if rt.eventWebhooks == nil || e.StatusCode >= http.StatusBadRequest {
		return
	}
	typ := eventhook.TypeForRequest(e.Method, e.Path, e.Action)
	if typ == "" {
		return
	}
	rt.eventWebhooks.Emit(eventhook.Event{Type: typ, Data: map[string]any{
		"actor_type": e.ActorType, "actor_id": e.ActorID, "actor_name": e.ActorName,
		"method": e.Method, "path": e.Path, "status": e.StatusCode, "audit_id": e.ID,
	}})
}

type eventWebhookResource struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Enabled   bool     `json:"enabled"`
	Events    []string `json:"events"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
	Pending   int      `json:"pending"`
	Delivered int      `json:"delivered"`
	Dead      int      `json:"dead"`
	// LastStatus is the HTTP status of the newest delivery, 0 when none.
	LastStatus int `json:"last_status"`
	// SigningSecret is present only in the create and rotate responses.
	SigningSecret string `json:"signing_secret,omitempty"`
}

type eventDeliveryResource struct {
	ID             string `json:"id"`
	WebhookID      string `json:"webhook_id"`
	EventID        string `json:"event_id"`
	EventType      string `json:"event_type"`
	State          string `json:"state"`
	Attempts       int    `json:"attempts"`
	ResponseStatus int    `json:"response_status"`
	LastError      string `json:"last_error,omitempty"`
	NextAttemptAt  string `json:"next_attempt_at"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type eventWebhookTestResponse struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

type createEventWebhookRequest struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Enabled *bool    `json:"enabled,omitempty"`
	Events  []string `json:"events,omitempty"`
}

type updateEventWebhookRequest struct {
	Name    *string   `json:"name,omitempty"`
	URL     *string   `json:"url,omitempty"`
	Enabled *bool     `json:"enabled,omitempty"`
	Events  *[]string `json:"events,omitempty"`
}

func (rt *Router) eventWebhookResource(ctx context.Context, svc EventWebhookService, w store.EventWebhook) eventWebhookResource {
	res := eventWebhookResource{
		ID: w.ID, Name: w.Name, URL: w.URL, Enabled: w.Enabled, Events: w.Events,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
	}
	if res.Events == nil {
		res.Events = []string{}
	}
	if c, err := svc.Summary(ctx, w.ID); err == nil {
		res.Pending, res.Delivered, res.Dead, res.LastStatus = c.Pending, c.Delivered, c.Dead, c.LastStatus
	}
	return res
}

func (rt *Router) writeEventWebhookError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, eventhook.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), eventhook.ErrInvalid.Error()+": "))
	case errors.Is(err, eventhook.ErrSecretsUnavailable):
		writeError(w, http.StatusConflict, "configure a master key before adding event webhooks, signing secrets are stored encrypted")
	case errors.Is(err, store.ErrEventWebhookNotFound):
		writeError(w, http.StatusNotFound, "event webhook not found")
	case errors.Is(err, store.ErrEventDeliveryNotFound):
		writeError(w, http.StatusNotFound, "event webhook delivery not found")
	case errors.Is(err, store.ErrEventWebhookNameTaken):
		writeError(w, http.StatusConflict, "an event webhook with this name already exists")
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) handleListEventWebhooks(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	hooks, err := svc.List(r.Context())
	if err != nil {
		rt.writeEventWebhookError(w, "list event webhooks", err)
		return
	}
	out := make([]eventWebhookResource, len(hooks))
	for i, h := range hooks {
		out[i] = rt.eventWebhookResource(r.Context(), svc, h)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetEventWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	h, err := svc.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeEventWebhookError(w, "get event webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.eventWebhookResource(r.Context(), svc, h))
}

func (rt *Router) handleCreateEventWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	var req createEventWebhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	h, secret, err := svc.Create(r.Context(), eventhook.Input{Name: req.Name, URL: req.URL, Enabled: enabled, Events: req.Events})
	if err != nil {
		rt.writeEventWebhookError(w, "create event webhook", err)
		return
	}
	res := rt.eventWebhookResource(r.Context(), svc, h)
	res.SigningSecret = secret
	writeJSON(w, http.StatusCreated, res)
}

func (rt *Router) handleUpdateEventWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	var req updateEventWebhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	h, err := svc.Update(r.Context(), r.PathValue("id"), eventhook.Patch{Name: req.Name, URL: req.URL, Enabled: req.Enabled, Events: req.Events})
	if err != nil {
		rt.writeEventWebhookError(w, "update event webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.eventWebhookResource(r.Context(), svc, h))
}

func (rt *Router) handleDeleteEventWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	if err := svc.Delete(r.Context(), r.PathValue("id")); err != nil {
		rt.writeEventWebhookError(w, "delete event webhook", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleRotateEventWebhookSecret(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	secret, err := svc.RotateSecret(r.Context(), id)
	if err != nil {
		rt.writeEventWebhookError(w, "rotate event webhook secret", err)
		return
	}
	h, err := svc.Get(r.Context(), id)
	if err != nil {
		rt.writeEventWebhookError(w, "rotate event webhook secret", err)
		return
	}
	res := rt.eventWebhookResource(r.Context(), svc, h)
	res.SigningSecret = secret
	writeJSON(w, http.StatusOK, res)
}

// handleTestEventWebhook sends one probe. A delivery failure is the answer,
// not a server error, so it returns 200 with ok=false.
func (rt *Router) handleTestEventWebhook(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, err := svc.Get(r.Context(), id); err != nil {
		rt.writeEventWebhookError(w, "test event webhook", err)
		return
	}
	status, err := svc.Test(r.Context(), id)
	if err != nil {
		msg := err.Error()
		if len(msg) > 300 {
			msg = msg[:300]
		}
		writeJSON(w, http.StatusOK, eventWebhookTestResponse{OK: false, Status: status, Error: msg})
		return
	}
	writeJSON(w, http.StatusOK, eventWebhookTestResponse{OK: true, Status: status})
}

func toEventDeliveryResource(d store.EventWebhookDelivery) eventDeliveryResource {
	return eventDeliveryResource{
		ID: d.ID, WebhookID: d.WebhookID, EventID: d.EventID, EventType: d.EventType, State: d.State,
		Attempts: d.Attempts, ResponseStatus: d.ResponseStatus, LastError: d.LastError,
		NextAttemptAt: d.NextAttemptAt, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func (rt *Router) handleListEventWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	limit := 50
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	ds, err := svc.Deliveries(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		rt.writeEventWebhookError(w, "list event webhook deliveries", err)
		return
	}
	out := make([]eventDeliveryResource, len(ds))
	for i, d := range ds {
		out[i] = toEventDeliveryResource(d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleReplayEventWebhookDelivery(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.eventWebhooksOrNotImplemented(w)
	if !ok {
		return
	}
	d, err := svc.Replay(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeEventWebhookError(w, "replay event webhook delivery", err)
		return
	}
	writeJSON(w, http.StatusOK, toEventDeliveryResource(d))
}

func (rt *Router) handleListEventWebhookTypes(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, eventhook.EventTypes())
}
