package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func investigateFixture(t *testing.T) (*Router, *store.DB, *telemetry.DB, *http.Cookie) {
	t.Helper()
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	return rt, db, tdb, cookie
}

func getObs(t *testing.T, rt *Router, cookie *http.Cookie, path string, out any) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func TestHandleInvestigate(t *testing.T) {
	rt, db, tdb, cookie := investigateFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	spike := now.Add(-10 * time.Minute)

	var w telemetry.RequestWindow
	w.Requests, w.Status2xx, w.Status5xx = 100, 90, 10
	w.Latency[8] = 100
	w.ObserveRoute("/api/orders", 200, 800, 50)
	w.ObserveRoute("/api/orders", 500, 900, 50)
	w.ObserveRoute("/healthz", 200, 1, 50)
	if err := tdb.RecordRequests(ctx, map[string]telemetry.RequestWindow{"web": w}, spike); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		ts := spike.Add(time.Duration(i) * 15 * time.Second)
		if err := tdb.WriteSamples(ctx, []telemetry.Sample{
			{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: 97},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tdb.RecordContainerRestart(ctx, "web", spike.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dpl_1", ServiceName: "web", Image: "img:2", Status: store.DeployAttemptStatusSucceeded,
		StartedAt: spike.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	q := url.Values{}
	q.Set("from", spike.Add(-2*time.Minute).Format(time.RFC3339))
	q.Set("to", spike.Add(5*time.Minute).Format(time.RFC3339))
	var got investigateResponse
	getObs(t, rt, cookie, "/api/v1/apps/web/investigate?"+q.Encode(), &got)

	if !got.Summary.HasTraffic || got.Summary.Requests != 100 {
		t.Errorf("summary = %+v", got.Summary)
	}
	if got.Baseline.HasTraffic {
		t.Errorf("baseline should be empty, got %+v", got.Baseline)
	}
	if len(got.TopRoutes) < 2 || got.TopRoutes[0].Route != "/api/orders" || got.TopRoutes[0].ErrorRate5xx != 0.5 {
		t.Errorf("top routes = %+v", got.TopRoutes)
	}
	if len(got.StatusCodes) == 0 || got.StatusCodes[0].Status != 200 {
		t.Errorf("status codes = %+v", got.StatusCodes)
	}
	kinds := map[string]bool{}
	var last time.Time
	for _, e := range got.Timeline {
		kinds[e.Kind] = true
		if e.At.Before(last) {
			t.Errorf("timeline not ascending: %+v", got.Timeline)
		}
		last = e.At
	}
	for _, want := range []string{"deploy", "restart", "saturation"} {
		if !kinds[want] {
			t.Errorf("timeline missing %q: %+v", want, got.Timeline)
		}
	}
}

func TestHandleInvestigate_Validation(t *testing.T) {
	rt, _, _, cookie := investigateFixture(t)
	now := time.Now().UTC()
	cases := []struct {
		name  string
		query url.Values
		want  int
	}{
		{"inverted", url.Values{"from": {now.Format(time.RFC3339)}, "to": {now.Add(-time.Hour).Format(time.RFC3339)}}, http.StatusBadRequest},
		{"too wide", url.Values{"from": {now.Add(-72 * time.Hour).Format(time.RFC3339)}, "to": {now.Format(time.RFC3339)}}, http.StatusBadRequest},
		{"ok default", url.Values{}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/investigate?"+tc.query.Encode(), ""))
			if rec.Code != tc.want {
				t.Errorf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestHandleFailureContext(t *testing.T) {
	rt, db, tdb, cookie := investigateFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	var healthy failureContextResponse
	getObs(t, rt, cookie, "/api/v1/apps/web/failure-context", &healthy)
	if healthy.State != "healthy" || len(healthy.Lines) != 0 {
		t.Fatalf("healthy = %+v", healthy)
	}

	var entries []telemetry.LogEntry
	for i := 0; i < 250; i++ {
		entries = append(entries, telemetry.LogEntry{
			ResourceID: "service:web", ContainerID: "c1", Stream: "stderr",
			Timestamp: now.Add(-time.Duration(250-i) * time.Second), Message: "boom",
		})
	}
	if err := tdb.WriteLogBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := tdb.RecordContainerRestart(ctx, "web", now.Add(-time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	var loop failureContextResponse
	getObs(t, rt, cookie, "/api/v1/apps/web/failure-context", &loop)
	if loop.State != "crashlooping" || loop.RestartsInWin != 4 {
		t.Fatalf("crashloop = %+v", loop)
	}
	if len(loop.Lines) != 200 || loop.TotalLines != 250 || loop.ContainerID != "c1" {
		t.Errorf("lines = %d total = %d container = %q", len(loop.Lines), loop.TotalLines, loop.ContainerID)
	}

	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dpl_f", ServiceName: "web", Image: "img:3", Status: store.DeployAttemptStatusFailed,
		StartedAt: now.Add(-time.Minute), Error: "readiness probe failed",
	}); err != nil {
		t.Fatal(err)
	}
	var both failureContextResponse
	getObs(t, rt, cookie, "/api/v1/apps/web/failure-context", &both)
	if both.State != "crashlooping" || both.Deploy == nil || both.Deploy.ID != "dpl_f" {
		t.Errorf("both = %+v", both)
	}
}
