package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

// maxSilenceDuration caps a single silence so a typo cannot mute alerts for months.
const maxSilenceDuration = 90 * 24 * time.Hour

// AlertNoise is the surface the silence, maintenance window and alert
// history handlers need from internal/alerting.DB.
type AlertNoise interface {
	CreateSilence(ctx context.Context, s alerting.Silence) error
	GetSilence(ctx context.Context, id string) (*alerting.Silence, error)
	ListSilences(ctx context.Context, now time.Time, includeExpired bool, limit int) ([]alerting.Silence, error)
	ListActiveSilences(ctx context.Context, now time.Time) ([]alerting.Silence, error)
	ExpireSilence(ctx context.Context, id string, now time.Time) (*alerting.Silence, error)
	SaveMaintenanceWindow(ctx context.Context, w alerting.MaintenanceWindow) error
	GetMaintenanceWindow(ctx context.Context, id string) (*alerting.MaintenanceWindow, error)
	ListMaintenanceWindows(ctx context.Context) ([]alerting.MaintenanceWindow, error)
	DeleteMaintenanceWindow(ctx context.Context, id string) error
	ListHistory(ctx context.Context, f alerting.HistoryFilter) ([]alerting.HistoryEntry, error)
}

// WithAlertNoise enables the silence, maintenance window and alert history routes.
func WithAlertNoise(a AlertNoise) Option {
	return func(rt *Router) { rt.alertNoise = a }
}

type silenceResource struct {
	ID        string                  `json:"id"`
	Matchers  alerting.SilenceMatcher `json:"matchers"`
	StartsAt  time.Time               `json:"starts_at"`
	EndsAt    time.Time               `json:"ends_at"`
	CreatedBy string                  `json:"created_by"`
	Reason    string                  `json:"reason,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	ExpiredAt *time.Time              `json:"expired_at,omitempty"`
	Status    string                  `json:"status"`
}

func toSilenceResource(s alerting.Silence, now time.Time) silenceResource {
	return silenceResource{ID: s.ID, Matchers: s.Matchers, StartsAt: s.StartsAt.UTC(), EndsAt: s.EndsAt.UTC(), CreatedBy: s.CreatedBy,
		Reason: s.Reason, CreatedAt: s.CreatedAt.UTC(), ExpiredAt: s.ExpiredAt, Status: s.Status(now)}
}

type createSilenceRequest struct {
	Matchers alerting.SilenceMatcher `json:"matchers"`
	StartsAt *time.Time              `json:"starts_at,omitempty"`
	EndsAt   *time.Time              `json:"ends_at,omitempty"`
	Duration string                  `json:"duration,omitempty"`
	Reason   string                  `json:"reason,omitempty"`
}

// buildSilence turns a request into a validated Silence starting at or after now.
func buildSilence(req createSilenceRequest, actor string, now time.Time) (alerting.Silence, error) {
	start := now
	if req.StartsAt != nil {
		start = *req.StartsAt
	}
	var end time.Time
	switch {
	case req.Duration != "":
		d, err := time.ParseDuration(req.Duration)
		if err != nil || d <= 0 {
			return alerting.Silence{}, errors.New("duration must be a positive duration such as \"1h\"")
		}
		end = start.Add(d)
	case req.EndsAt != nil:
		end = *req.EndsAt
	default:
		return alerting.Silence{}, errors.New("duration or ends_at is required")
	}
	if end.Sub(start) > maxSilenceDuration {
		return alerting.Silence{}, fmt.Errorf("a silence may last at most %s", maxSilenceDuration)
	}
	id, err := alerting.NewSilenceID()
	if err != nil {
		return alerting.Silence{}, err
	}
	s := alerting.Silence{ID: id, Matchers: req.Matchers, StartsAt: start, EndsAt: end, CreatedBy: actor,
		Reason: strings.TrimSpace(req.Reason), CreatedAt: now}
	if err := s.Validate(); err != nil {
		return alerting.Silence{}, err
	}
	return s, nil
}

func (rt *Router) alertNoiseReady(w http.ResponseWriter) bool {
	if rt.alertNoise == nil {
		writeError(w, http.StatusNotImplemented, "alerting is not configured on this control plane")
		return false
	}
	return true
}

func (rt *Router) handleListAlertSilences(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	now := time.Now()
	list, err := rt.alertNoise.ListSilences(r.Context(), now, r.URL.Query().Get("include_expired") == "true", 200)
	if err != nil {
		rt.internalError(w, "api: list alert silences failed", err)
		return
	}
	out := make([]silenceResource, 0, len(list))
	for _, s := range list {
		out = append(out, toSilenceResource(s, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateAlertSilence(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	var req createSilenceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	now := time.Now()
	s, err := buildSilence(req, rt.pipelineActor(r), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.alertNoise.CreateSilence(r.Context(), s); err != nil {
		rt.internalError(w, "api: create alert silence failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toSilenceResource(s, now))
}

// handleExpireAlertSilence ends a silence now; the row is kept as history.
func (rt *Router) handleExpireAlertSilence(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	now := time.Now()
	s, err := rt.alertNoise.ExpireSilence(r.Context(), r.PathValue("id"), now)
	if errors.Is(err, alerting.ErrSilenceNotFound) {
		writeError(w, http.StatusNotFound, "silence not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: expire alert silence failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toSilenceResource(*s, now))
}

// handleSilenceAlertRule handles POST /api/v1/apps/{name}/alerts/{id}/silence:
// the quick "silence this alert for N" action.
func (rt *Router) handleSilenceAlertRule(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil || !rt.alertNoiseReady(w) {
		return
	}
	name := r.PathValue("name")
	rule, err := rt.alertRules.GetRule(r.Context(), r.PathValue("id"))
	if errors.Is(err, alerting.ErrRuleNotFound) || (err == nil && rule.ResourceID != resourceIDForApp(name)) {
		writeError(w, http.StatusNotFound, "alert rule not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: silence alert rule: load rule failed", err)
		return
	}
	var req createSilenceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Matchers = alerting.SilenceMatcher{RuleIDs: []string{rule.ID}}
	now := time.Now()
	s, err := buildSilence(req, rt.pipelineActor(r), now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.alertNoise.CreateSilence(r.Context(), s); err != nil {
		rt.internalError(w, "api: silence alert rule failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toSilenceResource(s, now))
}

type maintenanceWindowResource struct {
	ID          string     `json:"id,omitempty"`
	Name        string     `json:"name"`
	Cron        string     `json:"cron"`
	Duration    string     `json:"duration"`
	Timezone    string     `json:"timezone,omitempty"`
	Scope       string     `json:"scope"`
	Targets     []string   `json:"targets,omitempty"`
	Enabled     bool       `json:"enabled"`
	CreatedBy   string     `json:"created_by,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
	Active      bool       `json:"active"`
	ActiveUntil *time.Time `json:"active_until,omitempty"`
	NextStart   *time.Time `json:"next_start,omitempty"`
}

