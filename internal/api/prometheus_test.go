package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"

	"github.com/GLINCKER/levelrail/internal/prompb"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func promReadRequest(t *testing.T, cookie *http.Cookie, req prompb.ReadRequest) *http.Request {
	t.Helper()
	body := snappy.Encode(nil, prompb.MarshalReadRequest(req))
	r := httptest.NewRequest(http.MethodPost, "/api/v1/prometheus/read", strings.NewReader(string(body)))
	r.AddCookie(cookie)
	return r
}

func decodePromReadResponse(t *testing.T, rec *httptest.ResponseRecorder) prompb.ReadResponse {
	t.Helper()
	raw, err := snappy.Decode(nil, rec.Body.Bytes())
	if err != nil {
		t.Fatalf("snappy.Decode() error = %v", err)
	}
	resp, err := prompb.UnmarshalReadResponse(raw)
	if err != nil {
		t.Fatalf("UnmarshalReadResponse() error = %v", err)
	}
	return resp
}

func TestHandlePrometheusRead_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, promReadRequest(t, cookie, prompb.ReadRequest{}))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandlePrometheusRead_RequiresAuth(t *testing.T) {
	rt, _, _ := newTestRouterWithTelemetry(t)

	body := snappy.Encode(nil, prompb.MarshalReadRequest(prompb.ReadRequest{}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/prometheus/read", strings.NewReader(string(body)))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d (no session/token: this endpoint must not be open)", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlePrometheusRead_ReturnsSamples(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now.Add(-time.Minute), Value: 12.5},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now, Value: 15.0},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	promReq := prompb.ReadRequest{
		Queries: []prompb.Query{{
			StartTimestampMs: now.Add(-time.Hour).UnixMilli(),
			EndTimestampMs:   now.Add(time.Minute).UnixMilli(),
			Matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchEqual, Name: "__name__", Value: "levelrail_cpu_percent"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
		}},
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, promReadRequest(t, cookie, promReq))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-protobuf" {
		t.Errorf("Content-Type = %q, want application/x-protobuf", ct)
	}
	if ce := rec.Header().Get("Content-Encoding"); ce != "snappy" {
		t.Errorf("Content-Encoding = %q, want snappy", ce)
	}

	resp := decodePromReadResponse(t, rec)
	if len(resp.Results) != 1 {
		t.Fatalf("Results = %d, want 1 (one per query)", len(resp.Results))
	}
	ts := resp.Results[0].TimeSeries
	if len(ts) != 1 {
		t.Fatalf("TimeSeries = %d, want 1", len(ts))
	}
	if len(ts[0].Samples) != 2 {
		t.Fatalf("Samples = %d, want 2", len(ts[0].Samples))
	}

	labels := map[string]string{}
	for _, l := range ts[0].Labels {
		labels[l.Name] = l.Value
	}
	if labels["__name__"] != "levelrail_cpu_percent" {
		t.Errorf("__name__ label = %q, want levelrail_cpu_percent", labels["__name__"])
	}
	if labels["service"] != "web" {
		t.Errorf("service label = %q, want web", labels["service"])
	}
}

func TestHandlePrometheusRead_UnknownMetricName_EmptyResult(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	promReq := prompb.ReadRequest{
		Queries: []prompb.Query{{
			StartTimestampMs: 0,
			EndTimestampMs:   time.Now().UnixMilli(),
			Matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchEqual, Name: "__name__", Value: "not_a_levelrail_metric"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
		}},
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, promReadRequest(t, cookie, promReq))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unsatisfiable selector is empty, not an error)", rec.Code)
	}

	resp := decodePromReadResponse(t, rec)
	if len(resp.Results) != 1 || len(resp.Results[0].TimeSeries) != 0 {
		t.Errorf("Results = %+v, want one empty QueryResult", resp.Results)
	}
}

func TestHandlePrometheusRead_RegexMatcher_EmptyResult(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	promReq := prompb.ReadRequest{
		Queries: []prompb.Query{{
			StartTimestampMs: 0,
			EndTimestampMs:   time.Now().UnixMilli(),
			Matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchRegexp, Name: "__name__", Value: "levelrail_.*"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
		}},
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, promReadRequest(t, cookie, promReq))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a matcher type this pass doesn't support is empty, not an error)", rec.Code)
	}

	resp := decodePromReadResponse(t, rec)
	if len(resp.Results) != 1 || len(resp.Results[0].TimeSeries) != 0 {
		t.Errorf("Results = %+v, want one empty QueryResult (regex matchers not supported this pass)", resp.Results)
	}
}

func TestMatchersToMetricAndService(t *testing.T) {
	tests := []struct {
		name        string
		matchers    []prompb.LabelMatcher
		wantMetric  string
		wantService string
		wantOK      bool
	}{
		{
			name: "valid pair",
			matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchEqual, Name: "__name__", Value: "levelrail_cpu_percent"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
			wantMetric: "cpu_percent", wantService: "web", wantOK: true,
		},
		{name: "empty matchers", matchers: nil, wantOK: false},
		{
			name:     "missing service matcher",
			matchers: []prompb.LabelMatcher{{Type: prompb.MatchEqual, Name: "__name__", Value: "levelrail_cpu_percent"}},
			wantOK:   false,
		},
		{
			name:     "missing __name__ matcher",
			matchers: []prompb.LabelMatcher{{Type: prompb.MatchEqual, Name: "service", Value: "web"}},
			wantOK:   false,
		},
		{
			name: "non-levelrail metric name",
			matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchEqual, Name: "__name__", Value: "up"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
			wantOK: false,
		},
		{
			name: "unsupported matcher type",
			matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchNotEqual, Name: "__name__", Value: "levelrail_cpu_percent"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
			},
			wantOK: false,
		},
		{
			name: "unsupported label",
			matchers: []prompb.LabelMatcher{
				{Type: prompb.MatchEqual, Name: "__name__", Value: "levelrail_cpu_percent"},
				{Type: prompb.MatchEqual, Name: "service", Value: "web"},
				{Type: prompb.MatchEqual, Name: "job", Value: "levelrail"},
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric, service, ok := matchersToMetricAndService(tt.matchers)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if metric != tt.wantMetric || service != tt.wantService {
				t.Errorf("(metric, service) = (%q, %q), want (%q, %q)", metric, service, tt.wantMetric, tt.wantService)
			}
		})
	}
}
