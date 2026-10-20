package store

import (
	"context"
	"errors"
	"testing"
)

func sampleSchedule(id string) UpgradeSchedule {
	return UpgradeSchedule{
		ID: id, TargetVersion: "v1.2.0", Source: UpgradeSourceManual, Mode: UpgradeModeAt,
		RunAt: "2026-10-12T03:00:00Z", RetryUntil: "2026-10-12T05:00:00Z", Timezone: "UTC",
		CreatedBy: "root", CreatedAt: "2026-10-11T10:00:00Z",
	}
}

func TestUpgradeScheduleSingleActiveAndCompareAndSet(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.InsertUpgradeSchedule(ctx, sampleSchedule("us_1")); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertUpgradeSchedule(ctx, sampleSchedule("us_2")); !errors.Is(err, ErrUpgradeScheduleActive) {
		t.Fatalf("second active schedule must be refused, got %v", err)
	}

	active, err := db.ActiveUpgradeSchedule(ctx)
	if err != nil || active == nil || active.ID != "us_1" || active.State != UpgradeSchedulePending {
		t.Fatalf("active = %+v, %v", active, err)
	}

	running := *active
	running.State, running.FiredAt = UpgradeScheduleRunning, "2026-10-12T03:00:01Z"
	if ok, err := db.UpdateUpgradeSchedule(ctx, running, UpgradeSchedulePending, UpgradeScheduleBlocked); err != nil || !ok {
		t.Fatalf("fire transition: ok=%v err=%v", ok, err)
	}
	cancelled := *active
	cancelled.State = UpgradeScheduleCancelled
	if ok, err := db.UpdateUpgradeSchedule(ctx, cancelled, UpgradeSchedulePending, UpgradeScheduleBlocked); err != nil || ok {
		t.Fatalf("a cancel racing a fire must lose, ok=%v err=%v", ok, err)
	}

	done := running
	done.State, done.FinishedAt = UpgradeScheduleApplied, "2026-10-12T03:05:00Z"
	if ok, err := db.UpdateUpgradeSchedule(ctx, done, UpgradeScheduleRunning); err != nil || !ok {
		t.Fatalf("finish: ok=%v err=%v", ok, err)
	}
	if got, _ := db.ActiveUpgradeSchedule(ctx); got != nil {
		t.Fatalf("no active schedule expected, got %+v", got)
	}
	if err := db.InsertUpgradeSchedule(ctx, sampleSchedule("us_3")); err != nil {
		t.Fatalf("a finished schedule must not block a new one: %v", err)
	}
	list, err := db.ListUpgradeSchedules(ctx, 10)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
}

func TestUpgradePolicyDefaultsAndUpdate(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	p, err := db.GetUpgradePolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != "off" || p.Paused || p.MinReleaseAgeDays != 3 || p.WindowTimezone != "UTC" {
		t.Fatalf("unexpected defaults: %+v", p)
	}
	p.Mode, p.WindowCron, p.Paused, p.NotifyChannelIDs = "patch", "0 3 * * *", true, `["ch_1"]`
	if err := db.UpdateUpgradePolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := db.SetUpgradeDecision(ctx, "v1.2.1", "held", "2026-10-11T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetUpgradePolicy(ctx)
	if got.Mode != "patch" || !got.Paused || got.DecisionTag != "v1.2.1" || got.NotifyChannelIDs != `["ch_1"]` {
		t.Fatalf("round trip lost fields: %+v", got)
	}
}
