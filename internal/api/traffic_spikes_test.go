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

func TestTrafficErrorSpikeTiedToDeploy(t *testing.T) {
	t.Setenv(telemetry.EnvSpikeMinRequests, "10")
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	for i := 1; i <= 10; i++ {
		var w telemetry.RequestWindow
		w.Requests, w.Status2xx = 100, 100
		w.Latency[2] = 100
		if i == 3 {
			w.Status2xx, w.Status5xx = 40, 60
		}
		at := now.Add(-time.Duration(i)*time.Minute + 5*time.Second)
		if err := tdb.RecordRequests(ctx, map[string]telemetry.RequestWindow{"web": w}, at); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep1", ServiceName: "web", Image: "img:2", Status: store.DeployAttemptStatusSucceeded, StartedAt: now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("seed deploy: %v", err)
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
	if len(got.ErrorSpikes) != 1 {
		t.Fatalf("spikes = %+v", got.ErrorSpikes)
	}
	s := got.ErrorSpikes[0]
	if s.PeakRate5xx != 0.6 || s.DeployID != "dep1" || s.DeployImage != "img:2" {
		t.Errorf("spike = %+v", s)
	}
}
