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

func TestHandleAppResourceUsage_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/resource-usage", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleAppResourceUsage_RanksAppsByLatestSample(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:1", Port: 3000},
		{Name: "worker", Image: "img:1", Port: 3001},
		{Name: "idle", Image: "img:1", Port: 3002}, // never recorded telemetry: must still appear, with no fields set
	} {
		if err := db.SaveDesiredService(context.Background(), svc); err != nil {
			t.Fatalf("seed app %s: %v", svc.Name, err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now.Add(-time.Minute), Value: 10},
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now, Value: 85}, // newest: this is the one that should win
		{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: now, Value: 536870912},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: now, Value: 12},
		{ResourceID: "service:deleted-app", Metric: "cpu_percent", Timestamp: now, Value: 99}, // no longer a real app: must not appear
	})
	if err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/resource-usage", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []appResourceUsageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d apps, want 3 (web, worker, idle), body=%s", len(got), rec.Body.String())
	}

	byName := make(map[string]appResourceUsageResource, len(got))
	for _, u := range got {
		byName[u.Name] = u
	}

	web, ok := byName["web"]
	if !ok {
		t.Fatal("response missing web")
	}
	if web.CPUPercent == nil || *web.CPUPercent != 85 {
		t.Errorf("web.CPUPercent = %v, want 85 (the newest sample, not the older 10)", web.CPUPercent)
	}
	if web.MemoryUsageBytes == nil || *web.MemoryUsageBytes != 536870912 {
		t.Errorf("web.MemoryUsageBytes = %v, want 536870912", web.MemoryUsageBytes)
	}

	worker, ok := byName["worker"]
	if !ok {
		t.Fatal("response missing worker")
	}
	if worker.CPUPercent == nil || *worker.CPUPercent != 12 {
		t.Errorf("worker.CPUPercent = %v, want 12", worker.CPUPercent)
	}

	idle, ok := byName["idle"]
	if !ok {
		t.Fatal("response missing idle")
	}
	if idle.CPUPercent != nil {
		t.Errorf("idle.CPUPercent = %v, want nil (no telemetry ever recorded)", idle.CPUPercent)
	}

	if _, ok := byName["deleted-app"]; ok {
		t.Error("response includes deleted-app, want stale telemetry for a nonexistent app filtered out")
	}
}

func TestHandleAppResourceUsage_NoApps_ReturnsEmptyList(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/resource-usage", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []appResourceUsageResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d apps, want 0", len(got))
	}
}
