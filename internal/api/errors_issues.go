package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type errorTrackingResource struct {
	Enabled bool `json:"enabled"`
	// Endpoint is the DSN without its key.
	Endpoint string `json:"endpoint"`
	// DSN and Env are returned once, when the key is created or rotated.
	DSN string            `json:"dsn,omitempty"`
	Env map[string]string `json:"env,omitempty"`
}

func errorTrackingBase(r *http.Request) string {
	return strings.TrimSuffix(traceEndpoint(r), "/api/v1/otlp") + "/api/v1/sentry/" + sentryProjectID
}

func (rt *Router) requireAppForErrors(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: error tracking: load app", err, slog.String("name", name))
		return "", false
	}
	return name, true
}

// handleGetErrorTracking handles GET /api/v1/apps/{name}/error-tracking.
func (rt *Router) handleGetErrorTracking(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForErrors(w, r)
	if !ok {
		return
	}
	enabled := false
	if rt.secrets != nil {
		enabled, _ = rt.secrets.Exists(r.Context(), errorIngestSecretsKey(name), errorIngestSecretName)
	}
	writeJSON(w, http.StatusOK, errorTrackingResource{Enabled: enabled, Endpoint: errorTrackingBase(r)})
}

// handleEnableErrorTracking handles POST /api/v1/apps/{name}/error-tracking:
// creates the app's DSN key, or rotates it. The DSN is in this response only.
func (rt *Router) handleEnableErrorTracking(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForErrors(w, r)
	if !ok {
		return
	}
	if rt.secrets == nil || rt.errTrack.store == nil {
		writeError(w, http.StatusNotImplemented, "error tracking needs a master key and the telemetry store")
		return
	}
	tok, err := newTraceToken(name)
	if err != nil {
		rt.internalError(w, "api: error tracking: generate key", err)
		return
	}
	if err := rt.secrets.SetValueGuarded(r.Context(), errorIngestSecretsKey(name), errorIngestSecretName, tok, true); err != nil {
		rt.internalError(w, "api: error tracking: store key", err, slog.String("name", name))
		return
	}
	rt.errTrack.keys.drop(name)
	base := errorTrackingBase(r)
	dsn := strings.Replace(base, "://", "://"+tok+"@", 1)
	writeJSON(w, http.StatusOK, errorTrackingResource{
		Enabled: true, Endpoint: base, DSN: dsn, Env: map[string]string{"SENTRY_DSN": dsn},
	})
}

// handleDisableErrorTracking handles DELETE /api/v1/apps/{name}/error-tracking.
func (rt *Router) handleDisableErrorTracking(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForErrors(w, r)
	if !ok {
		return
	}
	if d, ok := rt.secrets.(interface {
		DeleteAll(ctx context.Context, serviceName string) error
	}); ok {
		if err := d.DeleteAll(r.Context(), errorIngestSecretsKey(name)); err != nil {
			rt.internalError(w, "api: error tracking: delete key", err, slog.String("name", name))
			return
		}
	}
	rt.errTrack.keys.drop(name)
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) errorStoreOr501(w http.ResponseWriter) (ErrorStore, bool) {
	if rt.errTrack.store == nil {
		writeError(w, http.StatusNotImplemented, "error storage is not configured on this control plane")
		return nil, false
	}
	return rt.errTrack.store, true
}

// handleListErrors handles GET /api/v1/apps/{name}/errors.
func (rt *Router) handleListErrors(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.errorStoreOr501(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	q := r.URL.Query()
	f := telemetry.ErrorFilter{ResourceID: "service:" + name, Status: q.Get("status")}
	switch f.Status {
	case "", telemetry.ErrorStatusOpen, telemetry.ErrorStatusResolved, telemetry.ErrorStatusIgnored:
	default:
		writeError(w, http.StatusBadRequest, "status must be open, resolved or ignored")
		return
	}
	if v := q.Get("since"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			f.Since = time.Now().Add(-d)
		} else if t, err := time.Parse(time.RFC3339, v); err == nil {
			f.Since = t
		} else {
			writeError(w, http.StatusBadRequest, "since must be a duration like 24h or an RFC3339 time")
			return
		}
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		f.Limit = n
	}
	out, err := st.ListErrorIssues(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: list errors", err, slog.String("name", name))
		return
	}
	if out == nil {
		out = []telemetry.ErrorIssue{}
	}
	writeJSON(w, http.StatusOK, out)
}

type errorEventResource struct {
	EventID     string          `json:"event_id"`
	TimestampNs int64           `json:"timestamp_ns"`
	Release     string          `json:"release,omitempty"`
	Environment string          `json:"environment,omitempty"`
	DeployID    string          `json:"deploy_id,omitempty"`
	Event       json.RawMessage `json:"event,omitempty"`
}

type errorIssueResource struct {
	Issue  telemetry.ErrorIssue `json:"issue"`
	Events []errorEventResource `json:"events"`
}

// handleGetError handles GET /api/v1/apps/{name}/errors/{fingerprint}.
func (rt *Router) handleGetError(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.errorStoreOr501(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	issue, samples, err := st.GetErrorIssue(r.Context(), "service:"+name, r.PathValue("fingerprint"))
	if errors.Is(err, telemetry.ErrNoErrorIssue) {
		writeError(w, http.StatusNotFound, "error issue not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get error issue", err, slog.String("name", name))
		return
	}
	out := errorIssueResource{Issue: issue, Events: make([]errorEventResource, 0, len(samples))}
	for _, s := range samples {
		e := errorEventResource{EventID: s.EventID, TimestampNs: s.TimestampNs, Release: s.Release,
			Environment: s.Environment, DeployID: s.DeployID}
		if json.Valid([]byte(s.Raw)) {
			e.Event = json.RawMessage(s.Raw)
		}
		out.Events = append(out.Events, e)
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePatchError handles PATCH /api/v1/apps/{name}/errors/{fingerprint}.
func (rt *Router) handlePatchError(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.errorStoreOr501(w)
	if !ok {
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "body must be {\"status\": \"open|resolved|ignored\"}")
		return
	}
	switch body.Status {
	case telemetry.ErrorStatusOpen, telemetry.ErrorStatusResolved, telemetry.ErrorStatusIgnored:
	default:
		writeError(w, http.StatusBadRequest, "status must be open, resolved or ignored")
		return
	}
	name, fp := r.PathValue("name"), r.PathValue("fingerprint")
	err := st.SetErrorIssueStatus(r.Context(), "service:"+name, fp, body.Status)
	if errors.Is(err, telemetry.ErrNoErrorIssue) {
		writeError(w, http.StatusNotFound, "error issue not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: set error issue status", err, slog.String("name", name))
		return
	}
	rt.handleGetError(w, r)
}
