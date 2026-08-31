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

func seedDatabase(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{
		Name: name, Engine: store.EnginePostgres, Version: "16",
	}); err != nil {
		t.Fatalf("seed database %q: %v", name, err)
	}
}

func TestHandleDatabaseResourceRecommendation_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	seedDatabase(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/resource-recommendation", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleDatabaseResourceRecommendation_DatabaseNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/ghost/resource-recommendation", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleDatabaseResourceRecommendation_InsufficientData(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedDatabase(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/resource-recommendation", ""))
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
	if got.OOMDetectedAt != "" {
		t.Errorf("OOMDetectedAt = %q, want empty", got.OOMDetectedAt)
	}
}

func TestHandleDatabaseResourceRecommendation_UsesCurrentLimitsAndUsage(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: "main", Engine: store.EnginePostgres, Version: "16",
		Resources: &store.ServiceResources{MemoryBytes: 512 * 1024 * 1024, NanoCPUs: 1_000_000_000},
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var samples []telemetry.Sample
	for i := 0; i < 40; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "database:main", Metric: "memory_usage_bytes", Timestamp: ts, Value: 480 * 1024 * 1024},
			telemetry.Sample{ResourceID: "database:main", Metric: "cpu_percent", Timestamp: ts, Value: 20},
		)
	}
	if err := tdb.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/resource-recommendation", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got resourceRecommendationResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Memory.CurrentLimit != 512*1024*1024 {
		t.Errorf("Memory.CurrentLimit = %d, want the database's configured limit", got.Memory.CurrentLimit)
	}
	if got.Memory.Action != "raise" {
		t.Errorf("Memory.Action = %q, want %q (p95 480MiB against a 512MiB limit)", got.Memory.Action, "raise")
	}
	if got.CPU.Action != "lower" {
		t.Errorf("CPU.Action = %q, want %q (20%% usage against a 1-core limit)", got.CPU.Action, "lower")
	}
}

func TestHandleDatabaseResourceRecommendation_OOMSignalFromLogs(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{
		Name: "main", Engine: store.EnginePostgres, Version: "16",
		Resources: &store.ServiceResources{MemoryBytes: 256 * 1024 * 1024},
	}); err != nil {
		t.Fatalf("seed database: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "database:main", Stream: "stdout", Timestamp: now.Add(-time.Hour), Message: "container exited: oomkilled"},
	}); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/resource-recommendation", ""))
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

func TestHandleDatabaseResourceRecommendation_RequiresAuth(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	seedDatabase(t, db, "main")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/databases/main/resource-recommendation", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
