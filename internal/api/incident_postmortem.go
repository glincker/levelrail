package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

// IncidentPostmortemStore is the postmortem persistence. *store.DB satisfies it next to IncidentStore.
type IncidentPostmortemStore interface {
	GetIncidentPostmortem(ctx context.Context, incidentID string) (store.IncidentPostmortem, error)
	DraftIncidentPostmortem(ctx context.Context, p store.IncidentPostmortem) error
	EditIncidentPostmortem(ctx context.Context, incidentID string, e store.PostmortemEdit, by string, at time.Time) error
	PublishIncidentPostmortem(ctx context.Context, incidentID, by string, at time.Time) (store.IncidentPostmortem, error)
	ListStatusIncidentLinks(ctx context.Context, incidentID string) ([]store.StatusIncidentLink, error)
}

type postmortemResource struct {
	IncidentID  string                  `json:"incident_id"`
	State       string                  `json:"state"`
	Summary     string                  `json:"summary"`
	Impact      string                  `json:"impact"`
	RootCause   string                  `json:"root_cause"`
	Resolution  string                  `json:"resolution"`
	ActionItems []string                `json:"action_items"`
	ImpactStart time.Time               `json:"impact_start"`
	ImpactEnd   *time.Time              `json:"impact_end,omitempty"`
	Alerts      []store.PostmortemAlert `json:"alerts"`
	CreatedBy   string                  `json:"created_by,omitempty"`
	UpdatedBy   string                  `json:"updated_by,omitempty"`
	PublishedBy string                  `json:"published_by,omitempty"`
	PublishedAt *time.Time              `json:"published_at,omitempty"`
	UpdatedAt   time.Time               `json:"updated_at"`
}

