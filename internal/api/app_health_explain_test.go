package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleGetAppHealthExplain_NotFound(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/health-explain", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestHandleGetAppHealthExplain_FailedDeploySuggestsRollback(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "api", Image: "img:v2", Replicas: 1}); err != nil {
		t.Fatal(err)
	}
	for _, a := range []store.DeployAttempt{
		{ID: "att_1", ServiceName: "api", Image: "img:v1", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now().Add(-2 * time.Minute)},
		{ID: "att_2", ServiceName: "api", Image: "img:v2", Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusFailed, StartedAt: time.Now().Add(-time.Minute)},
	} {
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/api/health-explain", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got appHealthExplainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != healthExplainFailing || got.NextStep.Kind != nextStepRollback {
		t.Fatalf("state=%q next=%q, want failing/rollback", got.State, got.NextStep.Kind)
	}
}
