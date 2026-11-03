package scheduledeploy

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestScheduler_Tick_ImpossibleDateNeverArmsOrFires(t *testing.T) {
	fakeStore := newFakeScheduleStore(store.AppSchedule{ServiceName: "web", Cron: "0 0 30 2 *", Branch: "main", Timezone: "UTC", Enabled: true})
	trigger := &fakeTrigger{}
	s := NewScheduler(fakeStore, nil, trigger, nil)
	s.Now = func() time.Time { return time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC) }
	for range 3 {
		if err := s.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error = %v", err)
		}
	}
	if trigger.count() != 0 || fakeStore.schedules["web"].NextFireAt != nil {
		t.Fatalf("impossible date fired %d times or armed", trigger.count())
	}
}

func TestScheduler_Tick_LeapDayLastRunDoesNotRefireEveryTick(t *testing.T) {
	due := time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC)
	sch := store.AppSchedule{ServiceName: "web", Cron: "0 0 29 2 *", Branch: "main", Timezone: "UTC", Enabled: true, NextFireAt: &due}
	fakeStore := newFakeScheduleStore(sch)
	trigger := &fakeTrigger{}
	s := NewScheduler(fakeStore, nil, trigger, nil)
	s.Now = func() time.Time { return due.Add(30 * time.Second) }
	for range 3 {
		if err := s.Tick(context.Background()); err != nil {
			t.Fatalf("Tick() error = %v", err)
		}
	}
	if trigger.count() > 1 {
		t.Fatalf("fired %d times, want at most 1", trigger.count())
	}
}
