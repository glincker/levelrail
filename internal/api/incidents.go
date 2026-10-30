package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/incident"
	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	incidentSourceAlert = "alert"
	maxIncidentTitle    = 140
	incidentListLimit   = 200
)

// IncidentStore is the persistence the internal incident routes need. *store.DB satisfies it.
type IncidentStore interface {
	CreateInternalStatusIncident(ctx context.Context, in store.InternalStatusIncident) error
	GetInternalStatusIncident(ctx context.Context, id string) (store.InternalStatusIncident, error)
	ListInternalStatusIncidents(ctx context.Context, limit int) ([]store.InternalStatusIncident, error)
	SetStatusIncidentInternalState(ctx context.Context, id, state, by string, at time.Time) error
	AddInternalIncidentUpdate(ctx context.Context, u store.StatusIncidentUpdate, visibility, author string) error
	ListIncidentTimeline(ctx context.Context, incidentID string) ([]store.IncidentTimelineEntry, error)
}

// SetIncidents enables the internal incident routes. bodyMax caps a note in bytes.
func (rt *Router) SetIncidents(s IncidentStore, bodyMax int) {
	if bodyMax <= 0 {
		bodyMax = incident.DefaultUpdateBodyMax
	}
	rt.incidents, rt.incidentBodyMax = s, bodyMax
}

// incidentsResource keeps the status-page resource; the read:sensitive ability is what keeps viewers out.
func incidentsResource(*http.Request) string { return "status-page" }

func (rt *Router) incidentsReady(w http.ResponseWriter) bool {
	if rt.incidents == nil {
		writeError(w, http.StatusNotImplemented, "internal incidents are not enabled (APP_MONITORS_ENABLED and APP_INCIDENTS_ENABLED)")
		return false
	}
	return true
}

type incidentTimelineResource struct {
	Status     string    `json:"status"`
	Body       string    `json:"body"`
	Visibility string    `json:"visibility"`
	Author     string    `json:"author,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type incidentResource struct {
	ID             string                     `json:"id"`
	Title          string                     `json:"title"`
	Status         string                     `json:"status"`
	Impact         string                     `json:"impact"`
	Source         string                     `json:"source"`
	SourceRef      string                     `json:"source_ref,omitempty"`
	InternalState  string                     `json:"internal_state"`
	Published      bool                       `json:"published"`
	ComponentIDs   []string                   `json:"component_ids"`
	StartsAt       time.Time                  `json:"starts_at"`
	AcknowledgedAt *time.Time                 `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string                     `json:"acknowledged_by,omitempty"`
	ResolvedAt     *time.Time                 `json:"internal_resolved_at,omitempty"`
	Timeline       []incidentTimelineResource `json:"timeline,omitempty"`
}

// incidentStateResource is what a mutation returns, so a write-only token never reads incident text back.
type incidentStateResource struct {
	ID            string `json:"id"`
	InternalState string `json:"internal_state"`
}

func toIncidentResource(i store.InternalStatusIncident) incidentResource {
	return incidentResource{ID: i.ID, Title: i.Title, Status: i.Status, Impact: i.Impact, Source: i.Source, SourceRef: i.SourceRef,
		InternalState: i.InternalState, Published: i.Published, ComponentIDs: nonNilStrings(i.ComponentIDs), StartsAt: i.StartsAt.UTC(),
		AcknowledgedAt: i.AcknowledgedAt, AcknowledgedBy: i.AcknowledgedBy, ResolvedAt: i.InternalResolvedAt}
}

func (rt *Router) incidentActor(r *http.Request) string {
	_, id, _, err := rt.callerPrincipal(r)
	if err != nil {
		return ""
	}
	return id
}

