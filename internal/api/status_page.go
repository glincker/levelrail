package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxStatusComponents = 100
	maxStatusBody       = 4000
	maxStatusTitle      = 140
	maxDisplayName      = 80
)

var statusDomainRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)+$`)

// StatusPageStore is the persistence surface of the status page handlers. *store.DB satisfies it.
type StatusPageStore interface {
	GetStatusPageSettings(ctx context.Context) (store.StatusPageSettings, error)
	SaveStatusPageSettings(ctx context.Context, s store.StatusPageSettings) error
	ListStatusComponents(ctx context.Context) ([]store.StatusComponent, error)
	GetStatusComponent(ctx context.Context, id string) (store.StatusComponent, error)
	SaveStatusComponent(ctx context.Context, c store.StatusComponent) error
	DeleteStatusComponent(ctx context.Context, id string) error
	SaveStatusIncident(ctx context.Context, i store.StatusIncident) error
	GetStatusIncident(ctx context.Context, id string) (store.StatusIncident, error)
	ListStatusIncidents(ctx context.Context, limit int) ([]store.StatusIncident, error)
	DeleteStatusIncident(ctx context.Context, id string) error
	AddStatusIncidentUpdate(ctx context.Context, u store.StatusIncidentUpdate) error
	ListStatusIncidentUpdates(ctx context.Context) ([]store.StatusIncidentUpdate, error)
}

// StatusPageViewer builds the public view. *statuspage.Service satisfies it.
type StatusPageViewer interface {
	View(ctx context.Context) (statuspage.View, error)
	PreviewView(ctx context.Context) (statuspage.View, error)
	Enabled(ctx context.Context) (bool, error)
	Invalidate()
}

// StatusPageDB is everything the status page needs from the database.
type StatusPageDB interface {
	StatusPageStore
	statuspage.Store
}

// WithStatusPage enables the status page management routes, the public
// page and the health sampler (see RunStatusPageSampler). perMinute is
// the per-client-IP budget on public reads.
func WithStatusPage(db StatusPageDB, cfg statuspage.Config, perMinute int) Option {
	return func(rt *Router) {
		svc := statuspage.New(db, statusAppSource{rt}, rt, cfg, rt.logger)
		rt.statusPage, rt.statusView, rt.statusSampler = db, svc, svc
		rt.statusLimiter = newAPIRateLimiter(perMinute)
	}
}

// WithStatusPageViewer wires a custom view builder, for tests.
func WithStatusPageViewer(st StatusPageStore, v StatusPageViewer, perMinute int) Option {
	return func(rt *Router) {
		rt.statusPage, rt.statusView = st, v
		rt.statusLimiter = newAPIRateLimiter(perMinute)
	}
}

// RunStatusPageSampler probes status page components until ctx ends. It
// returns nil at once when the status page is not configured.
func (rt *Router) RunStatusPageSampler(ctx context.Context) error {
	if rt.statusSampler == nil {
		return nil
	}
	return rt.statusSampler.Run(ctx)
}

type statusAppSource struct{ rt *Router }

func (s statusAppSource) AppStatus(ctx context.Context, name string) (string, error) {
	return s.rt.StatusPageAppStatus(ctx, name)
}

func statusPageResource(*http.Request) string { return "status-page" }

func (rt *Router) statusPageReady(w http.ResponseWriter) bool {
	if rt.statusPage == nil || rt.statusView == nil {
		writeError(w, http.StatusNotImplemented, "the status page is not configured on this control plane")
		return false
	}
	return true
}

type statusSettingsResource struct {
	Enabled      bool   `json:"enabled"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CustomDomain string `json:"custom_domain"`
	PublicPath   string `json:"public_path"`
}

func (rt *Router) handleGetStatusPage(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	s, err := rt.statusPage.GetStatusPageSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get status page settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, statusSettingsResource{Enabled: s.Enabled, Title: s.Title, Description: s.Description,
		CustomDomain: s.CustomDomain, PublicPath: statuspage.PagePath})
}

