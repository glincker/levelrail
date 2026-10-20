package store

import (
	"context"
	"testing"
	"time"
)

func TestListPrunableDeployAttempts(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	add := func(id, status string, age time.Duration) {
		t.Helper()
		a := DeployAttempt{ID: id, ServiceName: "web", Image: "web:" + id, Source: DeployAttemptSourceImage, Status: status, StartedAt: now.Add(-age)}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	add("old-failed", DeployAttemptStatusFailed, 60*24*time.Hour)
	add("old-ok", DeployAttemptStatusSucceeded, 59*24*time.Hour)
	add("old-pinned", DeployAttemptStatusFailed, 58*24*time.Hour)
	add("recent-failed", DeployAttemptStatusFailed, time.Hour)
	add("newest-old-failed", DeployAttemptStatusFailed, 40*24*time.Hour)
	if err := db.SaveRollbackPin(ctx, RollbackPin{ServiceName: "web", DeployAttemptID: "old-pinned", Image: "web:x", ImageID: "sha256:x", PinRef: "pin"}); err != nil {
		t.Fatalf("pin: %v", err)
	}

	got, err := db.ListPrunableDeployAttempts(ctx, now.Add(-30*24*time.Hour), 1, 100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	ids := map[string]bool{}
	for _, a := range got {
		ids[a.ID] = true
	}
	for _, want := range []string{"old-failed", "newest-old-failed"} {
		if !ids[want] {
			t.Errorf("%s should be prunable, got %v", want, ids)
		}
	}
	for _, keep := range []string{"old-ok", "old-pinned", "recent-failed"} {
		if ids[keep] {
			t.Errorf("%s must be kept", keep)
		}
	}

	if err := db.DeleteDeployAttempts(ctx, []string{"old-failed"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	again, err := db.ListPrunableDeployAttempts(ctx, now.Add(-30*24*time.Hour), 1, 100)
	if err != nil {
		t.Fatalf("list again: %v", err)
	}
	for _, a := range again {
		if a.ID == "old-failed" {
			t.Fatal("deleted attempt still listed")
		}
	}
}

func TestJanitorQuarantineKeepsFirstSeen(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	first := time.Now().UTC().Add(-48 * time.Hour)
	if err := db.UpsertJanitorQuarantine(ctx, JanitorQuarantine{Kind: "volume", Name: "v1", SizeBytes: 1, FirstSeen: first}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertJanitorQuarantine(ctx, JanitorQuarantine{Kind: "volume", Name: "v1", SizeBytes: 9, FirstSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListJanitorQuarantine(ctx, "volume")
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %v, %v", got, err)
	}
	if got[0].SizeBytes != 9 || got[0].FirstSeen.Sub(first) > time.Second {
		t.Fatalf("got %+v, want size 9 and original first_seen", got[0])
	}
}
