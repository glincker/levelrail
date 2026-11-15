package store

import (
	"context"
	"testing"
	"time"
)

func TestRecoverInterruptedDeployAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	seed := func(a DeployAttempt) {
		t.Helper()
		a.StartedAt = now.Add(-time.Hour)
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("seed %s: %v", a.ID, err)
		}
	}
	seed(DeployAttempt{ID: "dep_build", ServiceName: "web", Image: "web:new", Status: DeployAttemptStatusRunning})
	seed(DeployAttempt{ID: "dep_cut", ServiceName: "api", Image: "api:new", Status: DeployAttemptStatusRunning})
	seed(DeployAttempt{ID: "dep_held", ServiceName: "job", Image: "job:new", Status: DeployAttemptStatusRunning, HeldRequest: "{}"})
	seed(DeployAttempt{ID: "dep_queued", ServiceName: "web", Image: "web:q", Status: DeployAttemptStatusQueued})
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "api", Image: "api:new", Port: 80}); err != nil {
		t.Fatalf("seed desired: %v", err)
	}

	got, err := db.RecoverInterruptedDeployAttempts(ctx, now)
	if err != nil {
		t.Fatalf("RecoverInterruptedDeployAttempts() error = %v", err)
	}
	outcomes := map[string]string{}
	for _, r := range got {
		outcomes[r.ID] = r.Outcome
	}
	want := map[string]string{"dep_build": RecoveredInterrupted, "dep_cut": RecoveredConverging, "dep_held": RecoveredRequeued}
	for id, o := range want {
		if outcomes[id] != o {
			t.Errorf("%s outcome = %q, want %q", id, outcomes[id], o)
		}
	}
	build, _ := db.GetDeployAttempt(ctx, "dep_build")
	if build.Status != DeployAttemptStatusFailed || build.Reason != DeployReasonInterrupted || build.FinishedAt == nil {
		t.Errorf("interrupted attempt = %+v", build)
	}
	cut, _ := db.GetDeployAttempt(ctx, "dep_cut")
	if cut.Status != DeployAttemptStatusSucceeded {
		t.Errorf("cut-over attempt status = %q, want succeeded", cut.Status)
	}
	queued, _ := db.GetDeployAttempt(ctx, "dep_queued")
	if queued.Status != DeployAttemptStatusQueued {
		t.Errorf("queued attempt status = %q, want queued", queued.Status)
	}

	again, err := db.RecoverInterruptedDeployAttempts(ctx, now)
	if err != nil || len(again) != 0 {
		t.Errorf("second run = %v, %v; want nothing to do", again, err)
	}
}
