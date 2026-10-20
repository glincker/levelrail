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

type fakeMetrics struct {
	samples []telemetry.Sample
}

func (f fakeMetrics) QueryMany(_ context.Context, ids, metrics []string, from, to time.Time) ([]telemetry.Sample, error) {
	var out []telemetry.Sample
	for _, s := range f.samples {
		if s.Timestamp.Before(from) || s.Timestamp.After(to) {
			continue
		}
		for _, id := range ids {
			for _, m := range metrics {
				if s.ResourceID == id && s.Metric == m {
					out = append(out, s)
				}
			}
		}
	}
	return out, nil
}

type fakeBreakdowns struct{ routes []telemetry.RouteRow }

func (f fakeBreakdowns) QueryBreakdown(context.Context, string, time.Time, time.Time) (telemetry.Breakdown, error) {
	return telemetry.Breakdown{Routes: f.routes}, nil
}

type fakeConns int

func (f fakeConns) Open(context.Context, string, []int) int { return int(f) }

type fakeOps map[string]bool

func (f fakeOps) ServicesWithActiveDeploy(context.Context) (map[string]bool, error) { return f, nil }

func tickApp(t *testing.T, mutate func(s *Scheduler), reqAt time.Time, extra ...telemetry.Sample) (slept bool, s *Scheduler) {
	t.Helper()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	old := now.Add(-3 * time.Hour)
	sleep := &fakeSleep{rows: map[string]*store.AppSleep{"web": {ServiceName: "web", IdleMinutes: 30, Since: old}}}
	apps := &fakeApps{suspended: map[string]bool{"web": false}}
	traffic := fakeTraffic{last: map[string]time.Time{}}
	var samples []telemetry.Sample
	if !reqAt.IsZero() {
		traffic.last["service:web"] = reqAt
		samples = append(samples, telemetry.Sample{ResourceID: "service:web", Metric: telemetry.MetricHTTPRequests, Timestamp: reqAt, Value: 10})
	}
	samples = append(samples, extra...)
	s = &Scheduler{
		Sleep: sleep, Apps: apps, Traffic: traffic, Metrics: fakeMetrics{samples}, Breakdowns: fakeBreakdowns{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now }, ObservedFrom: old,
		Cfg: Config{NetworkFloorBytesPerMin: 4096, WakeCooldown: 5 * time.Minute},
	}
	if mutate != nil {
		mutate(s)
	}
	if err := s.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	return apps.suspended["web"], s
}

func TestTickIdleSignals(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	net := func(sec int, v float64) []telemetry.Sample {
		at := now.Add(-time.Duration(sec) * time.Second)
		return []telemetry.Sample{{ResourceID: "service:web", Metric: "network_rx_bytes", Timestamp: at, Value: v}}
	}
	tests := []struct {
		name      string
		mutate    func(*Scheduler)
		reqAt     time.Time
		extra     []telemetry.Sample
		wantSleep bool
	}{
		{name: "nothing seen", wantSleep: true},
		{name: "real request 5 minutes ago", reqAt: now.Add(-5 * time.Minute), wantSleep: false},
		{name: "only probe requests", reqAt: now.Add(-time.Minute), mutate: func(s *Scheduler) {
			s.Breakdowns = fakeBreakdowns{routes: []telemetry.RouteRow{{Route: "/healthz", Requests: 10}}}
		}, wantSleep: true},
		{name: "probes plus one real request", reqAt: now.Add(-time.Minute), mutate: func(s *Scheduler) {
			s.Breakdowns = fakeBreakdowns{routes: []telemetry.RouteRow{{Route: "/healthz", Requests: 9}}}
		}, wantSleep: false},
		{name: "network traffic above the floor", extra: append(net(120, 100), net(60, 5_000_000)...), wantSleep: false},
		{name: "network trickle below the floor", extra: append(net(120, 100), net(60, 500)...), wantSleep: true},
		{name: "open connection", mutate: func(s *Scheduler) { s.Conns = fakeConns(2) }, wantSleep: false},
		{name: "deploy in flight", mutate: func(s *Scheduler) { s.Ops = fakeOps{"web": true} }, wantSleep: false},
		{name: "other app deploying", mutate: func(s *Scheduler) { s.Ops = fakeOps{"other": true} }, wantSleep: true},
		{name: "control plane just restarted", mutate: func(s *Scheduler) { s.ObservedFrom = now.Add(-time.Minute) }, wantSleep: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			suspended, _ := tickApp(t, tc.mutate, tc.reqAt, tc.extra...)
			if suspended != tc.wantSleep {
				t.Fatalf("suspended = %v, want %v", suspended, tc.wantSleep)
			}
		})
	}
}

func TestTickSnapshotReportsWhenItWillSleep(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	_, s := tickApp(t, nil, now.Add(-10*time.Minute))
	snap, ok := s.Snapshot("web")
	if !ok || snap.Decision.Sleep || snap.LastRequest.IsZero() {
		t.Fatalf("snapshot = %+v ok=%v", snap, ok)
	}
	if want := now.Add(20 * time.Minute); !snap.Decision.WillSleepAt.Equal(want) {
		t.Errorf("will sleep at %v, want %v", snap.Decision.WillSleepAt, want)
	}
}

func TestObserveResetsAfterAGap(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s := &Scheduler{}
	if got := s.observe(now); !got.Equal(now) {
		t.Fatalf("first tick must start observing now, got %v", got)
	}
	if got := s.observe(now.Add(time.Minute)); !got.Equal(now) {
		t.Errorf("a normal tick must keep the window, got %v", got)
	}
	resumed := now.Add(time.Hour)
	if got := s.observe(resumed); !got.Equal(resumed) {
		t.Errorf("a long gap (host suspended) must restart the window, got %v", got)
	}
}