// postmortemStateResource is what a postmortem write returns, so a write-only token reads no text back.
type postmortemStateResource struct {
	IncidentID string    `json:"incident_id"`
	State      string    `json:"state"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type postmortemEditRequest struct {
	Summary     *string   `json:"summary"`
	Impact      *string   `json:"impact"`
	RootCause   *string   `json:"root_cause"`
	Resolution  *string   `json:"resolution"`
	ActionItems *[]string `json:"action_items"`
}

func toPostmortemResource(p store.IncidentPostmortem) postmortemResource {
	items := p.ActionItems
	if items == nil {
		items = []string{}
	}
	alerts := p.Alerts
	if alerts == nil {
		alerts = []store.PostmortemAlert{}
	}
	return postmortemResource{IncidentID: p.IncidentID, State: p.State, Summary: p.Summary, Impact: p.Impact, RootCause: p.RootCause,
		Resolution: p.Resolution, ActionItems: items, ImpactStart: p.ImpactStart.UTC(), ImpactEnd: p.ImpactEnd, Alerts: alerts,
		CreatedBy: p.CreatedBy, UpdatedBy: p.UpdatedBy, PublishedBy: p.PublishedBy, PublishedAt: p.PublishedAt, UpdatedAt: p.UpdatedAt.UTC()}
}

// postmortemReady loads the incident through the same 404 path as every incident route.
func (rt *Router) postmortemReady(w http.ResponseWriter, r *http.Request) (IncidentPostmortemStore, store.InternalStatusIncident, bool) {
	if !rt.incidentsReady(w) {
		return nil, store.InternalStatusIncident{}, false
	}
	pm, ok := rt.incidents.(IncidentPostmortemStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "incident postmortems are not available on this control plane")
		return nil, store.InternalStatusIncident{}, false
	}
	inc, ok := rt.loadIncident(w, r)
	return pm, inc, ok
}

// postmortemWritable also requires a resolved incident, the precondition of every postmortem write.
func (rt *Router) postmortemWritable(w http.ResponseWriter, r *http.Request) (IncidentPostmortemStore, store.InternalStatusIncident, bool) {
	pm, inc, ok := rt.postmortemReady(w, r)
	if ok && inc.InternalState != store.IncidentStateResolved {
		writeError(w, http.StatusConflict, "resolve the incident before writing its postmortem")
		return nil, inc, false
	}
	return pm, inc, ok
}

func (rt *Router) writePostmortemErr(w http.ResponseWriter, err error, msg string) {
	switch {
	case errors.Is(err, store.ErrIncidentPostmortemNotFound):
		writeError(w, http.StatusNotFound, "postmortem not found; create it with POST .../postmortem/draft")
	case errors.Is(err, store.ErrIncidentPostmortemPublished):
		writeError(w, http.StatusConflict, "postmortem is published and can no longer be edited")
	case errors.Is(err, store.ErrIncidentPostmortemIncomplete):
		writeError(w, http.StatusUnprocessableEntity, "write a summary and a root cause before publishing")
	default:
		rt.internalError(w, msg, err)
	}
}

func (rt *Router) handleGetIncidentPostmortem(w http.ResponseWriter, r *http.Request) {
	pm, inc, ok := rt.postmortemReady(w, r)
	if !ok {
		return
	}
	p, err := pm.GetIncidentPostmortem(r.Context(), inc.ID)
	if err != nil {
		rt.writePostmortemErr(w, err, "api: load incident postmortem failed")
		return
	}
	writeJSON(w, http.StatusOK, toPostmortemResource(p))
}

// handleDraftIncidentPostmortem fills the impact window and alert list from the incident's own record;
// a re-draft refreshes only those, never the written text.
func (rt *Router) handleDraftIncidentPostmortem(w http.ResponseWriter, r *http.Request) {
	pm, inc, ok := rt.postmortemWritable(w, r)
	if !ok {
		return
	}
	links, err := pm.ListStatusIncidentLinks(r.Context(), inc.ID)
	if err != nil {
		rt.internalError(w, "api: load incident links failed", err)
		return
	}
	entries, err := rt.incidents.ListIncidentTimeline(r.Context(), inc.ID)
	if err != nil {
		rt.internalError(w, "api: load incident timeline failed", err)
		return
	}
	p := store.IncidentPostmortem{IncidentID: inc.ID, UpdatedBy: rt.incidentActor(r), UpdatedAt: time.Now().UTC()}
	p.ImpactStart, p.ImpactEnd = postmortemWindow(inc, links, entries)
	p.Alerts = postmortemAlerts(inc, links)
	if err := pm.DraftIncidentPostmortem(r.Context(), p); err != nil {
		rt.writePostmortemErr(w, err, "api: draft incident postmortem failed")
		return
	}
	writeJSON(w, http.StatusOK, postmortemStateResource{IncidentID: inc.ID, State: store.PostmortemStateDraft, UpdatedAt: p.UpdatedAt})
}

func (rt *Router) handleSaveIncidentPostmortem(w http.ResponseWriter, r *http.Request) {
	pm, inc, ok := rt.postmortemWritable(w, r)
	if !ok {
		return
	}
	limits := rt.RuntimeSettings().PostmortemLimits(r.Context())
	bodyMax := min(int64(limits.FieldMax)*int64(4+limits.ActionMax)+4096, int64(limits.BodyMax))
	var req postmortemEditRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyMax)).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body is larger than APP_INCIDENT_POSTMORTEM_BODY_MAX allows")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	e := store.PostmortemEdit{}
	for _, f := range []struct {
		in  *string
		out **string
	}{{req.Summary, &e.Summary}, {req.Impact, &e.Impact}, {req.RootCause, &e.RootCause}, {req.Resolution, &e.Resolution}} {
		if f.in == nil {
			continue
		}
		v, msg := cleanPostmortemText(*f.in, limits.FieldMax)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		*f.out = &v
	}
	if req.ActionItems != nil {
		items := make([]string, 0, len(*req.ActionItems))
		for _, it := range *req.ActionItems {
			v, msg := cleanPostmortemText(it, limits.FieldMax)
			if msg != "" {
				writeError(w, http.StatusBadRequest, "action item: "+msg)
				return
			}
			if v != "" {
				items = append(items, v)
			}
		}
		if len(items) > limits.ActionMax {
			writeError(w, http.StatusBadRequest, "too many action items for APP_INCIDENT_POSTMORTEM_ACTION_ITEMS_MAX")
			return
		}
		e.ActionItems = &items
	}
	now := time.Now().UTC()
	if err := pm.EditIncidentPostmortem(r.Context(), inc.ID, e, rt.incidentActor(r), now); err != nil {
		rt.writePostmortemErr(w, err, "api: save incident postmortem failed")
		return
	}
	writeJSON(w, http.StatusOK, postmortemStateResource{IncidentID: inc.ID, State: store.PostmortemStateDraft, UpdatedAt: now})
}

// cleanPostmortemText trims and checks one field; control and bidi override characters are refused
// because the text reaches terminals and other operators' screens.
func cleanPostmortemText(s string, maxLen int) (string, string) {
	v := strings.ReplaceAll(s, "\r\n", "\n")
	if len(v) > maxLen {
		return "", "a postmortem field is longer than APP_INCIDENT_POSTMORTEM_FIELD_MAX"
	}
	if !utf8.ValidString(v) {
		return "", "a postmortem field is not valid UTF-8"
	}
	for _, c := range v {
		if (c < 0x20 && c != '\n' && c != '\t') || (c >= 0x7f && c <= 0x9f) || (c >= 0x202a && c <= 0x202e) || (c >= 0x2066 && c <= 0x2069) {
			return "", "a postmortem field contains control or bidi override characters"
		}
	}
	return strings.TrimSpace(v), ""
}

func (rt *Router) handlePublishIncidentPostmortem(w http.ResponseWriter, r *http.Request) {
	pm, inc, ok := rt.postmortemWritable(w, r)
	if !ok {
		return
	}
	out, err := pm.PublishIncidentPostmortem(r.Context(), inc.ID, rt.incidentActor(r), time.Now().UTC())
	if err != nil {
		rt.writePostmortemErr(w, err, "api: publish incident postmortem failed")
		return
	}
	writeJSON(w, http.StatusOK, postmortemStateResource{IncidentID: out.IncidentID, State: out.State, UpdatedAt: out.UpdatedAt.UTC()})
}

// postmortemWindow spans the earliest recorded outage to the resolve stamp, falling back to the
// last recovery or resolved timeline entry when the incident carries no resolve time.
func postmortemWindow(inc store.InternalStatusIncident, links []store.StatusIncidentLink, entries []store.IncidentTimelineEntry) (time.Time, *time.Time) {
	start := inc.StartsAt.UTC()
	var end *time.Time
	later := func(t time.Time) {
		if t = t.UTC(); end == nil || t.After(*end) {
			end = &t
		}
	}
	for _, l := range links {
		if l.DownAt.Before(start) {
			start = l.DownAt.UTC()
		}
		if l.UpAt != nil {
			later(*l.UpAt)
		}
	}
	for _, e := range entries {
		if e.Status == statuspage.StatusResolved {
			later(e.CreatedAt)
		}
	}
	if inc.InternalResolvedAt != nil {
		t := inc.InternalResolvedAt.UTC()
		end = &t
	}
	return start, end
}

// postmortemAlerts lists only sources already stored on the incident, never a lookup the caller could not make.
func postmortemAlerts(inc store.InternalStatusIncident, links []store.StatusIncidentLink) []store.PostmortemAlert {
	out := make([]store.PostmortemAlert, 0, len(links)+1)
	linked := map[string]bool{}
	for _, l := range links {
		down := l.DownAt.UTC()
		out = append(out, store.PostmortemAlert{Kind: "monitor", Ref: l.MonitorID, DownAt: &down, UpAt: l.UpAt})
		linked[l.MonitorID] = true
	}
	switch {
	case inc.SourceRef == "":
	case inc.Source == incidentSourceAlert:
		out = append([]store.PostmortemAlert{{Kind: "alert_rule", Ref: inc.SourceRef}}, out...)
	case !linked[inc.SourceRef]:
		out = append([]store.PostmortemAlert{{Kind: inc.Source, Ref: inc.SourceRef}}, out...)
	}
	return out
}
