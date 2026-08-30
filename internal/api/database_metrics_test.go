package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleQueryDatabaseMetrics_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryDatabaseMetrics_DatabaseNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/nonexistent/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryDatabaseMetrics_MissingMetricParam(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/metrics", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleQueryDatabaseMetrics_RawAndAggregated(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "database:main", Metric: "cpu_percent", Timestamp: now.Add(-30 * time.Minute), Value: 10},
		{ResourceID: "database:main", Metric: "cpu_percent", Timestamp: now.Add(-20 * time.Minute), Value: 20},
		// A same-named-prefix app must never leak into a database query's
		// results: distinct resourceID namespaces ("service:" vs
		// "database:"), not a shared key space that a naming collision
		// could bleed across.
		{ResourceID: "service:main", Metric: "cpu_percent", Timestamp: now.Add(-25 * time.Minute), Value: 99},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/databases/main/metrics?metric=cpu_percent&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got metricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Points) != 2 {
		t.Fatalf("raw query points = %d, want 2 (the service:main sample must not leak in)", len(got.Points))
	}

	rec2 := httptest.NewRecorder()
	url2 := url + "&step=1h"
	rt.Handler().ServeHTTP(rec2, authedRequest(t, cookie, http.MethodGet, url2, ""))
	var gotAgg metricsResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &gotAgg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(gotAgg.Points) != 1 {
		t.Fatalf("aggregated query points = %d, want 1", len(gotAgg.Points))
	}
	if gotAgg.Points[0].Value != 15 {
		t.Errorf("aggregated value = %v, want 15 (average of 10 and 20)", gotAgg.Points[0].Value)
	}
}

func TestHandleQueryDatabaseMetrics_InvalidTimeRange(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tests := []string{
		"/api/v1/databases/main/metrics?metric=cpu_percent&from=not-a-time",
		"/api/v1/databases/main/metrics?metric=cpu_percent&to=not-a-time",
		"/api/v1/databases/main/metrics?metric=cpu_percent&step=not-a-duration",
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