func toWindowResource(w alerting.MaintenanceWindow, now time.Time) maintenanceWindowResource {
	out := maintenanceWindowResource{ID: w.ID, Name: w.Name, Cron: w.Cron, Duration: w.Duration.String(), Timezone: w.Timezone,
		Scope: w.Scope, Targets: w.Targets, Enabled: w.Enabled, CreatedBy: w.CreatedBy}
	if !w.CreatedAt.IsZero() {
		c := w.CreatedAt.UTC()
		out.CreatedAt = &c
	}
	if st, err := w.StateAt(now); err == nil {
		out.Active = w.Enabled && st.Active
		if out.Active {
			end := st.End.UTC()
			out.ActiveUntil = &end
		}
		if !st.NextStart.IsZero() {
			next := st.NextStart.UTC()
			out.NextStart = &next
		}
	}
	return out
}

func (m maintenanceWindowResource) toWindow(id, actor string) (alerting.MaintenanceWindow, error) {
	d, err := time.ParseDuration(m.Duration)
	if err != nil {
		return alerting.MaintenanceWindow{}, errors.New("duration must be a duration such as \"2h\"")
	}
	w := alerting.MaintenanceWindow{ID: id, Name: strings.TrimSpace(m.Name), Cron: strings.TrimSpace(m.Cron), Duration: d,
		Timezone: m.Timezone, Scope: m.Scope, Targets: m.Targets, Enabled: m.Enabled, CreatedBy: actor}
	if w.Scope == "" {
		w.Scope = alerting.ScopeAll
	}
	if w.Timezone == "" {
		w.Timezone = "UTC"
	}
	if err := w.Validate(); err != nil {
		return alerting.MaintenanceWindow{}, err
	}
	return w, nil
}

