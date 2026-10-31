package api

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/otlpingest"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const (
	// maxOTLPBody bounds one export request after decompression.
	maxOTLPBody = 8 << 20
	// traceKeyCacheTTL is how long a resolved ingest key is trusted before the
	// secret is read again, so a rotation takes effect within a minute.
	traceKeyCacheTTL = time.Minute
)

// traceIngestSecret is where an app's OTLP ingest key lives, never in the
// desired_services row.
func traceIngestSecretsKey(app string) string { return "trace-ingest/" + app }

const traceIngestKeyName = "key"

// TraceStore is the span storage the trace routes need.
type TraceStore interface {
	WriteSpans(ctx context.Context, spans []telemetry.Span) error
	ListTraces(ctx context.Context, f telemetry.TraceFilter) ([]telemetry.TraceSummary, error)
	GetTrace(ctx context.Context, resourceID, traceID string) ([]telemetry.Span, error)
}

// WithTraceStore turns on the OTLP receiver and the trace routes.
func WithTraceStore(s TraceStore) Option {
	return func(rt *Router) { rt.traces = s }
}

type traceKeyCacheEntry struct {
	key     string
	expires time.Time
}

type traceKeyCache struct {
	mu sync.Mutex
	m  map[string]traceKeyCacheEntry
}

func (c *traceKeyCache) get(app string, now time.Time) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[app]
	if !ok || now.After(e.expires) {
		return "", false
	}
	return e.key, true
}

func (c *traceKeyCache) put(app, key string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]traceKeyCacheEntry{}
	}
	c.m[app] = traceKeyCacheEntry{key: key, expires: now.Add(traceKeyCacheTTL)}
}

func (c *traceKeyCache) drop(app string) {
	c.mu.Lock()
	delete(c.m, app)
	c.mu.Unlock()
}

// newTraceToken builds "<base64url app>.<random hex>": the app is readable so
// the receiver knows whose key to check, the rest is the secret.
func newTraceToken(app string) (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString([]byte(app)) + "." + hex.EncodeToString(b[:]), nil
}

