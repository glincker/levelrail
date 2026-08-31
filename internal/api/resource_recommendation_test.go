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

func TestHandleAppResourceRecommendation_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/resource-recommendation", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleAppResourceRecommendation_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/resource-recommendation", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleAppResourceRecommendation_InsufficientData(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/resource-recommendation", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got resourceRecommendationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Memory.DataSufficient {
		t.Error("Memory.DataSufficient = true, want false with no telemetry history")
	}
	if got.Memory.Action != "" {
		t.Errorf("Memory.Action = %q, want empty with no data", got.Memory.Action)
	}
	if got.OOMDetectedAt != "" {
		t.Errorf("OOMDetectedAt = %q, want empty", got.OOMDetectedAt)
	}
}

func TestHandleAppResourceRecommendation_UsesCurrentLimitsAndUsage(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Resources: &store.ServiceResources{MemoryBytes: 512 * 1024 * 1024, NanoCPUs: 1_000_000_000},
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var samples []telemetry.Sample
	for i := 0; i < 40; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: ts, Value: 480 * 1024 * 1024},
			telemetry.Sample{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: 20},
		)
	}
	if err := tdb.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/resource-recommendation", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got resourceRecommendationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Memory.CurrentLimit != 512*1024*1024 {
		t.Errorf("Memory.CurrentLimit = %d, want the app's configured limit", got.Memory.CurrentLimit)
	}
	if got.Memory.Action != "raise" {
		t.Errorf("Memory.Action = %q, want %q (p95 480MiB against a 512MiB limit)", got.Memory.Action, "raise")
	}
	if got.CPU.CurrentLimit != 1_000_000_000 {
		t.Errorf("CPU.CurrentLimit = %d, want 1_000_000_000", got.CPU.CurrentLimit)
	}
	if got.CPU.Action != "lower" {
		t.Errorf("CPU.Action = %q, want %q (20%% usage against a 1-core limit)", got.CPU.Action, "lower")
	}
}

func TestHandleAppResourceRecommendation_OOMSignalFromLogs(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Resources: &store.ServiceResources{MemoryBytes: 256 * 1024 * 1024},
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "service:web", Stream: "stdout", Timestamp: now.Add(-time.Hour), Message: "container exited: oomkilled"},
	}); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/resource-recommendation", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got resourceRecommendationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.OOMDetectedAt == "" {
		t.Fatal("OOMDetectedAt is empty, want the seeded oomkilled log line to be detected")
	}
	if got.Memory.Action != "raise" {
		t.Errorf("Memory.Action = %q, want %q when an OOM kill was found in logs", got.Memory.Action, "raise")
	}
	if got.Memory.Confidence != "high" {
		t.Errorf("Memory.Confidence = %q, want %q for an OOM-backed recommendation", got.Memory.Confidence, "high")
	}
}

func TestHandleAppResourceRecommendation_RequiresAuth(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/resource-recommendation", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
