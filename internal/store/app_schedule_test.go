package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSetAndGetAppSchedule(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetAppSchedule(ctx, "web", "0 3 * * *", "main", "UTC", true); err != nil {
		t.Fatalf("SetAppSchedule() error = %v", err)
	}

	got, err := db.GetAppSchedule(ctx, "web")
	if err != nil {
		t.Fatalf("GetAppSchedule() error = %v", err)
	}
	if got.Cron != "0 3 * * *" || got.Branch != "main" || got.Timezone != "UTC" || !got.Enabled {
		t.Errorf("GetAppSchedule() = %+v, want cron/branch/timezone/enabled to match what was set", got)
	}
	if got.NextFireAt != nil {
		t.Errorf("GetAppSchedule() NextFireAt = %v, want nil (unarmed until the scheduler's first tick)", got.NextFireAt)
	}
}

func TestGetAppSchedule_NotFound(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.GetAppSchedule(context.Background(), "missing"); !errors.Is(err, ErrAppScheduleNotFound) {
		t.Fatalf("GetAppSchedule() error = %v, want ErrAppScheduleNotFound", err)
	}
}

func TestSetAppSchedule_ResetsNextFireAt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetAppSchedule(ctx, "web", "0 3 * * *", "main", "UTC", true); err != nil {
		t.Fatalf("SetAppSchedule() error = %v", err)
	}
	if err := db.ArmAppScheduleNextRun(ctx, "web", time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("ArmAppScheduleNextRun() error = %v", err)
	}

	// Changing the cron must not carry over an armed time computed under
	// the old schedule.
	if err := db.SetAppSchedule(ctx, "web", "0 4 * * *", "main", "UTC", true); err != nil {
		t.Fatalf("SetAppSchedule() (update) error = %v", err)
	}
	got, err := db.GetAppSchedule(ctx, "web")
	if err != nil {
		t.Fatalf("GetAppSchedule() error = %v", err)
	}
	if got.Cron != "0 4 * * *" {
		t.Errorf("GetAppSchedule() Cron = %q, want the updated cron", got.Cron)
	}
	if got.NextFireAt != nil {
		t.Errorf("GetAppSchedule() NextFireAt = %v, want nil after an update reset it", got.NextFireAt)
	}
}

func TestListEnabledAppSchedules(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SetAppSchedule(ctx, "web", "0 3 * * *", "main", "UTC", true); err != nil {
		t.Fatalf("SetAppSchedule() error = %v", err)
	}
	if err := db.SetAppSchedule(ctx, "api", "0 4 * * *", "main", "UTC", false); err != nil {
		t.Fatalf("SetAppSchedule() error = %v", err)
	}

	got, err := db.ListEnabledAppSchedules(ctx)
	if err != nil {
		t.Fatalf("ListEnabledAppSchedules() error = %v", err)
	}
	if len(got) != 1 || got[0].ServiceName != "web" {
		t.Fatalf("ListEnabledAppSchedules() = %+v, want exactly [web]", got)
	}
}

func TestArmAppScheduleNextRun(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SetAppSchedule(ctx, "web", "0 3 * * *", "main", "UTC", true); err != nil {
		t.Fatalf("SetAppSchedule() error = %v", err)
	}

	next := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	if err := db.ArmAppScheduleNextRun(ctx, "web", next); err != nil {
		t.Fatalf("ArmAppScheduleNextRun() error = %v", err)
	}

	got, err := db.GetAppSchedule(ctx, "web")
	if err != nil {
		t.Fatalf("GetAppSchedule() error = %v", err)
	}
	if got.NextFireAt == nil || !got.NextFireAt.Equal(next) {
		t.Errorf("GetAppSchedule() NextFireAt = %v, want %v", got.NextFireAt, next)
	}
}

func TestRecordAndListAppScheduleHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		e := AppScheduleHistoryEntry{
			ServiceName:  "web",
			ScheduledFor: base.Add(time.Duration(i) * 24 * time.Hour),
			FiredAt:      base.Add(time.Duration(i)*24*time.Hour + time.Second),
			Status:       AppScheduleHistoryFired,
			Reason:       "deploy triggered",
		}
		if err := db.RecordAppScheduleHistory(ctx, e); err != nil {
			t.Fatalf("RecordAppScheduleHistory() error = %v", err)
		}
	}
	// A different app's history must never leak into "web"'s list.
	if err := db.RecordAppScheduleHistory(ctx, AppScheduleHistoryEntry{
		ServiceName: "api", ScheduledFor: base, FiredAt: base, Status: AppScheduleHistoryFired,
	}); err != nil {
		t.Fatalf("RecordAppScheduleHistory() error = %v", err)
	}

	got, err := db.ListAppScheduleHistory(ctx, "web", 20)
	if err != nil {
		t.Fatalf("ListAppScheduleHistory() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListAppScheduleHistory() returned %d entries, want 3", len(got))
	}
	if got[0].ID == "" {
		t.Error("ListAppScheduleHistory() entry has no minted ID")
	}
	// Newest first.
	if !got[0].FiredAt.After(got[1].FiredAt) || !got[1].FiredAt.After(got[2].FiredAt) {
		t.Errorf("ListAppScheduleHistory() not newest-first: %+v", got)
	}

	limited, err := db.ListAppScheduleHistory(ctx, "web", 2)
	if err != nil {
		t.Fatalf("ListAppScheduleHistory() (limited) error = %v", err)
	}
	if len(limited) != 2 {
		t.Fatalf("ListAppScheduleHistory() with limit=2 returned %d entries, want 2", len(limited))
	}
}
