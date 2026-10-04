package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleListProbeAttempts(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "levelrail/web:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	base := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: "dep_1", ServiceName: "web", Image: "levelrail/web:1",
		Status: store.DeployAttemptStatusRunning, StartedAt: base,
	}); err != nil {
		t.Fatalf("seed attempt: %v", err)
	}

	// No probe attempts yet: empty array, not null, not an error.
	recEmpty := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recEmpty, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_1/probes", ""))
	if recEmpty.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recEmpty.Code, http.StatusOK, recEmpty.Body.String())
	}
	if strings.TrimSpace(recEmpty.Body.String()) != "[]" {
		t.Errorf("body = %q, want an empty JSON array", recEmpty.Body.String())
	}

	if err := db.FinishDeployAttempt(ctx, "dep_1", store.DeployAttemptStatusSucceeded, base.Add(time.Second), ""); err != nil {
		t.Fatalf("finish attempt: %v", err)
	}
	if err := db.RecordProbeAttempt(ctx, store.ProbeAttempt{
		ServiceName: "web", Image: "levelrail/web:1",
		Target: "10.0.0.5:3000", Success: false, StatusCode: 503, LatencyMS: 12,
		ProbedAt: base.Add(10 * time.Millisecond),
	}); err != nil {
		t.Fatalf("seed probe attempt: %v", err)
	}
	if err := db.RecordProbeAttempt(ctx, store.ProbeAttempt{
		ServiceName: "web", Image: "levelrail/web:1",
		Target: "10.0.0.5:3000", Success: true, StatusCode: 200, LatencyMS: 3,
		ProbedAt: base.Add(20 * time.Millisecond),
	}); err != nil {
		t.Fatalf("seed probe attempt: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_1/probes", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got []probeAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d probe attempts, want 2", len(got))
	}
	if got[0].Success || got[0].StatusCode != 503 || got[0].Target != "10.0.0.5:3000" {
		t.Errorf("first = %+v", got[0])
	}
	if !got[1].Success || got[1].StatusCode != 200 {
		t.Errorf("second = %+v", got[1])
	}

	// A deploy attempt that belongs to a different app must 404, the same
	// boundary handleDeployStepStream already enforces.
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "other", Image: "levelrail/other:1", Port: 3000}); err != nil {
		t.Fatalf("seed other app: %v", err)
	}
	recWrongApp := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recWrongApp, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/other/deploys/dep_1/probes", ""))
	if recWrongApp.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for a deploy attempt belonging to a different app", recWrongApp.Code, http.StatusNotFound)
	}

	recMissing := httptest.NewRecorder()
	rt.Handler().ServeHTTP(recMissing, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_missing/probes", ""))
	if recMissing.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d for an unknown deploy attempt", recMissing.Code, http.StatusNotFound)
	}
}
