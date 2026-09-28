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

func seedRequestTraffic(t *testing.T, tdb *telemetry.DB, app string, at time.Time) {
	t.Helper()
	var w telemetry.RequestWindow
	w.Requests, w.Status2xx, w.Status5xx = 100, 95, 5
	w.Latency[2] = 90
	w.Latency[5] = 10
	if err := tdb.RecordRequests(context.Background(), map[string]telemetry.RequestWindow{app: w}, at); err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
}

func TestHandleQueryRequests_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/requests", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestHandleQueryRequests_AppNotFound(t *testing.T) {
	rt, db, _ := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/ghost/requests", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleQueryRequests_EmptyAndPopulated(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}

	get := func(path string) requestsResponse {
		t.Helper()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
		}
		var out requestsResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	empty := get("/api/v1/apps/web/requests")
	if len(empty.Points) != 0 || empty.Summary.HasTraffic {
		t.Errorf("empty response = %+v", empty)
	}

	seedRequestTraffic(t, tdb, "web", time.Now().Add(-2*time.Minute).UTC().Truncate(time.Second))
	got := get("/api/v1/apps/web/requests?step=15s")
	if len(got.Points) != 1 || got.Points[0].Requests != 100 || got.Points[0].ErrorRate5xx != 0.05 {
		t.Fatalf("points = %+v", got.Points)
	}
	if !got.Summary.HasTraffic || got.Summary.Requests != 100 || got.Summary.P95Ms <= 0 {
		t.Errorf("summary = %+v", got.Summary)
	}
}

func TestHandleGetApp_IncludesRequestSummary(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	seedRequestTraffic(t, tdb, "web", time.Now().Add(-time.Minute).UTC().Truncate(time.Second))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Name     string                    `json:"name"`
		Requests *telemetry.RequestSummary `json:"requests"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != "web" || body.Requests == nil || !body.Requests.HasTraffic {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestDefaultRequestStep(t *testing.T) {
	tests := []struct {
		span time.Duration
		want time.Duration
	}{
		{time.Hour, 30 * time.Second},
		{10 * time.Minute, 15 * time.Second},
		{24 * time.Hour, 12 * time.Minute},
	}
	for _, tc := range tests {
		if got := defaultRequestStep(tc.span); got != tc.want {
			t.Errorf("defaultRequestStep(%v) = %v, want %v", tc.span, got, tc.want)
		}
	}
}
