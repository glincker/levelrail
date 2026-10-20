package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dashboards"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxDashboardBodyBytes = 256 << 10

// DashboardStore persists custom dashboards.
type DashboardStore interface {
	CreateCustomDashboard(ctx context.Context, d store.CustomDashboard) error
	UpdateCustomDashboard(ctx context.Context, d store.CustomDashboard) error
	GetCustomDashboard(ctx context.Context, id string) (store.CustomDashboard, error)
	ListCustomDashboards(ctx context.Context, owner, projectID string) ([]store.CustomDashboard, error)
	DeleteCustomDashboard(ctx context.Context, id string) error
}

type dashboardResource struct {
	ID        string         `json:"id"`
	Owner     string         `json:"owner"`
	Scope     string         `json:"scope"`
	ProjectID string         `json:"project_id,omitempty"`
	IsDefault bool           `json:"is_default"`
	Name      string         `json:"name"`
	Editable  bool           `json:"editable"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Doc       dashboards.Doc `json:"doc"`
}

type dashboardRequest struct {
	Scope     string         `json:"scope"`
	ProjectID string         `json:"project_id"`
	IsDefault bool           `json:"is_default"`
	Doc       dashboards.Doc `json:"doc"`
}

func (rt *Router) dashboardsReady(w http.ResponseWriter, r *http.Request) (string, bool) {
	if rt.dashboards == nil {
		writeError(w, http.StatusNotImplemented, "custom dashboards are not available on this control plane")
		return "", false
	}
	owner, ok := rt.viewOwner(r)
	if !ok {
		writeError(w, http.StatusForbidden, "dashboards need a signed-in user or a user token")
		return "", false
	}
	return owner, true
}

func toDashboardResource(d store.CustomDashboard, caller string) (dashboardResource, error) {
	var doc dashboards.Doc
	if err := json.Unmarshal([]byte(d.Body), &doc); err != nil {
		return dashboardResource{}, err
	}
	if doc.Widgets == nil {
		doc.Widgets = []dashboards.Widget{}
	}
	return dashboardResource{
		ID: d.ID, Owner: d.Owner, Scope: d.Scope, ProjectID: d.ProjectID, IsDefault: d.IsDefault, Name: d.Name,
		Editable:  d.Scope == dashboards.ScopeShared || d.Owner == caller,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, Doc: doc,
	}, nil
}

func visibleTo(d store.CustomDashboard, caller string) bool {
	return d.Scope == dashboards.ScopeShared || d.Owner == caller
}

func (rt *Router) loadDashboard(w http.ResponseWriter, r *http.Request, caller string, mustEdit bool) (store.CustomDashboard, bool) {
	d, err := rt.dashboards.GetCustomDashboard(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrDashboardNotFound) || (err == nil && !visibleTo(d, caller)) {
		writeError(w, http.StatusNotFound, "dashboard not found")
		return store.CustomDashboard{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get dashboard", err)
		return store.CustomDashboard{}, false
	}
	if mustEdit && d.Scope == dashboards.ScopeUser && d.Owner != caller {
		writeError(w, http.StatusForbidden, "only the owner can change this dashboard")
		return store.CustomDashboard{}, false
	}
	return d, true
}

func (rt *Router) handleDashboardTemplates(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, dashboards.Templates())
}

func (rt *Router) handleListDashboards(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	rows, err := rt.dashboards.ListCustomDashboards(r.Context(), owner, r.URL.Query().Get("project"))
	if err != nil {
		rt.internalError(w, "api: list dashboards", err)
		return
	}
	out := make([]dashboardResource, 0, len(rows))
	for _, d := range rows {
		res, err := toDashboardResource(d, owner)
		if err != nil {
			rt.internalError(w, "api: decode dashboard", err)
			return
		}
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleGetDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	d, ok := rt.loadDashboard(w, r, owner, false)
	if !ok {
		return
	}
	res, err := toDashboardResource(d, owner)
	if err != nil {
		rt.internalError(w, "api: decode dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleExportDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	d, ok := rt.loadDashboard(w, r, owner, false)
	if !ok {
		return
	}
	res, err := toDashboardResource(d, owner)
	if err != nil {
		rt.internalError(w, "api: decode dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, res.Doc)
}

func (rt *Router) handleCreateDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	var req dashboardRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDashboardBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rt.createDashboard(w, r, owner, req)
}

func (rt *Router) handleImportDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxDashboardBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	doc, err := dashboards.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	q := r.URL.Query()
	rt.createDashboard(w, r, owner, dashboardRequest{Scope: q.Get("scope"), ProjectID: q.Get("project"), Doc: doc})
}

func (rt *Router) createDashboard(w http.ResponseWriter, r *http.Request, owner string, req dashboardRequest) {
	body, ok := rt.checkDashboardRequest(w, &req)
	if !ok {
		return
	}
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown project"), projectRef(req.ProjectID)) {
		return
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		rt.internalError(w, "api: create dashboard: id", err)
		return
	}
	now := time.Now().UTC()
	d := store.CustomDashboard{
		ID: "dsh_" + hex.EncodeToString(buf), Owner: owner, Scope: req.Scope, ProjectID: req.ProjectID,
		IsDefault: req.IsDefault, Name: req.Doc.Name, Body: body, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.dashboards.CreateCustomDashboard(r.Context(), d); err != nil {
		rt.dashboardWriteFailed(w, err)
		return
	}
	res, err := toDashboardResource(d, owner)
	if err != nil {
		rt.internalError(w, "api: decode dashboard", err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (rt *Router) handleUpdateDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	existing, ok := rt.loadDashboard(w, r, owner, true)
	if !ok {
		return
	}
	var req dashboardRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDashboardBodyBytes)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body, ok := rt.checkDashboardRequest(w, &req)
	if !ok {
		return
	}
	if !rt.authorizeRefs(w, r, AbilityRead, notFoundAs(http.StatusBadRequest, "unknown project"), projectRef(req.ProjectID)) {
		return
	}
	existing.Scope, existing.ProjectID, existing.IsDefault = req.Scope, req.ProjectID, req.IsDefault
	existing.Name, existing.Body, existing.UpdatedAt = req.Doc.Name, body, time.Now().UTC()
	if err := rt.dashboards.UpdateCustomDashboard(r.Context(), existing); err != nil {
		rt.dashboardWriteFailed(w, err)
		return
	}
	res, err := toDashboardResource(existing, owner)
	if err != nil {
		rt.internalError(w, "api: decode dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleDeleteDashboard(w http.ResponseWriter, r *http.Request) {
	owner, ok := rt.dashboardsReady(w, r)
	if !ok {
		return
	}
	d, ok := rt.loadDashboard(w, r, owner, true)
	if !ok {
		return
	}
	if err := rt.dashboards.DeleteCustomDashboard(r.Context(), d.ID); err != nil && !errors.Is(err, store.ErrDashboardNotFound) {
		rt.internalError(w, "api: delete dashboard", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) checkDashboardRequest(w http.ResponseWriter, req *dashboardRequest) (string, bool) {
	if req.Scope == "" {
		req.Scope = dashboards.ScopeUser
	}
	req.Doc.Version = dashboards.Version
	req.Doc.Name = strings.TrimSpace(req.Doc.Name)
	if req.Doc.Widgets == nil {
		req.Doc.Widgets = []dashboards.Widget{}
	}
	if req.Scope != dashboards.ScopeUser && req.Scope != dashboards.ScopeShared {
		writeError(w, http.StatusBadRequest, "scope must be user or shared")
		return "", false
	}
	if req.IsDefault && (req.Scope != dashboards.ScopeShared || req.ProjectID == "") {
		writeError(w, http.StatusBadRequest, "a default dashboard must be shared and belong to a project")
		return "", false
	}
	if err := req.Doc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	raw, err := json.Marshal(req.Doc)
	if err != nil {
		rt.internalError(w, "api: encode dashboard", err)
		return "", false
	}
	return string(raw), true
}

func (rt *Router) dashboardWriteFailed(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrDashboardNameTaken):
		writeError(w, http.StatusConflict, "a dashboard with this name already exists")
	case errors.Is(err, store.ErrDashboardNotFound):
		writeError(w, http.StatusNotFound, "dashboard not found")
	default:
		rt.internalError(w, "api: save dashboard", err)
	}
}
