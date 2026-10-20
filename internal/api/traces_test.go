package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	collector "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type traceSecrets struct {
	SecretSetter
	values  map[string]string
	deleted []string
}

func (d *traceSecrets) SetValueGuarded(_ context.Context, svc, key, val string, _ bool) error {
	d.values[svc+"/"+key] = val
	return nil
}

func (d *traceSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	return d.values[svc+"/"+key], nil
}

func (d *traceSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	_, ok := d.values[svc+"/"+key]
	return ok, nil
}

func (d *traceSecrets) DeleteAll(_ context.Context, svc string) error {
	d.deleted = append(d.deleted, svc)
	for k := range d.values {
		if strings.HasPrefix(k, svc+"/") {
			delete(d.values, k)
		}
	}
	return nil
}

func newTraceRouter(t *testing.T) (*Router, *store.DB, *telemetry.DB) {
	t.Helper()
	db := openTestDB(t)
	tel, err := telemetry.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	sec := &traceSecrets{values: map[string]string{}}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithSecretSetter(sec), WithTraceStore(tel)), db, tel
}

func exportBody(t *testing.T) []byte {
	t.Helper()
	req := &collector.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{
		ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{{
			TraceId: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, SpanId: []byte{1, 1, 1, 1, 1, 1, 1, 1},
			Name: "GET /", StartTimeUnixNano: 1_000_000_000, EndTimeUnixNano: 1_100_000_000,
		}}}},
	}}}
	b, err := proto.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOTLPTraceIngestRequiresTheAppsOwnKey(t *testing.T) {
	rt, db, tel := newTraceRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tracing", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d %s", rec.Code, rec.Body)
	}
	var res tracingResource
	_ = json.NewDecoder(rec.Body).Decode(&res)
	if res.Key == "" || res.Env["OTEL_EXPORTER_OTLP_HEADERS"] != "Authorization=Bearer "+res.Key {
		t.Fatalf("enable response = %+v", res)
	}

	post := func(token string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/otlp/v1/traces", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/x-protobuf")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		rt.Handler().ServeHTTP(w, req)
		return w
	}
	if w := post("", exportBody(t)); w.Code != http.StatusUnauthorized {
		t.Fatalf("no key = %d, want 401", w.Code)
	}
	if w := post(res.Key+"x", exportBody(t)); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key = %d, want 401", w.Code)
	}
	if w := post(res.Key, []byte("garbage")); w.Code != http.StatusBadRequest {
		t.Fatalf("garbage body = %d, want 400", w.Code)
	}
	if w := post(res.Key, exportBody(t)); w.Code != http.StatusOK {
		t.Fatalf("valid export = %d %s", w.Code, w.Body)
	}

	spans, err := tel.GetTrace(context.Background(), "service:web", "0102030405060708090a0b0c0d0e0f10")
	if err != nil || len(spans) != 1 || spans[0].ResourceID != "service:web" {
		t.Fatalf("stored = %+v err=%v, want one span owned by web", spans, err)
	}

	// Another app's key cannot write to this app, and rotating invalidates the old one.
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/tracing", ""))
	var rotated tracingResource
	_ = json.NewDecoder(rec.Body).Decode(&rotated)
	rt.traceKeys = traceKeyCache{}
	if w := post(res.Key, exportBody(t)); w.Code != http.StatusUnauthorized {
		t.Fatalf("old key after rotation = %d, want 401", w.Code)
	}
	if w := post(rotated.Key, exportBody(t)); w.Code != http.StatusOK {
		t.Fatalf("new key = %d", w.Code)
	}
}

func TestTraceQueryRoutes(t *testing.T) {
	rt, db, tel := newTraceRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebAppForTest(t, db)
	start := time.Now().Add(-time.Minute).UnixNano()
	if err := tel.WriteSpans(context.Background(), []telemetry.Span{
		{TraceID: "tr1", SpanID: "s1", ResourceID: "service:web", Name: "GET /", StartNs: start, EndNs: start + 50_000_000},
		{TraceID: "tr1", SpanID: "s2", ParentID: "s1", ResourceID: "service:web", Name: "db", StartNs: start + 1_000_000, EndNs: start + 40_000_000, StatusCode: 2},
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/traces", ""))
	var list []telemetry.TraceSummary
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if rec.Code != http.StatusOK || len(list) != 1 || list[0].Errors != 1 || list[0].RootName != "GET /" {
		t.Fatalf("list = %d %+v", rec.Code, list)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/traces/tr1", ""))
	var spans []spanResource
	_ = json.NewDecoder(rec.Body).Decode(&spans)
	if rec.Code != http.StatusOK || len(spans) != 2 || spans[1].ParentID != "s1" {
		t.Fatalf("trace = %d %+v", rec.Code, spans)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/traces/nope", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown trace = %d, want 404", rec.Code)
	}
}
