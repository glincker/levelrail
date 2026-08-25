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

func TestHandleGetNodePatchStatus_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/patch-status", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleGetNodePatchStatus_NodeNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/nonexistent/patch-status", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestHandleGetNodePatchStatus_NoSample_ReportsUnchecked is the "never
// checked" case (HostPatchCollector hasn't run yet, or found no
// supported package manager on the host): checked must come back false,
// not a false Total == 0.
func TestHandleGetNodePatchStatus_NoSample_ReportsUnchecked(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/patch-status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodePatchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Checked {
		t.Errorf("checked = true, want false when no sample has ever been written")
	}
	if got.CheckedAt != nil {
		t.Errorf("checked_at = %v, want nil", got.CheckedAt)
	}
}

func TestHandleGetNodePatchStatus_ReadsLatestSampleDirectly(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSPatchesAvailable, Timestamp: now.Add(-time.Hour), Value: 5},
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSSecurityPatchesAvailable, Timestamp: now.Add(-time.Hour), Value: 2},
		// A later sample for the same node must win as "latest".
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSPatchesAvailable, Timestamp: now, Value: 3},
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSSecurityPatchesAvailable, Timestamp: now, Value: 1},
		// A different node's samples must never leak into node_a's reading.
		{ResourceID: "node:node_b", Metric: telemetry.MetricOSPatchesAvailable, Timestamp: now, Value: 999},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/patch-status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodePatchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Checked {
		t.Fatal("checked = false, want true")
	}
	if got.Total != 3 {
		t.Errorf("total = %d, want 3 (the latest sample, not the earlier 5 or node_b's 999)", got.Total)
	}
	if got.Security != 1 {
		t.Errorf("security = %d, want 1", got.Security)
	}
	if got.CheckedAt == nil {
		t.Fatal("checked_at = nil, want a timestamp")
	}
	if !got.CheckedAt.Equal(now) {
		t.Errorf("checked_at = %v, want %v", got.CheckedAt, now)
	}
}

// TestHandleGetNodePatchStatus_UpToDate_ReportsCheckedWithZero is the
// "checked, genuinely nothing pending" case: Total == 0 with
// Checked == true must still be distinguishable from the never-checked
// case above.
func TestHandleGetNodePatchStatus_UpToDate_ReportsCheckedWithZero(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSPatchesAvailable, Timestamp: now, Value: 0},
		{ResourceID: "node:node_a", Metric: telemetry.MetricOSSecurityPatchesAvailable, Timestamp: now, Value: 0},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/patch-status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got nodePatchStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Checked || got.Total != 0 || got.Security != 0 {
		t.Errorf("got = %+v, want checked=true total=0 security=0", got)
	}
}
