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

func TestTrafficNotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/traffic", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestTrafficValidation(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want int
	}{
		{"/api/v1/apps/ghost/traffic", http.StatusNotFound},
		{"/api/v1/apps/web/traffic?window=2d", http.StatusBadRequest},
		{"/api/v1/apps/web/traffic", http.StatusOK},
		{"/api/v1/apps/web/traffic?window=24h", http.StatusOK},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, c.path, ""))
		if rec.Code != c.want {
			t.Errorf("GET %s = %d, want %d: %s", c.path, rec.Code, c.want, rec.Body.String())
		}
	}
}

func TestTrafficPopulated(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	at := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)

	var w telemetry.RequestWindow
	w.Requests, w.Status2xx, w.Status5xx = 100, 95, 5
	w.Latency[2], w.Latency[5] = 90, 10
	for i := 0; i < 8; i++ {
		w.ObserveRoute("/slow", 200, 900, 50)
	}
	for i := 0; i < 8; i++ {
		w.ObserveRoute("/fast", 200, 5, 50)
	}
	for i := 0; i < 6; i++ {
		w.ObserveUpstream("172.18.0.2:3000", 200, false, 20, 4)
	}
	w.ObserveUpstream("172.18.0.3:3000", 502, true, 3000, 4)
	w.AddSample(telemetry.RequestSample{Time: at, Route: "/slow", Method: "GET", Status: 502, DurationMS: 3000, Upstream: "172.18.0.3:3000", RequestID: "req1"}, 10)
	if err := tdb.RecordRequests(context.Background(), map[string]telemetry.RequestWindow{"web": w}, at); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/traffic?window=1h", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got trafficResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.HasTraffic || got.Requests != 100 || got.Statuses.Status5xx != 5 || got.Statuses.Status2xx != 95 {
		t.Errorf("totals = %+v", got)
	}
	if got.Latency.P99Ms <= 0 {
		t.Errorf("p99 = %v", got.Latency.P99Ms)
	}
	if len(got.Replicas) != 2 || got.Replicas[0].Upstream != "172.18.0.2:3000" || got.Replicas[1].ErrorRate5xx != 1 {
		t.Errorf("replicas = %+v", got.Replicas)
	}
	if len(got.SlowRoutes) != 2 || got.SlowRoutes[0].Route != "/slow" {
		t.Errorf("slow routes = %+v", got.SlowRoutes)
	}
	if len(got.Samples) != 1 || got.Samples[0].RequestID != "req1" {
		t.Errorf("samples = %+v", got.Samples)
	}
	if got.SLO != nil {
		t.Errorf("slo = %+v, want nil without a rule", got.SLO)
	}
}

func TestTopRoutes_RanksByRequestsAndSkipsOverflow(t *testing.T) {
	b := telemetry.Breakdown{Routes: []telemetry.RouteRow{
		{Route: "/a", Requests: 5, LatencyMSSum: 50},
		{Route: "/b", Requests: 9, Errors5xx: 3, LatencyMSSum: 90},
		{Route: telemetry.OverflowRoute, Requests: 99},
	}}
	got := topRoutes(b, 10)
	if len(got) != 2 || got[0].Route != "/b" || got[0].ErrorRate5xx < 0.33 || got[0].ErrorRate5xx > 0.34 {
		t.Fatalf("topRoutes = %+v", got)
	}
}
