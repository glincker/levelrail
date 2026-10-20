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

func TestHandleAppCostEstimate_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/cost-estimate", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// Unlike resource-recommendation, cost-estimate works with no telemetry
// configured at all: a declared resources: block in app.yaml is enough.
func TestHandleAppCostEstimate_NoTelemetryDeclaredResources(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Resources: &store.ServiceResources{MemoryBytes: 1024 * 1024 * 1024, NanoCPUs: 1_000_000_000},
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/cost-estimate", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got costEstimateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CPUBasis != "declared" || got.MemoryBasis != "declared" {
		t.Errorf("basis = (%q, %q), want (declared, declared)", got.CPUBasis, got.MemoryBasis)
	}
	if got.VCPUCores != 1 || got.MemoryGiB != 1 {
		t.Errorf("size = (%v vcpu, %v GiB), want (1, 1)", got.VCPUCores, got.MemoryGiB)
	}
	if len(got.Providers) == 0 {
		t.Fatal("expected at least one provider estimate")
	}
	for _, p := range got.Providers {
		if p.TotalUSD <= 0 {
			t.Errorf("provider %q: TotalUSD = %v, want > 0", p.Key, p.TotalUSD)
		}
	}
	if got.Note == "" {
		t.Error("Note is empty, want an explicit estimate disclaimer")
	}
}

func TestHandleAppCostEstimate_NoDataAtAll(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/cost-estimate", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got costEstimateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CPUBasis != "unavailable" || got.MemoryBasis != "unavailable" {
		t.Errorf("basis = (%q, %q), want (unavailable, unavailable)", got.CPUBasis, got.MemoryBasis)
	}
}

func TestHandleAppCostEstimate_FallsBackToObservedUsage(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	// No declared resources: the handler must fall back to telemetry.
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
	}); err != nil {
		t.Fatalf("seed app: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var samples []telemetry.Sample
	for i := 0; i < 10; i++ {
		ts := now.Add(-time.Duration(i) * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: ts, Value: 2 * 1024 * 1024 * 1024},
			telemetry.Sample{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: 100},
		)
	}
	if err := tdb.WriteSamples(ctx, samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/cost-estimate", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got costEstimateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.CPUBasis != "observed" || got.MemoryBasis != "observed" {
		t.Errorf("basis = (%q, %q), want (observed, observed)", got.CPUBasis, got.MemoryBasis)
	}
	if got.VCPUCores != 1 || got.MemoryGiB != 2 {
		t.Errorf("size = (%v vcpu, %v GiB), want (1, 2)", got.VCPUCores, got.MemoryGiB)
	}
}
