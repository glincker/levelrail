package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

type incidentDraftRequest struct {
	Title  string `json:"title"`
	Status string `json:"status"`
	Body   string `json:"body"`
}

// handleSaveIncidentDraft stores operator text for the public page; nothing public changes until publish.
func (rt *Router) handleSaveIncidentDraft(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	inc, ok := rt.loadIncident(w, r)
	if !ok {
		return
	}
	if inc.Published {
		writeError(w, http.StatusConflict, "incident is published; post a public update instead")
		return
	}
	var req incidentDraftRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(rt.incidentBodyMax)+4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title, req.Body = strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if req.Status == "" {
		req.Status = inc.Status
	}
	if len(req.Title) > maxIncidentTitle || len(req.Body) > rt.incidentBodyMax {
		writeError(w, http.StatusBadRequest, "title or body is too long")
		return
	}
	if !validIncidentStatus(statuspage.IncidentKind, req.Status) {
		writeError(w, http.StatusBadRequest, "status is not valid for an incident")
		return
	}
	err := rt.incidents.SaveStatusIncidentDraftFields(r.Context(), inc.ID, req.Title, req.Status, req.Body)
	switch {
	case errors.Is(err, store.ErrStatusIncidentNotFound):
		writeError(w, http.StatusNotFound, "incident not found")
		return
	case errors.Is(err, store.ErrIncidentPublished):
		writeError(w, http.StatusConflict, "incident is published; post a public update instead")
		return
	case err != nil:
		rt.internalError(w, "api: save incident draft failed", err)
		return
	}
	rt.addInternalNote(r, inc.ID, "Public draft saved", time.Now().UTC())
	writeJSON(w, http.StatusOK, incidentStateResource{ID: inc.ID, InternalState: inc.InternalState})
}

type incidentPublishRequest struct {
	UpdatedAt time.Time `json:"updated_at"`
}

type incidentPublishResource struct {
	ID          string    `json:"id"`
	Published   bool      `json:"published"`
	PublishedAt time.Time `json:"published_at"`
}

// handlePublishIncident is the only way an internal incident reaches the public page: an explicit,
// audited operator request carrying the updated_at it was shown. There is no unpublish (ADR 061).
func (rt *Router) handlePublishIncident(w http.ResponseWriter, r *http.Request) {
	if !rt.incidentsReady(w) {
		return
	}
	var req incidentPublishRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil || req.UpdatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "updated_at of the incident you reviewed is required")
		return
	}
	updID, err := store.NewStatusID("su_")
	if err != nil {
		rt.internalError(w, "api: publish incident: mint id failed", err)
		return
	}
	now := time.Now().UTC()
	inc, err := rt.incidents.PublishStatusIncident(r.Context(), r.PathValue("id"), req.UpdatedAt.UTC(), rt.incidentActor(r), updID, now)
	switch {
	case errors.Is(err, store.ErrStatusIncidentNotFound):
		writeError(w, http.StatusNotFound, "incident not found")
		return
	case errors.Is(err, store.ErrIncidentPublished):
		writeError(w, http.StatusConflict, "incident is already published")
		return
	case errors.Is(err, store.ErrIncidentStale):
		writeError(w, http.StatusConflict, "incident changed since you loaded it; reload and review the draft")
		return
	case errors.Is(err, store.ErrIncidentDraftEmpty):
		writeError(w, http.StatusUnprocessableEntity, "draft title and body are required before publishing")
		return
	case err != nil:
		rt.internalError(w, "api: publish incident failed", err)
		return
	}
	rt.addInternalNote(r, inc.ID, "Published to the public status page", now)
	if rt.statusView != nil {
		rt.statusView.Invalidate()
	}
	writeJSON(w, http.StatusOK, incidentPublishResource{ID: inc.ID, Published: true, PublishedAt: now})
}
