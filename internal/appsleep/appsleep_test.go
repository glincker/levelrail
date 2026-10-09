package appsleep

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

type fakeSleep struct{ rows map[string]*store.AppSleep }

func (f *fakeSleep) ListAppSleep(context.Context) ([]store.AppSleep, error) {
	var out []store.AppSleep
	for _, r := range f.rows {
		out = append(out, *r)
	}
	return out, nil
}
func (f *fakeSleep) SetAppSleeping(_ context.Context, n string, s bool, now time.Time) error {
	f.rows[n].Sleeping, f.rows[n].Since = s, now
	return nil
}
func (f *fakeSleep) GetAppSleep(_ context.Context, n string) (store.AppSleep, error) {
	if r, ok := f.rows[n]; ok {
		return *r, nil
	}
	return store.AppSleep{}, nil
}

type fakeApps struct{ suspended map[string]bool }

func (f *fakeApps) GetDesiredService(_ context.Context, n string) (*store.DesiredService, error) {
	s, ok := f.suspended[n]
	if !ok {
		return nil, store.ErrServiceNotFound
	}
	return &store.DesiredService{Name: n, Suspended: s}, nil
}
func (f *fakeApps) UpdateServiceSuspended(_ context.Context, n string, s bool) error {
	f.suspended[n] = s
	return nil
}

type fakeTraffic struct{ last map[string]time.Time }

func (f fakeTraffic) LatestByMetric(context.Context, string) ([]telemetry.Sample, error) {
	var out []telemetry.Sample
	for r, t := range f.last {
		out = append(out, telemetry.Sample{ResourceID: r, Timestamp: t})
	}
	return out, nil
}

func TestTick(t *testing.T) {
	now := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	sleep := &fakeSleep{rows: map[string]*store.AppSleep{
		"idle":      {ServiceName: "idle", IdleMinutes: 30, Since: old},
		"busy":      {ServiceName: "busy", IdleMinutes: 30, Since: old},
		"fresh":     {ServiceName: "fresh", IdleMinutes: 30, Since: now.Add(-5 * time.Minute)},
		"stopped":   {ServiceName: "stopped", IdleMinutes: 30, Since: old},
		"restarted": {ServiceName: "restarted", IdleMinutes: 30, Sleeping: true, Since: old},
		"gone":      {ServiceName: "gone", IdleMinutes: 30, Since: old},
	}}
	apps := &fakeApps{suspended: map[string]bool{"idle": false, "busy": false, "fresh": false, "stopped": true, "restarted": false}}
	s := &Scheduler{
		Sleep: sleep, Apps: apps, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now },
		Traffic: fakeTraffic{last: map[string]time.Time{"service:busy": now.Add(-time.Minute)}},
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !apps.suspended["idle"] || !sleep.rows["idle"].Sleeping {
		t.Error("idle app was not put to sleep")
	}
	for _, n := range []string{"busy", "fresh"} {
		if apps.suspended[n] || sleep.rows[n].Sleeping {
			t.Errorf("%s must keep running", n)
		}
	}
	if sleep.rows["stopped"].Sleeping {
		t.Error("an app stopped by hand must not be marked sleeping")
	}
	if sleep.rows["restarted"].Sleeping {
		t.Error("an app started by hand must clear its sleeping flag")
	}
}

func TestWake(t *testing.T) {
	now := time.Now()
	sleep := &fakeSleep{rows: map[string]*store.AppSleep{"web": {ServiceName: "web", IdleMinutes: 30, Sleeping: true}}}
	apps := &fakeApps{suspended: map[string]bool{"web": true}}
	woke, err := Wake(context.Background(), sleep, apps, nil, "web", now)
	if err != nil || !woke || apps.suspended["web"] || sleep.rows["web"].Sleeping {
		t.Fatalf("woke=%v err=%v apps=%v", woke, err, apps.suspended)
	}
	if woke, _ := Wake(context.Background(), sleep, apps, nil, "web", now); woke {
		t.Error("waking an awake app must be a no-op")
	}
}