func (rt *Router) handleListMaintenanceWindows(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	list, err := rt.alertNoise.ListMaintenanceWindows(r.Context())
	if err != nil {
		rt.internalError(w, "api: list maintenance windows failed", err)
		return
	}
	now := time.Now()
	out := make([]maintenanceWindowResource, 0, len(list))
	for _, mw := range list {
		out = append(out, toWindowResource(mw, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	var req maintenanceWindowResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	id, err := alerting.NewMaintenanceWindowID()
	if err != nil {
		rt.internalError(w, "api: create maintenance window: mint id failed", err)
		return
	}
	mw, err := req.toWindow(id, rt.pipelineActor(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.alertNoise.SaveMaintenanceWindow(r.Context(), mw); err != nil {
		rt.internalError(w, "api: create maintenance window failed", err)
		return
	}
	saved, err := rt.alertNoise.GetMaintenanceWindow(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: create maintenance window: reload failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, toWindowResource(*saved, time.Now()))
}

func (rt *Router) handleUpdateMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	id := r.PathValue("id")
	existing, err := rt.alertNoise.GetMaintenanceWindow(r.Context(), id)
	if errors.Is(err, alerting.ErrMaintenanceWindowNotFound) {
		writeError(w, http.StatusNotFound, "maintenance window not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: update maintenance window: load failed", err)
		return
	}
	var req maintenanceWindowResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mw, err := req.toWindow(id, existing.CreatedBy)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.alertNoise.SaveMaintenanceWindow(r.Context(), mw); err != nil {
		rt.internalError(w, "api: update maintenance window failed", err)
		return
	}
	saved, err := rt.alertNoise.GetMaintenanceWindow(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: update maintenance window: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toWindowResource(*saved, time.Now()))
}

func (rt *Router) handleDeleteMaintenanceWindow(w http.ResponseWriter, r *http.Request) {
	if !rt.alertNoiseReady(w) {
		return
	}
	if err := rt.alertNoise.DeleteMaintenanceWindow(r.Context(), r.PathValue("id")); err != nil {
		rt.internalError(w, "api: delete maintenance window failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type alertHistoryResource struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	RuleID     string    `json:"rule_id"`
	RuleName   string    `json:"rule_name"`
	RuleKind   string    `json:"rule_kind"`
	ResourceID string    `json:"resource_id,omitempty"`
	App        string    `json:"app,omitempty"`
	Node       string    `json:"node,omitempty"`
	Severity   string    `json:"severity,omitempty"`
	Event      string    `json:"event"`
	Outcome    string    `json:"outcome"`
	Detail     string    `json:"detail,omitempty"`
	SilenceID  string    `json:"silence_id,omitempty"`
	ChannelID  string    `json:"channel_id,omitempty"`
	Error      string    `json:"error,omitempty"`
}

func (rt *Router) writeAlertHistory(w http.ResponseWriter, r *http.Request, app string) {
	if !rt.alertNoiseReady(w) {
		return
	}
	q := r.URL.Query()
	f := alerting.HistoryFilter{App: app, RuleID: q.Get("rule_id"), Outcome: q.Get("outcome"), Event: q.Get("event")}
	if app == "" {
		f.App = q.Get("app")
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		f.Limit = n
	}
	if v := q.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since must be an RFC 3339 timestamp")
			return
		}
		f.Since = t
	}
	if v := q.Get("before"); v != "" {
		t, err := time.Parse(time.RFC3339Nano, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before must be an RFC 3339 timestamp")
			return
		}
		f.Before = &t
	}
	list, err := rt.alertNoise.ListHistory(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: list alert history failed", err)
		return
	}
	out := make([]alertHistoryResource, 0, len(list))
	for _, e := range list {
		out = append(out, alertHistoryResource{ID: e.ID, At: e.At.UTC(), RuleID: e.RuleID, RuleName: e.RuleName, RuleKind: e.RuleKind,
			ResourceID: e.ResourceID, App: e.App, Node: e.Node, Severity: e.Severity, Event: e.Event, Outcome: e.Outcome,
			Detail: e.Detail, SilenceID: e.SilenceID, ChannelID: e.ChannelID, Error: e.Error})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleListAlertHistory(w http.ResponseWriter, r *http.Request) {
	rt.writeAlertHistory(w, r, "")
}

func (rt *Router) handleListAppAlertHistory(w http.ResponseWriter, r *http.Request) {
	rt.writeAlertHistory(w, r, r.PathValue("name"))
}

// annotateSilenced marks each rule in out that an active silence or
// maintenance window currently mutes. Best effort: a failure leaves the
// flags unset rather than failing the list.
func (rt *Router) annotateSilenced(ctx context.Context, app string, rules []alerting.Rule, out []ruleResource) {
	if rt.alertNoise == nil || len(rules) == 0 {
		return
	}
	now := time.Now()
	silences, err := rt.alertNoise.ListActiveSilences(ctx, now)
	if err != nil {
		rt.logger.Warn("api: annotate silenced: list silences failed", slog.String("error", err.Error()))
		return
	}
	windows, err := rt.alertNoise.ListMaintenanceWindows(ctx)
	if err != nil {
		rt.logger.Warn("api: annotate silenced: list windows failed", slog.String("error", err.Error()))
		return
	}
	nodeID, nodeName := rt.appNode(ctx, app)
	for i, rl := range rules {
		ac := alerting.AlertContext{RuleID: rl.ID, Kind: string(rl.Kind), Labels: rl.Labels, Severity: rl.Severity}
		if !alerting.IsPlatformKind(rl.Kind) {
			ac.App, ac.NodeID, ac.NodeName = app, nodeID, nodeName
		}
		if hit, ok := alerting.FindMute(silences, windows, ac, now); ok {
			out[i].Silenced = true
			out[i].SilencedBy = hit.ID
		}
	}
}

func (rt *Router) appNode(ctx context.Context, app string) (id, name string) {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc == nil || svc.NodeID == "" || rt.nodes == nil {
		return "", ""
	}
	node, err := rt.nodes.GetNode(ctx, svc.NodeID)
	if err != nil || node == nil {
		if !errors.Is(err, store.ErrNodeNotFound) && err != nil {
			rt.logger.Warn("api: resolve app node failed", slog.String("error", err.Error()))
		}
		return svc.NodeID, ""
	}
	return node.ID, node.Name
}
