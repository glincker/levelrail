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

func TestHandleBatchAppMetrics(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"web", "worker"} {
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: n, Image: "img:1", Port: 3000}); err != nil {
			t.Fatalf("seed %s: %v", n, err)
		}
	}
	now := time.Now().UTC()
	if err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now, Value: 42},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: now, Value: 7},
	}); err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	tests := []struct {
		name  string
		query string
		want  []string
	}{
		{"all", "", []string{"web", "worker"}},
		{"names filter", "?names=worker", []string{"worker"}},
		{"unknown name", "?names=ghost", []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps-metrics"+tc.query, ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var got []appMetricsSummary
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d rows, want %d", len(got), len(tc.want))
			}
			for i, w := range tc.want {
				if got[i].Name != w {
					t.Errorf("row %d = %s, want %s", i, got[i].Name, w)
				}
			}
			if tc.name == "all" && (got[0].CPUPercent == nil || *got[0].CPUPercent != 42) {
				t.Errorf("web cpu = %v, want 42", got[0].CPUPercent)
			}
		})
	}
}

func TestHandleBatchAppMetrics_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps-metrics", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}
