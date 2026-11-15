package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func saveOldRunning(t *testing.T, db *store.DB, id string, age time.Duration) {
	t.Helper()
	a := store.DeployAttempt{
		ID: id, ServiceName: "web", Image: "web:3", Source: store.DeployAttemptSourceWebhook,
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now().Add(-age),
	}
	if err := db.SaveDeployAttempt(context.Background(), a); err != nil {
		t.Fatalf("SaveDeployAttempt() error = %v", err)
	}
}

func TestHandleCancelDeploy_OrphanedRunningIsFinalized(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveOldRunning(t, db, "att_orphan", 48*time.Minute)

	rec := cancelRequest(t, rt, cookie, "att_orphan")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDeployAttempt(context.Background(), "att_orphan")
	if err != nil {
		t.Fatalf("GetDeployAttempt() error = %v", err)
	}
	if got.Status != store.DeployAttemptStatusCanceled || got.Reason != store.DeployReasonOrphaned || got.FinishedAt.IsZero() {
		t.Errorf("attempt = status %q reason %q finished %v, want canceled/orphaned/set", got.Status, got.Reason, got.FinishedAt)
	}
}

func TestHandleCancelDeploy_YoungRunningWithoutBuildStaysConflict(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveOldRunning(t, db, "att_young", time.Second)

	if rec := cancelRequest(t, rt, cookie, "att_young"); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 inside the grace window; body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDeployAttempt(context.Background(), "att_young")
	if err != nil || got.Status != store.DeployAttemptStatusRunning {
		t.Errorf("attempt must stay running: %+v, %v", got, err)
	}
}

func TestHandleCancelDeploy_RegisteredBuildIsNotOrphaned(t *testing.T) {
	t.Parallel()
	rt, db, cookie := newTimelineRouter(t)
	saveOldRunning(t, db, "att_live", 48*time.Minute)
	rt.cancels.Register("att_live")

	if rec := cancelRequest(t, rt, cookie, "att_live"); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDeployAttempt(context.Background(), "att_live")
	if err != nil || got.Reason != store.DeployReasonCanceled {
		t.Errorf("a live build is canceled normally, not orphaned: %+v, %v", got, err)
	}
}
