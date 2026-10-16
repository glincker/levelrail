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

func TestAppSleepOptionsAndWakeError(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	t0 := time.Date(2026, 10, 9, 10, 0, 0, 0, time.UTC)

	if a, _ := db.GetAppSleep(ctx, "web"); !a.PortWake {
		t.Error("an app with no row must default to port wake on")
	}
	if err := db.SetAppSleepIdle(ctx, "web", 30, t0); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.GetAppSleep(ctx, "web"); !a.PortWake || len(a.IgnorePaths) != 0 {
		t.Fatalf("defaults = %+v", a)
	}
	off := false
	if err := db.SetAppSleepOptions(ctx, "web", []string{"/up", "/metrics"}, &off, t0); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepWakeError(ctx, "web", "not ready", t0); err != nil {
		t.Fatal(err)
	}
	a, _ := db.GetAppSleep(ctx, "web")
	if a.PortWake || len(a.IgnorePaths) != 2 || a.WakeError != "not ready" {
		t.Fatalf("stored = %+v", a)
	}
	rows, _ := db.ListAppSleep(ctx)
	if len(rows) != 1 || rows[0].ServiceName != "web" || rows[0].WakeError != "not ready" || rows[0].PortWake {
		t.Fatalf("list = %+v", rows)
	}
	if err := db.SetAppSleeping(ctx, "web", true, t0); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.GetAppSleep(ctx, "web"); a.WakeError == "" {
		t.Error("going to sleep must not clear the last wake error")
	}
	if err := db.SetAppSleeping(ctx, "web", false, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if a, _ := db.GetAppSleep(ctx, "web"); a.WakeError != "" {
		t.Errorf("a wake must clear the error, got %q", a.WakeError)
	}
}

func TestServicesWithActiveDeploy(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	for i, st := range []string{DeployAttemptStatusRunning, DeployAttemptStatusSucceeded, DeployAttemptStatusQueued} {
		name := []string{"building", "done", "waiting"}[i]
		a := DeployAttempt{ID: "dep_" + name, ServiceName: name, Image: "x:1", Source: "api", Status: st, StartedAt: time.Now()}
		if err := db.SaveDeployAttempt(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ServicesWithActiveDeploy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got["building"] || !got["waiting"] || got["done"] || len(got) != 2 {
		t.Errorf("active = %v", got)
	}
}
