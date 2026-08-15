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

func TestHandleQueryNodeMetrics_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithTelemetryQuerier
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleQueryNodeMetrics_NodeNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/nonexistent/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleQueryNodeMetrics_MissingMetricParam(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/metrics", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestHandleQueryNodeMetrics_RejectsMemoryLimit is the one case worth a
// dedicated test: memory_limit_bytes is a real MetricName the per-app
// endpoint happily serves, and it would silently "work" here too
// (Query would just return whatever was collected) while being actively
// misleading once summed across containers, see nodeSummableMetrics'
// own doc comment. This is the one metric that must be rejected, not
// just quietly aggregated.
func TestHandleQueryNodeMetrics_RejectsMemoryLimit(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/metrics?metric=memory_limit_bytes", ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

// TestHandleQueryNodeMetrics_SumsAcrossPlacedServices is the real
// end-to-end case: two services placed on the same node, each with its
// own collected samples, must come back as one summed series with
// resource_count reflecting both having contributed.
func TestHandleQueryNodeMetrics_SumsAcrossPlacedServices(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed web: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "web", "node_a"); err != nil {
		t.Fatalf("place web on node_a: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "worker", Image: "img:1", Port: 3001}); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	if err := db.UpdateServiceNode(ctx, "worker", "node_a"); err != nil {
		t.Fatalf("place worker on node_a: %v", err)
	}
	// A third service exists but is placed on a different node: its
	// samples must not leak into node_a's sum.
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "elsewhere", Image: "img:1", Port: 3002}); err != nil {
		t.Fatalf("seed elsewhere: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteSamples(ctx, []telemetry.Sample{
		{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: now.Add(-10 * time.Minute), Value: 10},
		{ResourceID: "service:worker", Metric: "cpu_percent", Timestamp: now.Add(-10 * time.Minute), Value: 25},
		{ResourceID: "service:elsewhere", Metric: "cpu_percent", Timestamp: now.Add(-10 * time.Minute), Value: 1000},
	})
	if err != nil {
		t.Fatalf("seed samples: %v", err)
	}

	rec := httptest.NewRecorder()
	url := "/api/v1/nodes/node_a/metrics?metric=cpu_percent&from=" + now.Add(-time.Hour).Format(time.RFC3339) + "&to=" + now.Format(time.RFC3339)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, url, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got nodeMetricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Points) != 1 {
		t.Fatalf("points = %d, want 1", len(got.Points))
	}
	if got.Points[0].Value != 35 {
		t.Errorf("summed value = %v, want 35 (10 from web + 25 from worker, not the 1000 from elsewhere)", got.Points[0].Value)
	}
	if got.ResourceCount != 2 {
		t.Errorf("resource_count = %d, want 2 (web and worker, not elsewhere)", got.ResourceCount)
	}
}

// TestHandleQueryNodeMetrics_NoPlacedServices_ReturnsEmptyNotError is
// the "node with nothing on it yet" case: a valid, non-error state, the
// same convention Query's own doc comment establishes at the store
// layer.
func TestHandleQueryNodeMetrics_NoPlacedServices_ReturnsEmptyNotError(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedNode(t, db, "node_a", "alpha")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/nodes/node_a/metrics?metric=cpu_percent", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got nodeMetricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Points) != 0 {
		t.Errorf("points = %d, want 0", len(got.Points))
	}
	if got.ResourceCount != 0 {
		t.Errorf("resource_count = %d, want 0", got.ResourceCount)
	}
}
