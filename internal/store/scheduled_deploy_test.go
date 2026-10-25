package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestScheduledDeployLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)

	mk := func(app string, at time.Time) ScheduledDeploy {
		d, err := db.CreateScheduledDeploy(ctx, ScheduledDeploy{ServiceName: app, Kind: ScheduledDeployKindImage, Image: "img:v2", RunAt: at})
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		return d
	}
	a := mk("web", base)
	b := mk("web", base.Add(time.Hour))
	mk("api", base)

	got, err := db.GetScheduledDeploy(ctx, "web", a.ID)
	if err != nil || got.Status != ScheduledDeployPending || !got.RunAt.Equal(base) {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if _, err := db.GetScheduledDeploy(ctx, "api", a.ID); !errors.Is(err, ErrScheduledDeployNotFound) {
		t.Fatalf("cross-app get err = %v, want not found", err)
	}

	list, _ := db.ListScheduledDeploys(ctx, "web", nil, 0)
	if len(list) != 2 {
		t.Fatalf("list web = %d, want 2", len(list))
	}

	tests := []struct {
		name string
		now  time.Time
		want int
	}{
		{"before any", base.Add(-time.Minute), 0},
		{"first due", base.Add(time.Second), 2},
		{"both web due", base.Add(2 * time.Hour), 3},
	}
	for _, tc := range tests {
		due, err := db.ListDueScheduledDeploys(ctx, tc.now, tc.now.Add(-time.Hour))
		if err != nil || len(due) != tc.want {
			t.Errorf("%s: due = %d, %v; want %d", tc.name, len(due), err, tc.want)
		}
	}

	if err := db.ClaimScheduledDeploy(ctx, a.ID, base); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := db.ClaimScheduledDeploy(ctx, a.ID, base); !errors.Is(err, ErrScheduledDeployNotPending) {
		t.Fatalf("second claim err = %v, want not pending", err)
	}
	if err := db.FinishScheduledDeploy(ctx, a.ID, ScheduledDeployFired, "ok", "", base); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := db.CancelScheduledDeploy(ctx, "web", a.ID, base); !errors.Is(err, ErrScheduledDeployNotPending) {
		t.Fatalf("cancel fired err = %v", err)
	}
	c, err := db.CancelScheduledDeploy(ctx, "web", b.ID, base)
	if err != nil || c.Status != ScheduledDeployCanceled || c.FinishedAt == nil {
		t.Fatalf("cancel = %+v, %v", c, err)
	}
	fired, _ := db.ListScheduledDeploys(ctx, "web", []string{ScheduledDeployFired}, 0)
	if len(fired) != 1 || fired[0].Reason != "ok" {
		t.Fatalf("fired filter = %+v", fired)
	}
}