func (rt *Router) handlePutStatusPage(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	var req statusSettingsResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Title, req.Description = strings.TrimSpace(req.Title), strings.TrimSpace(req.Description)
	req.CustomDomain = strings.ToLower(strings.TrimSpace(req.CustomDomain))
	switch {
	case len(req.Title) > maxStatusTitle:
		writeError(w, http.StatusBadRequest, "title is too long")
		return
	case len(req.Description) > 500:
		writeError(w, http.StatusBadRequest, "description is too long")
		return
	case req.CustomDomain != "" && !statusDomainRe.MatchString(req.CustomDomain):
		writeError(w, http.StatusBadRequest, "custom_domain must be a plain hostname such as status.example.com")
		return
	}
	err := rt.statusPage.SaveStatusPageSettings(r.Context(), store.StatusPageSettings{Enabled: req.Enabled, Title: req.Title,
		Description: req.Description, CustomDomain: req.CustomDomain})
	if err != nil {
		rt.internalError(w, "api: save status page settings failed", err)
		return
	}
	rt.statusView.Invalidate()
	rt.statusHost.invalidate()
	rt.handleGetStatusPage(w, r)
}

func (rt *Router) handleStatusPagePreview(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	v, err := rt.statusView.PreviewView(r.Context())
	if err != nil {
		rt.internalError(w, "api: status page preview failed", err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type statusComponentResource struct {
	ID          string `json:"id,omitempty"`
	Kind        string `json:"kind"`
	Target      string `json:"target"`
	DisplayName string `json:"display_name"`
	Position    int    `json:"position"`
}

func toStatusComponentResource(c store.StatusComponent) statusComponentResource {
	return statusComponentResource{ID: c.ID, Kind: c.Kind, Target: c.Target, DisplayName: c.DisplayName, Position: c.Position}
}

func (rt *Router) validateStatusComponent(ctx context.Context, req *statusComponentResource) string {
	req.DisplayName, req.Target = strings.TrimSpace(req.DisplayName), strings.TrimSpace(req.Target)
	if req.DisplayName == "" || len(req.DisplayName) > maxDisplayName {
		return "display_name is required (at most 80 characters): it is the only name shown publicly"
	}
	switch req.Kind {
	case statuspage.KindApp:
		if _, err := rt.apps.GetDesiredService(ctx, req.Target); errors.Is(err, store.ErrServiceNotFound) {
			return "unknown app"
		} else if err != nil {
			return "could not verify the app"
		}
	case statuspage.KindDomain:
		req.Target = strings.ToLower(req.Target)
		if !statusDomainRe.MatchString(req.Target) {
			return "target must be a hostname such as example.com"
		}
	case statuspage.KindCheck:
		u, err := url.Parse(req.Target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil {
			return "target must be an http or https URL without credentials"
		}
	default:
		return "kind must be app, domain or check"
	}
	return ""
}

func (rt *Router) handleListStatusComponents(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	list, err := rt.statusPage.ListStatusComponents(r.Context())
	if err != nil {
		rt.internalError(w, "api: list status components failed", err)
		return
	}
	out := make([]statusComponentResource, 0, len(list))
	for _, c := range list {
		out = append(out, toStatusComponentResource(c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) saveStatusComponent(w http.ResponseWriter, r *http.Request, id string, status int) {
	var req statusComponentResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := rt.validateStatusComponent(r.Context(), &req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	c := store.StatusComponent{ID: id, Kind: req.Kind, Target: req.Target, DisplayName: req.DisplayName, Position: req.Position}
	if err := rt.statusPage.SaveStatusComponent(r.Context(), c); err != nil {
		rt.internalError(w, "api: save status component failed", err)
		return
	}
	rt.statusView.Invalidate()
	writeJSON(w, status, toStatusComponentResource(c))
}

func (rt *Router) handleCreateStatusComponent(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	existing, err := rt.statusPage.ListStatusComponents(r.Context())
	if err != nil {
		rt.internalError(w, "api: create status component: list failed", err)
		return
	}
	if len(existing) >= maxStatusComponents {
		writeError(w, http.StatusBadRequest, "too many components")
		return
	}
	id, err := store.NewStatusID("sc_")
	if err != nil {
		rt.internalError(w, "api: create status component: mint id failed", err)
		return
	}
	rt.saveStatusComponent(w, r, id, http.StatusCreated)
}

func (rt *Router) handleUpdateStatusComponent(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.statusPage.GetStatusComponent(r.Context(), id); errors.Is(err, store.ErrStatusComponentNotFound) {
		writeError(w, http.StatusNotFound, "component not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update status component: load failed", err)
		return
	}
	rt.saveStatusComponent(w, r, id, http.StatusOK)
}

func (rt *Router) handleDeleteStatusComponent(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	if err := rt.statusPage.DeleteStatusComponent(r.Context(), r.PathValue("id")); err != nil {
		rt.internalError(w, "api: delete status component failed", err)
		return
	}
	rt.statusView.Invalidate()
	w.WriteHeader(http.StatusNoContent)
}

type statusIncidentResource struct {
	ID           string                 `json:"id,omitempty"`
	Kind         string                 `json:"kind"`
	Title        string                 `json:"title"`
	Status       string                 `json:"status"`
	Impact       string                 `json:"impact"`
	ComponentIDs []string               `json:"component_ids"`
	StartsAt     *time.Time             `json:"starts_at,omitempty"`
	EndsAt       *time.Time             `json:"ends_at,omitempty"`
	ResolvedAt   *time.Time             `json:"resolved_at,omitempty"`
	Body         string                 `json:"body,omitempty"`
	Updates      []statusUpdateResource `json:"updates,omitempty"`
}

type statusUpdateResource struct {
	Status    string    `json:"status"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func validIncidentStatus(kind, status string) bool {
	switch kind {
	case statuspage.IncidentKind:
		return status == statuspage.StatusInvestigating || status == statuspage.StatusIdentified ||
			status == statuspage.StatusMonitoring || status == statuspage.StatusResolved
	case statuspage.MaintenanceKind:
		return status == statuspage.StatusScheduled || status == statuspage.StatusInProgress || status == statuspage.StatusCompleted
	default:
		return false
	}
}

func isClosedStatus(status string) bool {
	return status == statuspage.StatusResolved || status == statuspage.StatusCompleted
}

func (rt *Router) handleListStatusIncidents(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	list, err := rt.statusPage.ListStatusIncidents(r.Context(), 100)
	if err != nil {
		rt.internalError(w, "api: list status incidents failed", err)
		return
	}
	updates, err := rt.statusPage.ListStatusIncidentUpdates(r.Context())
	if err != nil {
		rt.internalError(w, "api: list status incident updates failed", err)
		return
	}
	byIncident := map[string][]statusUpdateResource{}
	for _, u := range updates {
		byIncident[u.IncidentID] = append(byIncident[u.IncidentID], statusUpdateResource{Status: u.Status, Body: u.Body, CreatedAt: u.CreatedAt.UTC()})
	}
	out := make([]statusIncidentResource, 0, len(list))
	for _, i := range list {
		res := toStatusIncidentResource(i)
		res.Updates = byIncident[i.ID]
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

func toStatusIncidentResource(i store.StatusIncident) statusIncidentResource {
	starts := i.StartsAt.UTC()
	return statusIncidentResource{ID: i.ID, Kind: i.Kind, Title: i.Title, Status: i.Status, Impact: i.Impact,
		ComponentIDs: i.ComponentIDs, StartsAt: &starts, EndsAt: i.EndsAt, ResolvedAt: i.ResolvedAt}
}

func (rt *Router) validateIncidentFields(ctx context.Context, res *statusIncidentResource) string {
	res.Title, res.Body = strings.TrimSpace(res.Title), strings.TrimSpace(res.Body)
	if res.Title == "" || len(res.Title) > maxStatusTitle {
		return "title is required (at most 140 characters)"
	}
	if len(res.Body) > maxStatusBody {
		return "body is too long"
	}
	if res.Kind != statuspage.IncidentKind && res.Kind != statuspage.MaintenanceKind {
		return "kind must be incident or maintenance"
	}
	if res.Impact == "" {
		res.Impact = statuspage.ImpactNone
	}
	switch res.Impact {
	case statuspage.ImpactNone, statuspage.ImpactMinor, statuspage.ImpactMajor, statuspage.ImpactCritical:
	default:
		return "impact must be none, minor, major or critical"
	}
	if res.Status == "" {
		res.Status = statuspage.StatusInvestigating
		if res.Kind == statuspage.MaintenanceKind {
			res.Status = statuspage.StatusScheduled
		}
	}
	if !validIncidentStatus(res.Kind, res.Status) {
		return "status is not valid for this kind"
	}
	if res.Kind == statuspage.MaintenanceKind && (res.StartsAt == nil || res.EndsAt == nil || !res.EndsAt.After(*res.StartsAt)) {
		return "maintenance needs starts_at and a later ends_at"
	}
	for _, id := range res.ComponentIDs {
		if _, err := rt.statusPage.GetStatusComponent(ctx, id); err != nil {
			return "unknown component in component_ids"
		}
	}
	return ""
}

func (rt *Router) handleCreateStatusIncident(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	var req statusIncidentResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := rt.validateIncidentFields(r.Context(), &req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	now := time.Now().UTC()
	starts := now
	if req.StartsAt != nil {
		starts = req.StartsAt.UTC()
	}
	id, err := store.NewStatusID("si_")
	if err != nil {
		rt.internalError(w, "api: create status incident: mint id failed", err)
		return
	}
	inc := store.StatusIncident{ID: id, Kind: req.Kind, Title: req.Title, Status: req.Status, Impact: req.Impact,
		ComponentIDs: req.ComponentIDs, StartsAt: starts, EndsAt: req.EndsAt}
	if isClosedStatus(inc.Status) {
		inc.ResolvedAt = &now
	}
	if err := rt.statusPage.SaveStatusIncident(r.Context(), inc); err != nil {
		rt.internalError(w, "api: create status incident failed", err)
		return
	}
	if req.Body != "" {
		if err := rt.addIncidentUpdate(r.Context(), id, req.Status, req.Body, now); err != nil {
			rt.internalError(w, "api: create status incident: add update failed", err)
			return
		}
	}
	rt.statusView.Invalidate()
	writeJSON(w, http.StatusCreated, toStatusIncidentResource(inc))
}

func (rt *Router) addIncidentUpdate(ctx context.Context, incidentID, status, body string, at time.Time) error {
	id, err := store.NewStatusID("su_")
	if err != nil {
		return err
	}
	return rt.statusPage.AddStatusIncidentUpdate(ctx, store.StatusIncidentUpdate{ID: id, IncidentID: incidentID, Status: status, Body: body, CreatedAt: at})
}

// handlePostStatusIncidentUpdate appends a timeline entry and moves the incident to its status.
func (rt *Router) handlePostStatusIncidentUpdate(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	inc, err := rt.statusPage.GetStatusIncident(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrStatusIncidentNotFound) {
		writeError(w, http.StatusNotFound, "incident not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: status incident update: load failed", err)
		return
	}
	var req statusUpdateResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" || len(req.Body) > maxStatusBody {
		writeError(w, http.StatusBadRequest, "body is required (at most 4000 characters)")
		return
	}
	if !validIncidentStatus(inc.Kind, req.Status) {
		writeError(w, http.StatusBadRequest, "status is not valid for this kind")
		return
	}
	now := time.Now().UTC()
	inc.Status = req.Status
	inc.ResolvedAt = nil
	if isClosedStatus(req.Status) {
		inc.ResolvedAt = &now
	}
	if err := rt.statusPage.SaveStatusIncident(r.Context(), inc); err != nil {
		rt.internalError(w, "api: status incident update: save failed", err)
		return
	}
	if err := rt.addIncidentUpdate(r.Context(), inc.ID, req.Status, req.Body, now); err != nil {
		rt.internalError(w, "api: status incident update: add failed", err)
		return
	}
	rt.statusView.Invalidate()
	writeJSON(w, http.StatusOK, toStatusIncidentResource(inc))
}

func (rt *Router) handleDeleteStatusIncident(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	if err := rt.statusPage.DeleteStatusIncident(r.Context(), r.PathValue("id")); err != nil {
		rt.internalError(w, "api: delete status incident failed", err)
		return
	}
	rt.statusView.Invalidate()
	w.WriteHeader(http.StatusNoContent)
}

// StatusPageAppStatus reports an app's status for the status page sampler.
func (rt *Router) StatusPageAppStatus(ctx context.Context, name string) (string, error) {
	conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{applicationControllerName(name)})
	if err != nil {
		return "", err
	}
	summary := summarizeAppConditions(conds[applicationControllerName(name)])
	switch summary.Variant {
	case "success":
		return statuspage.Operational, nil
	case "destructive":
		return statuspage.Outage, nil
	default:
		if summary.Label == "Reconciling" {
			return statuspage.Degraded, nil
		}
		if summary.Label == "Stopped" {
			return statuspage.Outage, nil
		}
		return statuspage.Unknown, nil
	}
}
