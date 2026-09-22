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

func TestHandleFleetResourceUsage_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/resource-usage", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleFleetResourceUsage_NoNodes_ReturnsEmpty(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/resource-usage", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got fleetResourceUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Nodes) != 0 {
		t.Errorf("got %d nodes, want 0", len(got.Nodes))
	}
	if got.Fleet.NodeCount != 0 {
		t.Errorf("Fleet.NodeCount = %d, want 0", got.Fleet.NodeCount)
	}
}

// TestHandleFleetResourceUsage_SumsPlacedServicesPerNode covers the core
// aggregation: two services placed on the same node have their latest
// cpu_percent/memory_usage_bytes summed into that node's row, a service
// on a different node stays in its own row, and a node with no placed
// service reporting yet still appears with nil usage fields (a real
// zero-data node, not a missing one, matching
// handleAppResourceUsage's own "every app appears" contract).
func TestHandleFleetResourceUsage_SumsPlacedServicesPerNode(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	seedNode(t, db, "node_a", "alpha")
	seedNode(t, db, "node_b", "bravo")

	ctx := context.Background()
	// SaveDesiredService deliberately never writes NodeID (its own doc
	// comment: an ordinary redeploy has no opinion on placement), so
	// placement is a separate UpdateServiceNode call per service, the
	// same two-step shape handleDrainNode already uses.
	for _, seed := range []struct {
		svc    store.DesiredService
		nodeID string
	}{
		{store.DesiredService{Name: "web", Image: "img:1", Port: 3000}, "node_a"},
		{store.DesiredService{Name: "worker", Image: "img:1", Port: 3001}, "node_a"},
		{store.DesiredService{Name: "api", Image: "img:1", Port: 3002}, "node_b"},
	} {
		if err := db.SaveDesiredService(ctx, seed.svc); err != nil {
			t.Fatalf("seed service %s: %v", seed.svc.Name, err)
		}
		if err := db.UpdateServiceNode(ctx, seed.svc.Name, seed.nodeID); err != nil {
			t.Fatalf("place service %s on %s: %v", seed.svc.Name, seed.nodeID, err)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(ctx, []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now, Value: 30},
		{ResourceID: "service:web", Metric: "memory_usage_bytes", Timestamp: now, Value: 100},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: now, Value: 15},
		{ResourceID: "service:worker", Metric: "memory_usage_bytes", Timestamp: now, Value: 50},
		{ResourceID: "service:api", Metric: "cpu_percent", Timestamp: now, Value: 5},
		{ResourceID: "service:deleted", Metric: "cpu_percent", Timestamp: now, Value: 999}, // stale, must not appear anywhere
	})
	if err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/resource-usage", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got fleetResourceUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2, body=%s", len(got.Nodes), rec.Body.String())
	}

	byID := make(map[string]nodeResourceUsageResource, len(got.Nodes))
	for _, n := range got.Nodes {
		byID[n.NodeID] = n
	}

	alpha, ok := byID["node_a"]
	if !ok {
		t.Fatal("response missing node_a")
	}
	if alpha.CPUPercent == nil || *alpha.CPUPercent != 45 {
		t.Errorf("alpha.CPUPercent = %v, want 45 (30+15 summed)", alpha.CPUPercent)
	}
	if alpha.MemoryUsageBytes == nil || *alpha.MemoryUsageBytes != 150 {
		t.Errorf("alpha.MemoryUsageBytes = %v, want 150 (100+50 summed)", alpha.MemoryUsageBytes)
	}

	bravo, ok := byID["node_b"]
	if !ok {
		t.Fatal("response missing node_b")
	}
	if bravo.CPUPercent == nil || *bravo.CPUPercent != 5 {
		t.Errorf("bravo.CPUPercent = %v, want 5", bravo.CPUPercent)
	}
	if bravo.MemoryUsageBytes != nil {
		t.Errorf("bravo.MemoryUsageBytes = %v, want nil (never reported)", bravo.MemoryUsageBytes)
	}

	if got.Fleet.NodeCount != 2 {
		t.Errorf("Fleet.NodeCount = %d, want 2", got.Fleet.NodeCount)
	}
	if got.Fleet.TotalCPUPercent == nil || *got.Fleet.TotalCPUPercent != 50 {
		t.Errorf("Fleet.TotalCPUPercent = %v, want 50 (45+5)", got.Fleet.TotalCPUPercent)
	}
	if got.Fleet.NodesWithMemoryCapacity != 0 {
		t.Errorf("Fleet.NodesWithMemoryCapacity = %d, want 0 (no host metrics seeded)", got.Fleet.NodesWithMemoryCapacity)
	}
	if got.Fleet.MemoryUsedPercent != nil {
		t.Errorf("Fleet.MemoryUsedPercent = %v, want nil (no node reported capacity)", got.Fleet.MemoryUsedPercent)
	}
}

