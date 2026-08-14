package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// newTestTelemetryDB opens a fresh temp-file telemetry store, the same
// real-not-mocked pattern openTestDB already establishes for
// internal/store in this package's tests.
func newTestTelemetryDB(t *testing.T) *telemetry.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telemetry.db")
	db, err := telemetry.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("telemetry.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test telemetry db: %v", err)
		}
	})
	return db
}

// newTestRouterWithTelemetry is newTestRouter plus a wired
// TelemetryQuerier, for the metrics/logs query handlers specifically.
func newTestRouterWithTelemetry(t *testing.T) (*Router, *store.DB, *telemetry.DB) {
	t.Helper()
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)))
	return rt, db, tdb
}

func TestHandleQueryMetrics_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryMetrics_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nonexistent/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryMetrics_MissingMetricParam(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/metrics", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleQueryMetrics_RawAndAggregated(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now.Add(-30 * time.Minute), Value: 10},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now.Add(-20 * time.Minute), Value: 20},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	// Raw (no step): one point per sample.
	rec := httptest.NewRecorder()
	url := "/api/v1/apps/web/metrics?metric=cpu_percent&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got metricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Points) != 2 {
		t.Fatalf("raw query points = %d, want 2", len(got.Points))
	}

	// Aggregated: step wide enough to bucket both samples together.
	rec2 := httptest.NewRecorder()
	url2 := url + "&step=1h"
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, url2, ""))
	var gotAgg metricsResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &gotAgg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(gotAgg.Points) != 1 {
		t.Fatalf("aggregated query points = %d, want 1 (both samples in one 1h bucket)", len(gotAgg.Points))
	}
	if gotAgg.Points[0].Value != 15 {
		t.Errorf("aggregated value = %v, want 15 (average of 10 and 20)", gotAgg.Points[0].Value)
	}
	if gotAgg.Points[0].Count != 2 {
		t.Errorf("aggregated count = %d, want 2", gotAgg.Points[0].Count)
	}
}

func TestHandleQueryMetrics_InvalidTimeRange(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tests := []string{
		"/api/v1/apps/web/metrics?metric=cpu_percent&from=not-a-time",
		"/api/v1/apps/web/metrics?metric=cpu_percent&to=not-a-time",
		"/api/v1/apps/web/metrics?metric=cpu_percent&step=not-a-duration",
	}
	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}
