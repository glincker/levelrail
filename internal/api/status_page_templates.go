package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

// statusTemplateStore is optional so fakes of StatusPageStore keep compiling; *store.DB satisfies it.
type statusTemplateStore interface {
	ListStatusIncidentTemplates(ctx context.Context) ([]store.StatusIncidentTemplate, error)
	GetStatusIncidentTemplate(ctx context.Context, id string) (store.StatusIncidentTemplate, error)
	SaveStatusIncidentTemplate(ctx context.Context, t store.StatusIncidentTemplate) error
	DeleteStatusIncidentTemplate(ctx context.Context, id string) error
}

type statusTemplateResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Impact    string    `json:"impact"`
	Body      string    `json:"body"`
	UpdatedAt time.Time `json:"updated_at"`
}

type statusTemplateRequest struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Impact string `json:"impact"`
	Body   string `json:"body"`
}

func toStatusTemplateResource(t store.StatusIncidentTemplate) statusTemplateResource {
	return statusTemplateResource{ID: t.ID, Name: t.Name, Title: t.Title, Status: t.Status, Impact: t.Impact, Body: t.Body, UpdatedAt: t.UpdatedAt.UTC()}
}

func (rt *Router) statusTemplatesReady(w http.ResponseWriter) bool {
	if !rt.statusPageReady(w) {
		return false
	}
	if rt.statusTemplates == nil {
		writeError(w, http.StatusNotImplemented, "incident templates are not available on this control plane")
		return false
	}
	return true
}

func (rt *Router) templatesMax(ctx context.Context) int {
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyStatusTemplatesMax)
}

// validateStatusTemplate normalizes the request with the same sets an incident uses, so a template can always be applied.
func validateStatusTemplate(req *statusTemplateRequest) string {
	req.Name, req.Title, req.Body = strings.TrimSpace(req.Name), strings.TrimSpace(req.Title), strings.TrimSpace(req.Body)
	if req.Name == "" || len(req.Name) > maxStatusTitle {
		return "name is required (at most 140 characters)"
	}
	if req.Title == "" || len(req.Title) > maxStatusTitle {
		return "title is required (at most 140 characters)"
	}
	if len(req.Body) > maxStatusBody {
		return "body is too long"
	}
	if req.Impact == "" {
		req.Impact = statuspage.ImpactNone
	}
	switch req.Impact {
	case statuspage.ImpactNone, statuspage.ImpactMinor, statuspage.ImpactMajor, statuspage.ImpactCritical:
	default:
		return "impact must be none, minor, major or critical"
	}
	if req.Status == "" {
		req.Status = statuspage.StatusInvestigating
	}
	if !validIncidentStatus(statuspage.IncidentKind, req.Status) {
		return "status is not valid for an incident"
	}
	return ""
}

func (rt *Router) statusTemplateError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, store.ErrStatusTemplateNotFound):
		writeError(w, http.StatusNotFound, "template not found")
	case errors.Is(err, store.ErrStatusTemplateName):
		writeError(w, http.StatusConflict, "a template with this name already exists")
	default:
		rt.internalError(w, "api: status incident template "+what+" failed", err)
	}
}

func (rt *Router) handleListStatusTemplates(w http.ResponseWriter, r *http.Request) {
	if !rt.statusTemplatesReady(w) {
		return
	}
	list, err := rt.statusTemplates.ListStatusIncidentTemplates(r.Context())
	if err != nil {
		rt.statusTemplateError(w, "list", err)
		return
	}
	out := make([]statusTemplateResource, 0, len(list))
	for _, t := range list {
		out = append(out, toStatusTemplateResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateStatusTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.statusTemplatesReady(w) {
		return
	}
	var req statusTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateStatusTemplate(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	existing, err := rt.statusTemplates.ListStatusIncidentTemplates(r.Context())
	if err != nil {
		rt.statusTemplateError(w, "create", err)
		return
	}
	if len(existing) >= rt.templatesMax(r.Context()) {
		writeError(w, http.StatusConflict, "too many incident templates")
		return
	}
	id, err := store.NewStatusID("st_")
	if err != nil {
		rt.internalError(w, "api: create status incident template: mint id failed", err)
		return
	}
	t := store.StatusIncidentTemplate{ID: id, Name: req.Name, Title: req.Title, Status: req.Status, Impact: req.Impact, Body: req.Body}
	if err := rt.statusTemplates.SaveStatusIncidentTemplate(r.Context(), t); err != nil {
		rt.statusTemplateError(w, "create", err)
		return
	}
	saved, err := rt.statusTemplates.GetStatusIncidentTemplate(r.Context(), id)
	if err != nil {
		rt.statusTemplateError(w, "create", err)
		return
	}
	writeJSON(w, http.StatusCreated, toStatusTemplateResource(saved))
}

func (rt *Router) handleUpdateStatusTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.statusTemplatesReady(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.statusTemplates.GetStatusIncidentTemplate(r.Context(), id); err != nil {
		rt.statusTemplateError(w, "update", err)
		return
	}
	var req statusTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := validateStatusTemplate(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	t := store.StatusIncidentTemplate{ID: id, Name: req.Name, Title: req.Title, Status: req.Status, Impact: req.Impact, Body: req.Body}
	if err := rt.statusTemplates.SaveStatusIncidentTemplate(r.Context(), t); err != nil {
		rt.statusTemplateError(w, "update", err)
		return
	}
	saved, err := rt.statusTemplates.GetStatusIncidentTemplate(r.Context(), id)
	if err != nil {
		rt.statusTemplateError(w, "update", err)
		return
	}
	writeJSON(w, http.StatusOK, toStatusTemplateResource(saved))
}

func (rt *Router) handleDeleteStatusTemplate(w http.ResponseWriter, r *http.Request) {
	if !rt.statusTemplatesReady(w) {
		return
	}
	if err := rt.statusTemplates.DeleteStatusIncidentTemplate(r.Context(), r.PathValue("id")); err != nil {
		rt.statusTemplateError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
