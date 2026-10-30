package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

// MonitorService is the engine the monitor routes drive. A nil service means the feature is off.
type MonitorService interface {
	Create(ctx context.Context, in monitor.CreateInput) (monitor.Monitor, error)
	List(ctx context.Context) ([]monitor.Monitor, error)
	Get(ctx context.Context, id string) (monitor.Monitor, error)
	Delete(ctx context.Context, id string) error
	State(id string) string
	Status(id string) monitor.Status
	Results(ctx context.Context, id string, window time.Duration, limit int) (monitor.History, error)
}

// SetMonitors enables the monitor routes. Without it they answer 404.
func (rt *Router) SetMonitors(s MonitorService) { rt.monitors = s }

func monitorsResource(*http.Request) string { return "monitors" }

func (rt *Router) monitorsReady(w http.ResponseWriter) bool {
	if rt.monitors == nil {
		writeError(w, http.StatusNotFound, "not found")
		return false
	}
	return true
}

type monitorRequest struct {
	Name            string                `json:"name"`
	Kind            string                `json:"kind"`
	IntervalSeconds int                   `json:"interval_seconds"`
	TimeoutMS       int                   `json:"timeout_ms"`
	Enabled         *bool                 `json:"enabled"`
	URL             string                `json:"url"`
	Method          string                `json:"method"`
	Headers         map[string]string     `json:"headers"`
	Expect          []monitor.StatusRange `json:"expect"`
	Contains        string                `json:"contains"`
	FollowRedirects bool                  `json:"follow_redirects"`
	Host            string                `json:"host"`
	Port            int                   `json:"port"`
}

// monitorResource never carries header values, only their names.
type monitorResource struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Kind            string                `json:"kind"`
	IntervalSeconds int                   `json:"interval_seconds"`
	TimeoutMS       int                   `json:"timeout_ms"`
	Enabled         bool                  `json:"enabled"`
	URL             string                `json:"url,omitempty"`
	Method          string                `json:"method,omitempty"`
	HeaderNames     []string              `json:"header_names,omitempty"`
	Expect          []monitor.StatusRange `json:"expect,omitempty"`
	Contains        string                `json:"contains,omitempty"`
	FollowRedirects bool                  `json:"follow_redirects,omitempty"`
	Host            string                `json:"host,omitempty"`
	Port            int                   `json:"port,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	// State fields are empty before the first probe and always absent for create responses.
	State         string `json:"state,omitempty"`
	FailStreak    int    `json:"consecutive_failures,omitempty"`
	SuccessStreak int    `json:"consecutive_successes,omitempty"`
}

func toMonitorResource(m monitor.Monitor) monitorResource {
	return monitorResource{ID: m.ID, Name: m.Name, Kind: m.Kind, IntervalSeconds: int(m.Interval / time.Second),
		TimeoutMS: int(m.Timeout / time.Millisecond), Enabled: m.Enabled, URL: m.Spec.URL, Method: m.Spec.Method,
		HeaderNames: m.Spec.HeaderNames, Expect: m.Spec.Expect, Contains: m.Spec.Contains,
		FollowRedirects: m.Spec.FollowRedirects, Host: m.Spec.Host, Port: m.Spec.Port, CreatedAt: m.CreatedAt}
}

type monitorResultResource struct {
	At        int64  `json:"at"`
	OK        bool   `json:"ok"`
	LatencyMS int    `json:"latency_ms"`
	Code      int    `json:"code,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type monitorRollupResource struct {
	Bucket       int64 `json:"bucket"`
	Total        int   `json:"total"`
	Failed       int   `json:"failed"`
	AvgLatencyMS int   `json:"avg_latency_ms"`
	MaxLatencyMS int   `json:"max_latency_ms"`
}

type monitorResultsResponse struct {
	MonitorID string                  `json:"monitor_id"`
	Raw       []monitorResultResource `json:"raw"`
	Rollup    []monitorRollupResource `json:"rollup"`
}

func (rt *Router) monitorError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, monitor.ErrNotFound):
		writeError(w, http.StatusNotFound, "monitor not found")
	case errors.Is(err, monitor.ErrInvalid):
		writeError(w, http.StatusBadRequest, strings.TrimPrefix(err.Error(), "monitor: invalid: "))
	case errors.Is(err, monitor.ErrLimit):
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "monitor: limit reached: "))
	default:
		rt.internalError(w, "api: "+op+" failed", err)
	}
}

func (rt *Router) handleListMonitors(w http.ResponseWriter, r *http.Request) {
	if !rt.monitorsReady(w) {
		return
	}
	list, err := rt.monitors.List(r.Context())
	if err != nil {
		rt.monitorError(w, "list monitors", err)
		return
	}
	out := make([]monitorResource, 0, len(list))
	for _, m := range list {
		res := toMonitorResource(m)
		st := rt.monitors.Status(m.ID)
		res.State, res.FailStreak, res.SuccessStreak = st.State, st.FailStreak, st.SuccessStreak
		out = append(out, res)
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreateMonitor(w http.ResponseWriter, r *http.Request) {
	if !rt.monitorsReady(w) {
		return
	}
	var req monitorRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	enabled := req.Enabled == nil || *req.Enabled
	m := monitor.Monitor{Name: req.Name, Kind: req.Kind, Enabled: enabled,
		Interval: time.Duration(req.IntervalSeconds) * time.Second, Timeout: time.Duration(req.TimeoutMS) * time.Millisecond,
		Spec: monitor.Spec{URL: req.URL, Method: req.Method, Expect: req.Expect, Contains: req.Contains,
			FollowRedirects: req.FollowRedirects, Host: req.Host, Port: req.Port}}
	created, err := rt.monitors.Create(r.Context(), monitor.CreateInput{Monitor: m, Headers: req.Headers})
	if err != nil {
		rt.monitorError(w, "create monitor", err)
		return
	}
	writeJSON(w, http.StatusCreated, toMonitorResource(created))
}

func (rt *Router) handleDeleteMonitor(w http.ResponseWriter, r *http.Request) {
	if !rt.monitorsReady(w) {
		return
	}
	if err := rt.monitors.Delete(r.Context(), r.PathValue("id")); err != nil {
		rt.monitorError(w, "delete monitor", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) handleMonitorResults(w http.ResponseWriter, r *http.Request) {
	if !rt.monitorsReady(w) {
		return
	}
	var window time.Duration
	if v := r.URL.Query().Get("window"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			writeError(w, http.StatusBadRequest, "window must be a positive duration such as 24h")
			return
		}
		window = d
	}
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	id := r.PathValue("id")
	h, err := rt.monitors.Results(r.Context(), id, window, limit)
	if err != nil {
		rt.monitorError(w, "monitor results", err)
		return
	}
	out := monitorResultsResponse{MonitorID: id, Raw: make([]monitorResultResource, 0, len(h.Raw)), Rollup: make([]monitorRollupResource, 0, len(h.Rollup))}
	for _, x := range h.Raw {
		out.Raw = append(out.Raw, monitorResultResource{At: x.At, OK: x.OK, LatencyMS: x.LatencyMS, Code: x.Code, Reason: x.Reason})
	}
	for _, x := range h.Rollup {
		avg := 0
		if okN := x.Total - x.Failed; okN > 0 {
			avg = int(x.LatSum / int64(okN))
		}
		out.Rollup = append(out.Rollup, monitorRollupResource{Bucket: x.Bucket, Total: x.Total, Failed: x.Failed, AvgLatencyMS: avg, MaxLatencyMS: x.LatMax})
	}
	writeJSON(w, http.StatusOK, out)
}
