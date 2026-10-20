package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDesiredServiceImageIDRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	svc := DesiredService{Name: "web", Image: "web:abc", ImageID: "sha256:111", ImageIDRef: "web:abc", Port: 80}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if got.ImageID != "sha256:111" || got.LocalImageID() != "sha256:111" {
		t.Fatalf("image id = %q / %q", got.ImageID, got.LocalImageID())
	}
	got.Image = "nginx:1.27"
	if got.LocalImageID() != "" {
		t.Fatalf("LocalImageID must not survive an image change, got %q", got.LocalImageID())
	}
}

func TestNextDeploySequenceIsMonotonic(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for want := int64(1); want <= 3; want++ {
		got, err := db.NextDeploySequence(ctx, "web")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("seq = %d, want %d", got, want)
		}
	}
	other, err := db.NextDeploySequence(ctx, "api")
	if err != nil || other != 1 {
		t.Fatalf("other service seq = %d, %v", other, err)
	}
}

var errTestStale = errors.New("stale")

func TestSaveDesiredServiceOrdered(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	rejectOlder := func(c DeployCursor, o DeployOrder) error {
		if o.Sequence < c.AppliedSequence {
			return errTestStale
		}
		return nil
	}

	if err := db.SaveDesiredServiceOrdered(ctx, DesiredService{Name: "web", Image: "web:b"}, DeployOrder{Sequence: 5, Automatic: true, CommitSHA: "b", Before: "a", CommitAt: at}, rejectOlder); err != nil {
		t.Fatal(err)
	}
	err := db.SaveDesiredServiceOrdered(ctx, DesiredService{Name: "web", Image: "web:a"}, DeployOrder{Sequence: 4, Automatic: true, CommitSHA: "a"}, rejectOlder)
	if !errors.Is(err, errTestStale) {
		t.Fatalf("older save err = %v, want stale", err)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if got.Image != "web:b" {
		t.Fatalf("rejected save still wrote image %q", got.Image)
	}

	if err := db.SaveDesiredServiceOrdered(ctx, DesiredService{Name: "web", Image: "web:old"}, DeployOrder{Sequence: 6}, rejectOlder); err != nil {
		t.Fatal(err)
	}
	c, err := db.GetDeployCursor(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if c.AppliedSequence != 6 || c.CommitSHA != "b" || c.Before != "a" || !c.CommitAt.Equal(at) {
		t.Fatalf("cursor after manual = %+v, want seq 6 keeping commit b", c)
	}
	next, _ := db.NextDeploySequence(ctx, "web")
	if next <= 6 {
		t.Fatalf("next sequence %d must be above the applied one", next)
	}
}

func TestDeployAttemptSafetyColumns(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	started := time.Now().Add(-time.Minute)
	a := DeployAttempt{ID: "dep_1", ServiceName: "web", Image: "nginx:latest@sha256:aa", Status: DeployAttemptStatusRunning, StartedAt: started, Sequence: 3, ImageDigest: "sha256:aa", DigestReason: DigestReasonResolved}
	if err := db.SaveDeployAttempt(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishDeployAttempt(ctx, "dep_1", DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordRollout(ctx, "web", a.Image, RolloutStateServing, "sha256:cfg"); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDeployAttempt(ctx, "dep_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Sequence != 3 || got.ImageDigest != "sha256:aa" || got.DigestReason != DigestReasonResolved || got.RolloutState != RolloutStateServing || got.RunningImageID != "sha256:cfg" {
		t.Fatalf("attempt = %+v", got)
	}

	held := DeployAttempt{ID: "dep_2", ServiceName: "web", Image: "nginx:1", Status: DeployAttemptStatusHeld, StartedAt: time.Now(), Reason: DeployReasonFrozen, HeldRequest: `{"kind":"image"}`}
	if err := db.SaveDeployAttempt(ctx, held); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListHeldDeployAttempts(ctx)
	if err != nil || len(list) != 1 || list[0].HeldRequest != held.HeldRequest {
		t.Fatalf("held list = %+v, %v", list, err)
	}
	moved, err := db.TransitionDeployAttempt(ctx, "dep_2", DeployAttemptStatusHeld, DeployAttemptStatusRunning, "")
	if err != nil || !moved {
		t.Fatalf("first transition = %v, %v", moved, err)
	}
	moved, err = db.TransitionDeployAttempt(ctx, "dep_2", DeployAttemptStatusHeld, DeployAttemptStatusRunning, "")
	if err != nil || moved {
		t.Fatalf("second transition must be a no-op, got %v, %v", moved, err)
	}
}

func TestReplaceDeployFreezeWindows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	saved, err := db.ReplaceDeployFreezeWindows(ctx, DeployFreezeScopeApp("web"), []DeployFreezeWindow{
		{Cron: "0 17 * * 5", Duration: 64 * time.Hour, Timezone: "Europe/Berlin", Reason: "weekend"},
	})
	if err != nil || len(saved) != 1 || saved[0].ID == "" {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	if _, err := db.ReplaceDeployFreezeWindows(ctx, DeployFreezeScopeGlobal, []DeployFreezeWindow{{Cron: "0 0 24 12 *", Duration: 24 * time.Hour}}); err != nil {
		t.Fatal(err)
	}
	all, err := db.ListDeployFreezeWindows(ctx, DeployFreezeScopeApp("web"), DeployFreezeScopeGlobal)
	if err != nil || len(all) != 2 {
		t.Fatalf("list = %+v, %v", all, err)
	}
	if all[1].Timezone != "UTC" || all[0].Duration != 64*time.Hour {
		t.Fatalf("defaults not applied: %+v", all)
	}
	if _, err := db.ReplaceDeployFreezeWindows(ctx, DeployFreezeScopeApp("web"), nil); err != nil {
		t.Fatal(err)
	}
	left, _ := db.ListDeployFreezeWindows(ctx, DeployFreezeScopeApp("web"))
	if len(left) != 0 {
		t.Fatalf("clear left %d windows", len(left))
	}
}
