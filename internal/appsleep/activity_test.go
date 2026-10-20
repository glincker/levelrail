package appsleep

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	long := now.Add(-3 * time.Hour)
	tests := []struct {
		name       string
		in         DecideInput
		wantSleep  bool
		wantReason string
	}{
		{"no signals, window elapsed", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long}, true, ReasonIdle},
		{"recent request", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{LastRequest: now.Add(-5 * time.Minute)}}, false, ReasonRecentUse},
		{"old request outside the window", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{LastRequest: now.Add(-31 * time.Minute)}}, true, ReasonIdle},
		{"network activity only", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{LastNetwork: now.Add(-2 * time.Minute)}}, false, ReasonRecentUse},
		{"open connection with no recent traffic", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{OpenConnections: 1}}, false, ReasonConnections},
		{"deploy in flight", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{OperationInFlight: true}}, false, ReasonOperation},
		{"cooldown after a wake", DecideInput{IdleMinutes: 5, Since: now.Add(-6 * time.Minute), ObservedFrom: long, Cooldown: 10 * time.Minute}, false, ReasonCooldown},
		{"cooldown elapsed", DecideInput{IdleMinutes: 5, Since: now.Add(-11 * time.Minute), ObservedFrom: long, Cooldown: 10 * time.Minute}, true, ReasonIdle},
		{"request timestamp in the future (clock skew)", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: long, Signals: Signals{LastRequest: now.Add(2 * time.Hour)}}, false, ReasonRecentUse},
		{"since in the future (clock moved back)", DecideInput{IdleMinutes: 30, Since: now.Add(time.Hour), ObservedFrom: long}, false, ReasonRecentUse},
		{"control plane was down while the window elapsed", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: now.Add(-time.Minute)}, false, ReasonUnobserved},
		{"observed for the whole window", DecideInput{IdleMinutes: 30, Since: long, ObservedFrom: now.Add(-31 * time.Minute)}, true, ReasonIdle},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now = now
			d := Decide(tc.in)
			if d.Sleep != tc.wantSleep || d.Reason != tc.wantReason {
				t.Fatalf("Decide = sleep %v reason %q, want %v %q", d.Sleep, d.Reason, tc.wantSleep, tc.wantReason)
			}
			if !d.Sleep && d.Reason == ReasonRecentUse && d.WillSleepAt.IsZero() {
				t.Error("recent use must report when the app will sleep")
			}
		})
	}
}

func TestNetworkActivity(t *testing.T) {
	t0 := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	const floor = 4096
	tests := []struct {
		name string
		in   []NetSample
		want time.Time
	}{
		{"keepalive trickle stays under the floor", []NetSample{{at(0), 1000}, {at(15), 1200}, {at(30), 1400}}, time.Time{}},
		{"real traffic", []NetSample{{at(0), 1000}, {at(15), 1200}, {at(30), 900000}}, at(30)},
		{"counter reset after container restart is not activity", []NetSample{{at(0), 9e9}, {at(15), 100}}, time.Time{}},
		{"large gap is skipped", []NetSample{{at(0), 0}, {at(3600), 9e9}}, time.Time{}},
		{"single sample", []NetSample{{at(0), 5}}, time.Time{}},
	}
	for _, tc := range tests {
		if got := NetworkActivity(tc.in, floor, 5*time.Minute); !got.Equal(tc.want) {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRealRequestsIgnoresProbes(t *testing.T) {
	routes := []telemetry.RouteRow{{Route: "/healthz", Requests: 90}, {Route: "/api/:id", Requests: 3}}
	ignore := []string{"/healthz"}
	if got := RealRequests(93, routes, ignore); got != 3 {
		t.Errorf("real = %v, want 3", got)
	}
	if got := RealRequests(90, routes[:1], ignore); got != 0 {
		t.Errorf("only probes = %v, want 0", got)
	}
	if got := RealRequests(50, nil, ignore); got != 50 {
		t.Errorf("no breakdown must count everything, got %v", got)
	}
}

func TestEffectiveIgnorePaths(t *testing.T) {
	svc := &store.DesiredService{Health: &store.ServiceHealth{Readiness: &store.ServiceProbe{Path: "/api/ready"}, Liveness: &store.ServiceProbe{TCP: true}}}
	got := map[string]bool{}
	for _, p := range EffectiveIgnorePaths(svc, []string{"metrics"}) {
		got[p] = true
	}
	for _, want := range []string{"/healthz", "/health", "/api/ready", "/metrics"} {
		if !got[want] {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}

func TestConfigFromEnv(t *testing.T) {
	env := map[string]string{EnvNetworkFloor: "100", EnvWakeCooldown: "90s", EnvPortWakeTimeout: "bogus"}
	c := ConfigFromEnv(func(k string) string { return env[k] })
	if c.NetworkFloorBytesPerMin != 100 || c.WakeCooldown != 90*time.Second || c.PortWakeTimeout != DefaultPortWakeTimeout || c.PortWakeMinPort != DefaultPortWakeMinPort {
		t.Errorf("config = %+v", c)
	}
}
