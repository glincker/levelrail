package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleNodeCapacityForecast_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/capacity-forecast", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleNodeCapacityForecast_NodeNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/nonexistent/capacity-forecast", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleNodeCapacityForecast_NoSamples_OmitsBothFields covers a node
// with telemetry configured but no host samples ever written: both
// Disk and Memory must be absent, not a zero-value projection.
func TestHandleNodeCapacityForecast_NoSamples_OmitsBothFields(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/capacity-forecast", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeCapacityForecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Disk != nil {
		t.Errorf("Disk = %+v, want nil", got.Disk)
	}
	if got.Memory != nil {
		t.Errorf("Memory = %+v, want nil", got.Memory)
	}
	if got.Note == "" {
		t.Error("Note is empty, want the honesty disclaimer present even with nothing to warn about")
	}
}

// TestHandleNodeCapacityForecast_GrowingDisk_ReportsProjection seeds a
// clean, steadily growing disk_used_bytes series (with a flat
// disk_total_bytes) and checks the handler surfaces a real projection
// derived from it, not just that the field is merely present.
func TestHandleNodeCapacityForecast_GrowingDisk_ReportsProjection(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-9 * 24 * time.Hour)
	var samples []telemetry.Sample
	for day := 0; day <= 9; day++ {
		ts := start.Add(time.Duration(day) * 24 * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricDiskUsedBytes, Timestamp: ts, Value: float64(1000 + day*100)},
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricDiskTotalBytes, Timestamp: ts, Value: 2000},
		)
	}
	if err := tdb.WriteSamples(context.Background(), samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/capacity-forecast", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeCapacityForecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Disk == nil {
		t.Fatal("Disk = nil, want a projection for a steadily growing series")
	}
	if got.Disk.SlopeBytesPerDay <= 0 {
		t.Errorf("SlopeBytesPerDay = %v, want > 0", got.Disk.SlopeBytesPerDay)
	}
	if got.Disk.TotalBytes != 2000 {
		t.Errorf("TotalBytes = %v, want 2000", got.Disk.TotalBytes)
	}
	if got.Memory != nil {
		t.Errorf("Memory = %+v, want nil (no memory samples seeded)", got.Memory)
	}
}

// TestHandleNodeCapacityForecast_FlatDisk_OmitsField seeds a disk series
// with no real growth and checks the handler omits Disk entirely rather
// than reporting a huge or zero DaysUntilFull.
func TestHandleNodeCapacityForecast_FlatDisk_OmitsField(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-9 * 24 * time.Hour)
	var samples []telemetry.Sample
	for day := 0; day <= 9; day++ {
		ts := start.Add(time.Duration(day) * 24 * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricDiskUsedBytes, Timestamp: ts, Value: 1000},
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricDiskTotalBytes, Timestamp: ts, Value: 2000},
		)
	}
	if err := tdb.WriteSamples(context.Background(), samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/capacity-forecast", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeCapacityForecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Disk != nil {
		t.Errorf("Disk = %+v, want nil for a flat series", got.Disk)
	}
}

// TestHandleNodeCapacityForecast_GrowingMemory_JoinsOnTimestamp seeds
// memory_total_bytes/memory_available_bytes pairs (as
// HostMemoryCollector.CollectOnce writes them, same timestamp per tick)
// with shrinking availability and checks the handler reports a growing
// memory-used trend.
func TestHandleNodeCapacityForecast_GrowingMemory_JoinsOnTimestamp(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-9 * 24 * time.Hour)
	var samples []telemetry.Sample
	for day := 0; day <= 9; day++ {
		ts := start.Add(time.Duration(day) * 24 * time.Hour)
		samples = append(samples,
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricMemoryTotalBytes, Timestamp: ts, Value: 10000},
			telemetry.Sample{ResourceID: "node:node_a", Metric: telemetry.MetricMemoryAvailableBytes, Timestamp: ts, Value: float64(9000 - day*100)},
		)
	}
	if err := tdb.WriteSamples(context.Background(), samples); err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/capacity-forecast", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodeCapacityForecastResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Memory == nil {
		t.Fatal("Memory = nil, want a projection for shrinking availability")
	}
	if got.Memory.TotalBytes != 10000 {
		t.Errorf("TotalBytes = %v, want 10000", got.Memory.TotalBytes)
	}
	// used = total - available = 1000 at day 0, growing 100/day.
	if got.Memory.SlopeBytesPerDay < 99 || got.Memory.SlopeBytesPerDay > 101 {
		t.Errorf("SlopeBytesPerDay = %v, want ~100", got.Memory.SlopeBytesPerDay)
	}
}