func splitTraceToken(tok string) (app string, ok bool) {
	enc, _, found := strings.Cut(tok, ".")
	if !found || enc == "" {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil || len(raw) == 0 || len(raw) > 128 {
		return "", false
	}
	return string(raw), true
}

// authTraceIngest resolves the app a bearer token belongs to, or "".
func (rt *Router) authTraceIngest(r *http.Request, header string) string {
	tok := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	if tok == "" || rt.secrets == nil {
		return ""
	}
	app, ok := splitTraceToken(tok)
	if !ok {
		return ""
	}
	now := time.Now()
	want, cached := rt.traceKeys.get(app, now)
	if !cached {
		if !rt.ingestLookupAllowed(r) {
			return ""
		}
		v, err := rt.secrets.Resolve(r.Context(), traceIngestSecretsKey(app), traceIngestKeyName)
		if err != nil || v == "" {
			return ""
		}
		want = v
		rt.traceKeys.put(app, want, now)
	}
	if subtle.ConstantTimeCompare([]byte(tok), []byte(want)) != 1 {
		return ""
	}
	return app
}

// handleOTLPTraces handles POST /api/v1/otlp/v1/traces, the path an OTLP/HTTP
// exporter reaches when its endpoint is <base>/api/v1/otlp. It carries its
// own bearer credential: the ingest key of exactly one app.
func (rt *Router) handleOTLPTraces(w http.ResponseWriter, r *http.Request) {
	if rt.traces == nil {
		writeError(w, http.StatusNotImplemented, "trace ingest is not configured on this control plane")
		return
	}
	app := rt.authTraceIngest(r, r.Header.Get("Authorization"))
	if app == "" {
		writeError(w, http.StatusUnauthorized, "missing or invalid ingest key")
		return
	}
	var src io.Reader = http.MaxBytesReader(w, r.Body, maxOTLPBody)
	if strings.EqualFold(r.Header.Get("Content-Encoding"), "gzip") {
		zr, err := gzip.NewReader(src)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid gzip body")
			return
		}
		defer func() { _ = zr.Close() }()
		src = io.LimitReader(zr, maxOTLPBody+1)
	}
	body, err := io.ReadAll(src)
	if err != nil || len(body) > maxOTLPBody {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	spans, err := otlpingest.DecodeTraces(r.Header.Get("Content-Type"), body, "service:"+app)
	switch {
	case errors.Is(err, otlpingest.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.traces.WriteSpans(r.Context(), spans); err != nil {
		rt.internalError(w, "api: write spans", err, slog.String("app", app))
		return
	}
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "application/json") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
}

type tracingResource struct {
	Enabled  bool   `json:"enabled"`
	Endpoint string `json:"endpoint"`
	Header   string `json:"header"`
	// Key is returned once, when it is created or rotated.
	Key string `json:"key,omitempty"`
	// Env is what to set on the app, with the key in place; only on create.
	Env map[string]string `json:"env,omitempty"`
}

func traceEndpoint(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	return scheme + "://" + r.Host + "/api/v1/otlp"
}

func tracingResourceFor(r *http.Request, enabled bool) tracingResource {
	return tracingResource{Enabled: enabled, Endpoint: traceEndpoint(r), Header: "Authorization"}
}

func (rt *Router) requireAppForTracing(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: tracing: load app", err, slog.String("name", name))
		return "", false
	}
	return name, true
}

// handleGetTracing handles GET /api/v1/apps/{name}/tracing.
func (rt *Router) handleGetTracing(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForTracing(w, r)
	if !ok {
		return
	}
	enabled := false
	if rt.secrets != nil {
		enabled, _ = rt.secrets.Exists(r.Context(), traceIngestSecretsKey(name), traceIngestKeyName)
	}
	writeJSON(w, http.StatusOK, tracingResourceFor(r, enabled))
}

// handleEnableTracing handles POST /api/v1/apps/{name}/tracing: creates the
// app's ingest key, or rotates it. The key is in this response only.
func (rt *Router) handleEnableTracing(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForTracing(w, r)
	if !ok {
		return
	}
	if rt.secrets == nil || rt.traces == nil {
		writeError(w, http.StatusNotImplemented, "tracing needs a master key and the telemetry store")
		return
	}
	tok, err := newTraceToken(name)
	if err != nil {
		rt.internalError(w, "api: tracing: generate key", err)
		return
	}
	if err := rt.secrets.SetValueGuarded(r.Context(), traceIngestSecretsKey(name), traceIngestKeyName, tok, true); err != nil {
		rt.internalError(w, "api: tracing: store key", err, slog.String("name", name))
		return
	}
	rt.traceKeys.drop(name)
	res := tracingResourceFor(r, true)
	res.Key = tok
	res.Env = map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": res.Endpoint,
		"OTEL_EXPORTER_OTLP_HEADERS":  "Authorization=Bearer " + tok,
		"OTEL_SERVICE_NAME":           name,
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDisableTracing handles DELETE /api/v1/apps/{name}/tracing.
func (rt *Router) handleDisableTracing(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireAppForTracing(w, r)
	if !ok {
		return
	}
	if d, ok := rt.secrets.(interface {
		DeleteAll(ctx context.Context, serviceName string) error
	}); ok {
		if err := d.DeleteAll(r.Context(), traceIngestSecretsKey(name)); err != nil {
			rt.internalError(w, "api: tracing: delete key", err, slog.String("name", name))
			return
		}
	}
	rt.traceKeys.drop(name)
	w.WriteHeader(http.StatusNoContent)
}

type spanResource struct {
	TraceID       string          `json:"trace_id"`
	SpanID        string          `json:"span_id"`
	ParentID      string          `json:"parent_id,omitempty"`
	Name          string          `json:"name"`
	Kind          int             `json:"kind"`
	StartNs       int64           `json:"start_ns"`
	DurationNs    int64           `json:"duration_ns"`
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message,omitempty"`
	Attributes    json.RawMessage `json:"attributes,omitempty"`
}

// handleListTraces handles GET /api/v1/apps/{name}/traces.
func (rt *Router) handleListTraces(w http.ResponseWriter, r *http.Request) {
	if rt.traces == nil {
		writeError(w, http.StatusNotImplemented, "trace storage is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	q := r.URL.Query()
	f := telemetry.TraceFilter{ResourceID: "service:" + name, ErrorsOnly: q.Get("errors") == "true"}
	var err error
	if f.From, err = parseOptionalTime(q.Get("from")); err != nil {
		writeError(w, http.StatusBadRequest, "from must be RFC3339")
		return
	}
	if f.To, err = parseOptionalTime(q.Get("to")); err != nil {
		writeError(w, http.StatusBadRequest, "to must be RFC3339")
		return
	}
	if f.From.IsZero() {
		f.From = time.Now().Add(-time.Hour)
	}
	if v := q.Get("min_ms"); v != "" {
		if f.MinMillis, err = strconv.ParseInt(v, 10, 64); err != nil || f.MinMillis < 0 {
			writeError(w, http.StatusBadRequest, "min_ms must be a non-negative integer")
			return
		}
	}
	if v := q.Get("limit"); v != "" {
		if f.Limit, err = strconv.Atoi(v); err != nil || f.Limit < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
	}
	out, err := rt.traces.ListTraces(r.Context(), f)
	if err != nil {
		rt.internalError(w, "api: list traces", err, slog.String("name", name))
		return
	}
	if out == nil {
		out = []telemetry.TraceSummary{}
	}
	writeJSON(w, http.StatusOK, out)
}

func parseOptionalTime(v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, v)
}

// handleGetTrace handles GET /api/v1/apps/{name}/traces/{id}.
func (rt *Router) handleGetTrace(w http.ResponseWriter, r *http.Request) {
	if rt.traces == nil {
		writeError(w, http.StatusNotImplemented, "trace storage is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	spans, err := rt.traces.GetTrace(r.Context(), "service:"+name, r.PathValue("id"))
	if err != nil {
		rt.internalError(w, "api: get trace", err, slog.String("name", name))
		return
	}
	if len(spans) == 0 {
		writeError(w, http.StatusNotFound, "trace not found")
		return
	}
	out := make([]spanResource, 0, len(spans))
	for _, s := range spans {
		var attrs json.RawMessage
		if s.Attrs != "" {
			attrs = json.RawMessage(s.Attrs)
		}
		out = append(out, spanResource{
			TraceID: s.TraceID, SpanID: s.SpanID, ParentID: s.ParentID, Name: s.Name, Kind: s.Kind,
			StartNs: s.StartNs, DurationNs: s.EndNs - s.StartNs, StatusCode: s.StatusCode,
			StatusMessage: s.StatusMessage, Attributes: attrs,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
