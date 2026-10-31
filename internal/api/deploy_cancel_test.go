package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func saveAttempt(t *testing.T, db *store.DB, id, status string) {
	t.Helper()
	a := store.DeployAttempt{
		ID: id, ServiceName: "web", Image: "web:2", Source: store.DeployAttemptSourceManual,
		Status: status, StartedAt: time.Now(),
	}
	if err := db.SaveDeployAttempt(context.Background(), a); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}
}

func cancelRequest(t *testing.T, rt *Router, cookie *http.Cookie, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys/"+id+"/cancel", ""))
	return rec
}

func TestHandleCancelDeploy_QueuedAttemptIsCanceled(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveAttempt(t, db, "att_queued", store.DeployAttemptStatusQueued)

	rec := cancelRequest(t, rt, cookie, "att_queued")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDeployAttempt(context.Background(), "att_queued")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != store.DeployAttemptStatusCanceled {
		t.Errorf("status = %q, want canceled", got.Status)
	}
}

func TestHandleCancelDeploy_FinishedAttemptConflicts(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveAttempt(t, db, "att_done", store.DeployAttemptStatusSucceeded)

	if rec := cancelRequest(t, rt, cookie, "att_done"); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDeployAttempt(context.Background(), "att_done")
	if err != nil || got.Status != store.DeployAttemptStatusSucceeded {
		t.Errorf("a refused cancel must not change the attempt: %+v, %v", got, err)
	}
}

func TestHandleCancelDeploy_RunningWithoutLiveBuildConflicts(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveAttempt(t, db, "att_stale", store.DeployAttemptStatusRunning)

	rec := cancelRequest(t, rt, cookie, "att_stale")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 for a running attempt no build is driving; body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCancelDeploy_UnknownAttemptIsNotFound(t *testing.T) {
	t.Parallel()
	rt, _, cookie := newTimelineRouter(t)
	if rec := cancelRequest(t, rt, cookie, "att_missing"); rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleCancelDeploy_NotConfiguredIs501(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	if rec := cancelRequest(t, rt, cookie, "att_x"); rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body = %s", rec.Code, rec.Body.String())
	}
}
