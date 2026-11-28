package appsleep

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// stackApps is a fake app store holding a three-service stack and logging
// every suspend change in order.
type stackApps struct {
	svcs      map[string]*store.DesiredService
	suspended map[string]bool
	calls     []string
}

func newStackApps() *stackApps {
	mk := func(n string, deps ...string) *store.DesiredService {
		return &store.DesiredService{Name: "shop-" + n, AppID: "shop", DependsOn: deps, Domains: []string{n + ".example.com"}}
	}
	return &stackApps{
		svcs:      map[string]*store.DesiredService{"shop-web": mk("web", "api"), "shop-api": mk("api", "db"), "shop-db": mk("db")},
		suspended: map[string]bool{},
	}
}

func (f *stackApps) GetDesiredService(_ context.Context, n string) (*store.DesiredService, error) {
	s, ok := f.svcs[n]
	if !ok {
		return nil, store.ErrServiceNotFound
	}
	c := *s
	c.Suspended = f.suspended[n]
	return &c, nil
}

func (f *stackApps) UpdateServiceSuspended(_ context.Context, n string, s bool) error {
	f.suspended[n] = s
	if s {
		f.calls = append(f.calls, "stop "+n)
	} else {
		f.calls = append(f.calls, "start "+n)
	}
	return nil
}

func (f *stackApps) ListServicesByApp(context.Context, string) ([]store.DesiredService, error) {
	var out []store.DesiredService
	for _, s := range f.svcs {
		out = append(out, *s)
	}
	return out, nil
}

func stackSleepRows(since time.Time, sleeping bool) *fakeSleep {
	r := map[string]*store.AppSleep{}
	for _, n := range []string{"shop-web", "shop-api", "shop-db"} {
		r[n] = &store.AppSleep{ServiceName: n, IdleMinutes: 30, Since: since, Sleeping: sleeping}
	}
	return &fakeSleep{rows: r}
}

func TestStackSleepsAndWakesInOrder(t *testing.T) {
	t.Setenv(EnvStackSleep, "true")
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	apps, sleep := newStackApps(), stackSleepRows(old, false)
	s := &Scheduler{Sleep: sleep, Apps: apps, Traffic: fakeTraffic{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }, ObservedFrom: old}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	for n, r := range sleep.rows {
		if !r.Sleeping || !apps.suspended[n] {
			t.Fatalf("%s should be asleep and stopped", n)
		}
	}
	apps.calls = nil
	woke, err := Wake(context.Background(), sleep, apps, nil, "shop-web", now)
	if err != nil || !woke {
		t.Fatalf("woke=%v err=%v", woke, err)
	}
	want := []string{"start shop-db", "start shop-api", "start shop-web"}
	if len(apps.calls) != 3 || apps.calls[0] != want[0] || apps.calls[1] != want[1] || apps.calls[2] != want[2] {
		t.Fatalf("wake order = %v, want %v", apps.calls, want)
	}
	for n, r := range sleep.rows {
		if r.Sleeping {
			t.Errorf("%s still marked sleeping", n)
		}
	}
}

func TestStackTrafficOnOneServiceKeepsAllAwake(t *testing.T) {
	t.Setenv(EnvStackSleep, "true")
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	apps, sleep := newStackApps(), stackSleepRows(old, false)
	s := &Scheduler{Sleep: sleep, Apps: apps, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }, ObservedFrom: old,
		Traffic: fakeTraffic{last: map[string]time.Time{"service:shop-api": now.Add(-time.Minute)}}}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(apps.calls) != 0 {
		t.Fatalf("a busy member must keep the stack awake, got %v", apps.calls)
	}
}

func TestStackOffKeepsPerServiceSleep(t *testing.T) {
	now := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-2 * time.Hour)
	apps, sleep := newStackApps(), stackSleepRows(old, false)
	s := &Scheduler{Sleep: sleep, Apps: apps, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }, ObservedFrom: old,
		Traffic: fakeTraffic{last: map[string]time.Time{"service:shop-api": now.Add(-time.Minute)}}}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !apps.suspended["shop-web"] || apps.suspended["shop-api"] {
		t.Fatalf("with stack sleep off each service sleeps alone: %v", apps.suspended)
	}
}