// TestHandleFleetResourceUsage_HostMetricsAndRollup covers the host-level
// side: a node with real disk/memory host readings (the local node,
// stamped is_local, but this handler doesn't special-case that field
// itself, it just reads whatever landed under node:<id>) gets those
// fields populated and folds into the fleet rollup's capacity-aware
// percentage, while a second node with no host readings keeps those
// fields nil and does not count toward NodesWithMemoryCapacity/
// NodesWithDiskCapacity.
func TestHandleFleetResourceUsage_HostMetricsAndRollup(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	seedNode(t, db, "node_a", "alpha")
	seedNode(t, db, "node_b", "bravo")
	rt.SetLocalNodeID("node_a")

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(ctx, []telemetry.Sample{
		{ResourceID: "node:node_a", Metric: telemetry.MetricMemoryTotalBytes, Timestamp: now, Value: 1000},
		{ResourceID: "node:node_a", Metric: telemetry.MetricDiskTotalBytes, Timestamp: now, Value: 2000},
		{ResourceID: "node:node_a", Metric: telemetry.MetricDiskUsedBytes, Timestamp: now, Value: 500},
	})
	if err != nil {
		t.Fatalf("seed telemetry: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/resource-usage", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got fleetResourceUsageResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	byID := make(map[string]nodeResourceUsageResource, len(got.Nodes))
	for _, n := range got.Nodes {
		byID[n.NodeID] = n
	}

	alpha, ok := byID["node_a"]
	if !ok {
		t.Fatal("response missing node_a")
	}
	if !alpha.IsLocal {
		t.Error("alpha.IsLocal = false, want true")
	}
	if alpha.MemoryTotalBytes == nil || *alpha.MemoryTotalBytes != 1000 {
		t.Errorf("alpha.MemoryTotalBytes = %v, want 1000", alpha.MemoryTotalBytes)
	}
	if alpha.DiskTotalBytes == nil || *alpha.DiskTotalBytes != 2000 {
		t.Errorf("alpha.DiskTotalBytes = %v, want 2000", alpha.DiskTotalBytes)
	}
	if alpha.DiskUsedBytes == nil || *alpha.DiskUsedBytes != 500 {
		t.Errorf("alpha.DiskUsedBytes = %v, want 500", alpha.DiskUsedBytes)
	}

	bravo, ok := byID["node_b"]
	if !ok {
		t.Fatal("response missing node_b")
	}
	if bravo.IsLocal {
		t.Error("bravo.IsLocal = true, want false")
	}
	if bravo.DiskTotalBytes != nil || bravo.MemoryTotalBytes != nil {
		t.Errorf("bravo host metrics = %+v, want all nil (no collector for this node)", bravo)
	}

	if got.Fleet.NodesWithDiskCapacity != 1 {
		t.Errorf("Fleet.NodesWithDiskCapacity = %d, want 1", got.Fleet.NodesWithDiskCapacity)
	}
	if got.Fleet.DiskUsedPercent == nil || *got.Fleet.DiskUsedPercent != 25 {
		t.Errorf("Fleet.DiskUsedPercent = %v, want 25 (500/2000*100)", got.Fleet.DiskUsedPercent)
	}
	if got.Fleet.NodesWithMemoryCapacity != 1 {
		t.Errorf("Fleet.NodesWithMemoryCapacity = %d, want 1", got.Fleet.NodesWithMemoryCapacity)
	}
}
