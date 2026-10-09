package store

import (
	"testing"
	"time"
)

func TestAppSleep(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	t0 := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

	if a, err := db.GetAppSleep(ctx, "web"); err != nil || a.IdleMinutes != 0 || a.Sleeping {
		t.Fatalf("missing row = %+v, %v", a, err)
	}
	if err := db.SetAppSleeping(ctx, "web", true, t0); err == nil {
		t.Fatal("sleeping an app with no setting must fail")
	}
	if err := db.SetAppSleepIdle(ctx, "web", 30, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleeping(ctx, "web", true, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	a, err := db.GetAppSleep(ctx, "web")
	if err != nil || !a.Sleeping || a.IdleMinutes != 30 || !a.Since.Equal(t0.Add(time.Hour)) {
		t.Fatalf("web = %+v, %v", a, err)
	}
	if err := db.SetAppSleepHold(ctx, "web", true, t0); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.GetAppSleep(ctx, "web"); !a.HoldRequests || !a.Sleeping {
		t.Fatalf("hold = %+v, want hold on and sleeping kept", a)
	}
	if err := db.SetAppSleepHold(ctx, "ghost", true, t0); err == nil {
		t.Fatal("hold on an app with no sleep setting must fail")
	}
	if err := db.SetAppSleepIdle(ctx, "off", 0, t0); err != nil {
		t.Fatal(err)
	}
	list, err := db.ListAppSleep(ctx)
	if err != nil || len(list) != 1 || list[0].ServiceName != "web" {
		t.Fatalf("list = %+v, %v; disabled apps must be omitted", list, err)
	}
}
