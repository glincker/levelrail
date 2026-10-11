package store

import (
	"context"
	"testing"
	"time"
)

func saveSucceededAttempt(t *testing.T, db *DB, id, service, image string, started time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: id, ServiceName: service, Image: image, Status: DeployAttemptStatusRunning, StartedAt: started}); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, id, DeployAttemptStatusSucceeded, started.Add(time.Second), ""); err != nil {
		t.Fatal(err)
	}
}

func TestDeploySafetySettings_RoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	empty, err := db.GetDeploySafetySettings(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if empty.SuccessThreshold != nil || empty.AutoRollback != nil || empty.StableWindow != nil {
		t.Fatalf("GetDeploySafetySettings() with no row = %+v, want every field nil", empty)
	}

	threshold, window, rate, off := 3, 20*time.Second, 0.25, false
	if err := db.SaveDeploySafetySettings(ctx, DeploySafetySettings{ServiceName: "web", SuccessThreshold: &threshold, StableWindow: &window, WatchErrorRate: &rate, AutoRollback: &off}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDeploySafetySettings(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.SuccessThreshold == nil || *got.SuccessThreshold != 3 || got.StableWindow == nil || *got.StableWindow != window ||
		got.WatchErrorRate == nil || *got.WatchErrorRate != 0.25 || got.AutoRollback == nil || *got.AutoRollback || got.DrainTimeout != nil {
		t.Errorf("round trip = %+v", got)
	}

	if err := db.SaveDeploySafetySettings(ctx, DeploySafetySettings{ServiceName: "web"}); err != nil {
		t.Fatal(err)
	}
	cleared, _ := db.GetDeploySafetySettings(ctx, "web")
	if cleared.SuccessThreshold != nil || cleared.AutoRollback != nil {
		t.Errorf("saving an empty value should clear overrides, got %+v", cleared)
	}
}

func TestGateEvents_ResolveAttemptAndList(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now()
	saveSucceededAttempt(t, db, "dep_old", "web", "app:v1", now.Add(-2*time.Hour))
	saveSucceededAttempt(t, db, "dep_new", "web", "app:v2", now.Add(-time.Minute))

	for _, e := range []DeployGateEvent{
		{ServiceName: "web", Image: "app:v2", Phase: "readiness", Status: "passed", At: now},
		{ServiceName: "web", Image: "app:v2", Phase: "cutover", Status: "passed", At: now.Add(time.Second)},
		{ServiceName: "web", Image: "app:unknown", Phase: "cutover", Status: "passed", At: now},
		{DeployAttemptID: "dep_old", ServiceName: "web", Phase: "watch", Status: "passed", At: now},
	} {
		if err := db.RecordGateEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	got, err := db.ListGateEvents(ctx, "dep_new")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Phase != "readiness" || got[1].Phase != "cutover" {
		t.Fatalf("ListGateEvents(dep_new) = %+v, want readiness then cutover", got)
	}

	since, err := db.ListGateEventsSince(ctx, "cutover", "passed", now.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(since) != 1 || since[0].DeployAttemptID != "dep_new" {
		t.Errorf("ListGateEventsSince() = %+v, want only dep_new's cutover (unknown image inserts nothing)", since)
	}
}

func TestFailDeployAttemptWithReason_OnlyFromSucceeded(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	saveSucceededAttempt(t, db, "dep_1", "web", "app:v1", time.Now())

	changed, err := db.FailDeployAttemptWithReason(ctx, "dep_1", "PostDeployCrashloop", "restarted 3 times")
	if err != nil || !changed {
		t.Fatalf("FailDeployAttemptWithReason() = %v, %v, want a change", changed, err)
	}
	a, _ := db.GetDeployAttempt(ctx, "dep_1")
	if a.Status != DeployAttemptStatusFailed || a.Reason != "PostDeployCrashloop" || a.RolloutState != RolloutStateFailed {
		t.Errorf("attempt = %+v", a)
	}
	again, _ := db.FailDeployAttemptWithReason(ctx, "dep_1", "x", "y")
	if again {
		t.Error("second FailDeployAttemptWithReason() changed an already failed attempt")
	}

	if err := db.MarkRolloutFailureReason(ctx, "web", "app:v1", "StabilityGateTimeout"); err != nil {
		t.Fatal(err)
	}
	a, _ = db.GetDeployAttempt(ctx, "dep_1")
	if a.Reason != "StabilityGateTimeout" {
		t.Errorf("Reason = %q after MarkRolloutFailureReason", a.Reason)
	}
}

func TestRollbackPins(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Now()
	for i, id := range []string{"dep_a", "dep_b"} {
		if err := db.SaveRollbackPin(ctx, RollbackPin{ServiceName: "web", DeployAttemptID: id, Image: "app:" + id, ImageID: "sha256:" + id, PinRef: "pin/web:" + id, CreatedAt: base.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}
	pins, err := db.ListRollbackPins(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if len(pins) != 2 || pins[0].DeployAttemptID != "dep_b" {
		t.Fatalf("ListRollbackPins() = %+v, want newest first", pins)
	}
	pinned, err := db.IsImagePinned(ctx, "sha256:dep_a")
	if err != nil || !pinned {
		t.Errorf("IsImagePinned(dep_a) = %v, %v", pinned, err)
	}
	if err := db.DeleteRollbackPin(ctx, "web", "dep_a"); err != nil {
		t.Fatal(err)
	}
	if p, _ := db.GetRollbackPin(ctx, "web", "dep_a"); p != nil {
		t.Errorf("GetRollbackPin() after delete = %+v", p)
	}
	if pinned, _ := db.IsImagePinned(ctx, "sha256:dep_a"); pinned {
		t.Error("image still reported pinned after its pin was deleted")
	}
}
