package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAppStatusOutcome(t *testing.T) {
	t.Setenv("APP_DEPLOY_ROLLOUT_CRASHLOOP_RESTARTS", "4")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedFailedDeploy(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got appStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.DeployID != "dep_bad" || got.Outcome != OutcomeFailed || got.OutcomeReason == "" || got.RolloutCrashLoopRestarts != 4 {
		t.Errorf("status = %+v, want the newest attempt's failed outcome, reason and threshold", got)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/nope/status", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown app = %d, want 404", rec.Code)
	}
}
