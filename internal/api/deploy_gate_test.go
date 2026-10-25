package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newGateRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db, cookie := newSafetyRouter(t, inspectResolver{})
	WithDeployGate(db, deploysafety.BuiltinDefaults())(rt)
	return rt, db, cookie
}

func TestDeployGate_TimelineAndStage(t *testing.T) {
	rt, db, cookie := newGateRouter(t)
	ctx := context.Background()
	seedTarget(t, db, store.DeployAttempt{ID: "dep_1", Image: "nginx:1"})
	now := time.Now()
	for i, e := range []store.DeployGateEvent{
		{Phase: deploysafety.PhaseReadiness, Status: deploysafety.StatusPassed, Message: "first readiness check passed"},
		{Phase: deploysafety.PhaseStability, Status: deploysafety.StatusPassed, Message: "3 passes"},
		{Phase: deploysafety.PhaseCutover, Status: deploysafety.StatusPassed, Message: "traffic moved"},
	} {
		e.DeployAttemptID, e.ServiceName, e.At = "dep_1", "web", now.Add(time.Duration(i)*time.Second)
		if err := db.RecordGateEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveRollbackPin(ctx, store.RollbackPin{ServiceName: "web", DeployAttemptID: "dep_1", Image: "nginx:1", ImageID: "sha256:x", PinRef: "t-rollback/web:dep_1"}); err != nil {
		t.Fatal(err)
	}

	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_1/gate", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET gate = %d %s", rec.Code, rec.Body.String())
	}
	var got deployGateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Stage != deploysafety.StageWatching || got.Terminal || len(got.Events) != 3 || got.Events[2].Phase != deploysafety.PhaseCutover {
		t.Errorf("gate = %+v, want 3 events and stage watching", got)
	}
	if got.RollbackPin == nil || got.RollbackPin.PinRef != "t-rollback/web:dep_1" || got.Settings.SuccessThreshold != 2 {
		t.Errorf("pin = %+v settings = %+v", got.RollbackPin, got.Settings)
	}

	if rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/dep_missing/gate", "")); rec.Code != http.StatusNotFound {
		t.Errorf("unknown deploy = %d, want 404", rec.Code)
	}
	rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploys/latest/gate", ""))
	var latest deployGateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &latest); rec.Code != http.StatusOK || err != nil || latest.DeployID == "" {
		t.Errorf("GET latest gate = %d %s, want the newest attempt", rec.Code, rec.Body.String())
	}
}

func TestDeploySafety_PutValidatesAndResolves(t *testing.T) {
	rt, _, cookie := newGateRouter(t)
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"bad duration", `{"stable_window":"soon"}`, http.StatusBadRequest},
		{"threshold zero", `{"success_threshold":0}`, http.StatusBadRequest},
		{"rate over one", `{"watch_error_rate":2}`, http.StatusBadRequest},
		{"valid", `{"success_threshold":4,"stable_window":"30s","auto_rollback":false}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploy-safety", tt.body))
			if rec.Code != tt.wantCode {
				t.Fatalf("PUT = %d %s, want %d", rec.Code, rec.Body.String(), tt.wantCode)
			}
		})
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-safety", ""))
	var got deploySafetyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Effective.SuccessThreshold != 4 || got.Effective.StableWindow != "30s" || got.Effective.AutoRollback || got.Defaults.SuccessThreshold != 2 {
		t.Errorf("deploy safety = %+v", got)
	}
	if got.Overrides.DrainTimeout != nil || got.Overrides.SuccessThreshold == nil {
		t.Errorf("overrides = %+v, want only the fields that were set", got.Overrides)
	}
}

func TestDeployGate_NotWiredIs501(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, inspectResolver{})
	if rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-safety", "")); rec.Code != http.StatusNotImplemented {
		t.Errorf("GET deploy-safety without the gate store = %d, want 501", rec.Code)
	}
}

// A local build whose tag was garbage collected still rolls back through
// its pin, and the rollback restores the deploy's config.
func TestRollbackTo_UsesPinAndRestoresConfig(t *testing.T) {
	rt, db, cookie := newGateRouter(t)
	ctx := context.Background()
	old := store.DesiredService{Name: "web", Image: "web:abc", Port: 80, Env: map[string]string{"MODE": "old"}}
	seedTarget(t, db, store.DeployAttempt{ID: "dep_old", Image: "web:abc", ImageDigest: "sha256:local", DigestReason: store.DigestReasonLocalBuild, Snapshot: store.NewDeployAttemptSnapshot(old)})
	cur, _ := db.GetDesiredService(ctx, "web")
	cur.Env = map[string]string{"MODE": "new"}
	cur.PullPolicy = store.PullPolicyAlways
	if err := db.SaveDesiredService(ctx, *cur); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveRollbackPin(ctx, store.RollbackPin{ServiceName: "web", DeployAttemptID: "dep_old", Image: "web:abc", ImageID: "sha256:local", PinRef: "t-rollback/web:dep_old"}); err != nil {
		t.Fatal(err)
	}

	rec := rollbackTo(rt, cookie, t, "dep_old", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("rollback = %d %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if svc.Image != "t-rollback/web:dep_old" || svc.Env["MODE"] != "old" || svc.PullPolicy != "" {
		t.Errorf("after rollback image=%q env=%v pull=%q, want the pin, the old env and no forced pull", svc.Image, svc.Env, svc.PullPolicy)
	}

	attempts, _ := db.ListDeployAttempts(ctx, "web")
	evs, _ := db.ListGateEvents(ctx, attempts[0].ID)
	found := false
	for _, e := range evs {
		found = found || e.Reason == deploysafety.ReasonOperatorRollback
	}
	if !found {
		t.Errorf("no OperatorRollback event on the rollback attempt: %+v", evs)
	}

	rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/rollback-targets", ""))
	var targets map[string][]rollbackTargetResource
	if err := json.Unmarshal(rec.Body.Bytes(), &targets); err != nil {
		t.Fatal(err)
	}
	if len(targets["targets"]) != 1 || !targets["targets"][0].Live {
		t.Errorf("rollback targets = %+v, want the pinned release reported live", targets)
	}
}