func (rt *Router) handleListIncidents(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	list, err := rt.incidents.ListInternalStatusIncidents(r.Context(), incidentListLimit)
	if err != nil {
		rt.internalError(w, "api: list incidents failed", err)
		return
	}
	state := r.URL.Query().Get("state")
	out := make([]incidentResource, 0, len(list))
	for _, i := range list {
		if state == "" || i.InternalState == state {
			out = append(out, toIncidentResource(i))
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// loadIncident writes the 404 or 500 itself and reports whether the incident was found.
func (rt *Router) loadIncident(w http.ResponseWriter, r *http.Request) (store.InternalStatusIncident, bool) {
	inc, err := rt.incidents.GetInternalStatusIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrStatusIncidentNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return inc, false
	}
	if err != nil {
		rt.internalError(w, "api: load incident failed", err)
		return inc, false
	}
	return inc, true
}

func (rt *Router) handleGetIncident(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	inc, ok := rt.loadIncident(w, r)
	if !ok {
		return
	}
	entries, err := rt.incidents.ListIncidentTimeline(r.Context(), inc.ID)
	if err != nil {
		rt.internalError(w, "api: load incident timeline failed", err)
		return
	}
	res := toIncidentResource(inc)
	res.Timeline = make([]incidentTimelineResource, 0, len(entries))
	for _, e := range entries {
		res.Timeline = append(res.Timeline, incidentTimelineResource{Status: e.Status, Body: e.Body, Visibility: e.Visibility, Author: e.Author, CreatedAt: e.CreatedAt.UTC()})
	}
	writeJSON(w, http.StatusOK, res)
}

type promoteIncidentRequest struct {
	Source string `json:"source"`
	RuleID string `json:"rule_id"`
	Title  string `json:"title"`
}

// handlePromoteIncident turns a firing alert rule into an unpublished internal incident.
func (rt *Router) handlePromoteIncident(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	var req promoteIncidentRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Source != incidentSourceAlert || req.RuleID == "" {
		writeError(w, http.StatusBadRequest, "source must be alert and rule_id is required")
		return
	}
	if len(req.Title) > maxIncidentTitle {
		writeError(w, http.StatusBadRequest, "title is too long")
		return
	}
	if rt.alertRules == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return
	}
	rule, err := rt.alertRules.GetRule(r.Context(), req.RuleID)
	if err != nil || rule == nil {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}
	if req.Title == "" {
		req.Title = "Alert: " + rule.Name
	}
	id, err := store.NewStatusID("si_")
	if err != nil {
		rt.internalError(w, "api: promote incident: mint id failed", err)
		return
	}
	now := time.Now().UTC()
	in := store.InternalStatusIncident{
		StatusIncident: store.StatusIncident{ID: id, Kind: statuspage.IncidentKind, Title: req.Title, Status: statuspage.StatusInvestigating,
			Impact: statuspage.ImpactNone, StartsAt: now},
		StatusIncidentInternal: store.StatusIncidentInternal{Source: incidentSourceAlert, SourceRef: rule.ID, InternalState: store.IncidentStateTriggered},
	}
	if err := rt.incidents.CreateInternalStatusIncident(r.Context(), in); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "this alert already has an open incident")
			return
		}
		rt.internalError(w, "api: promote incident failed", err)
		return
	}
	rt.addInternalNote(r, id, "Promoted from alert rule "+rule.Name, now)
	writeJSON(w, http.StatusCreated, incidentStateResource{ID: id, InternalState: store.IncidentStateTriggered})
}

// addInternalNote is best effort: the state change it records has already been saved.
func (rt *Router) addInternalNote(r *http.Request, incidentID, body string, at time.Time) {
	id, err := store.NewStatusID("su_")
	if err != nil {
		return
	}
	u := store.StatusIncidentUpdate{ID: id, IncidentID: incidentID, Status: statuspage.StatusInvestigating, Body: body, CreatedAt: at}
	if err := rt.incidents.AddInternalIncidentUpdate(r.Context(), u, store.UpdateVisibilityInternal, rt.incidentActor(r)); err != nil {
		rt.logger.Warn("api: add incident note failed", "incident", incidentID, "error", err.Error())
	}
}

func (rt *Router) handleAckIncident(w http.ResponseWriter, r *http.Request) {
	rt.transitionIncident(w, r, store.IncidentStateAcknowledged, "Acknowledged")
}

func (rt *Router) handleResolveIncident(w http.ResponseWriter, r *http.Request) {
	rt.transitionIncident(w, r, store.IncidentStateResolved, "Resolved by operator")
}

func (rt *Router) transitionIncident(w http.ResponseWriter, r *http.Request, to, note string) {
	if !rt.incidentsReady(w) {
		return
	}
	inc, ok := rt.loadIncident(w, r)
	if !ok {
		return
	}
	if inc.InternalState == store.IncidentStateResolved || inc.InternalState == to {
		writeError(w, http.StatusConflict, "incident is already "+inc.InternalState)
		return
	}
	now := time.Now().UTC()
	if err := rt.incidents.SetStatusIncidentInternalState(r.Context(), inc.ID, to, rt.incidentActor(r), now); err != nil {
		if errors.Is(err, store.ErrStatusIncidentNotFound) {
			writeError(w, http.StatusNotFound, "incident not found")
			return
		}
		rt.internalError(w, "api: incident transition failed", err)
		return
	}
	rt.addInternalNote(r, inc.ID, note, now)
	writeJSON(w, http.StatusOK, incidentStateResource{ID: inc.ID, InternalState: to})
}

type incidentNoteRequest struct {
	Body string `json:"body"`
}

func (rt *Router) handleAddIncidentNote(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	inc, ok := rt.loadIncident(w, r)
	if !ok {
		return
	}
	var req incidentNoteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(rt.incidentBodyMax)+4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" || len(req.Body) > rt.incidentBodyMax {
		writeError(w, http.StatusBadRequest, "body is required and must fit APP_INCIDENT_UPDATE_BODY_MAX")
		return
	}
	id, err := store.NewStatusID("su_")
	if err != nil {
		rt.internalError(w, "api: incident note: mint id failed", err)
		return
	}
	u := store.StatusIncidentUpdate{ID: id, IncidentID: inc.ID, Status: inc.Status, Body: req.Body, CreatedAt: time.Now().UTC()}
	if err := rt.incidents.AddInternalIncidentUpdate(r.Context(), u, store.UpdateVisibilityInternal, rt.incidentActor(r)); err != nil {
		rt.internalError(w, "api: incident note failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, incidentStateResource{ID: inc.ID, InternalState: inc.InternalState})
}
