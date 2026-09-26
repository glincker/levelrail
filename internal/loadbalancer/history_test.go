package loadbalancer

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func testRegistry(cfg HistoryConfig, clock *time.Time) *Registry {
	r := NewRegistryWithHistory(cfg)
	r.now = func() time.Time { return *clock }
	return r
}

func TestProbeReason(t *testing.T) {
	tests := []struct {
		name string
		res  ProbeResult
		want int
		out  string
	}{
		{"ok", ProbeResult{OK: true, StatusCode: 200}, 0, ""},
		{"timeout", ProbeResult{Err: `Get "http://a": context deadline exceeded (Client.Timeout exceeded)`}, 0, "timeout after 2s"},
		{"refused", ProbeResult{Err: "dial tcp 127.0.0.1:1: connect: connection refused"}, 0, "connection refused"},
		{"dns", ProbeResult{Err: "lookup x: no such host"}, 0, "host not found"},
		{"tls", ProbeResult{Err: "x509: certificate signed by unknown authority"}, 0, "TLS handshake failed"},
		{"wrong code", ProbeResult{StatusCode: 503, Err: "status 503"}, 200, "expected 200, got 503"},
		{"default range", ProbeResult{StatusCode: 500}, 0, "expected 2xx or 3xx, got 500"},
		{"exact code ok", ProbeResult{OK: true, StatusCode: 204}, 204, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := probeReason(tt.res, tt.want, 2*time.Second); got != tt.out {
				t.Errorf("probeReason = %q, want %q", got, tt.out)
			}
		})
	}
}

func statusFor(state, reason string, conns int, probed *CheckRecord) Status {
	return Status{Service: "web", Upstreams: []UpstreamStatus{{ID: "web#0", Dial: "a:1", State: state, Reason: reason, ActiveConns: conns, AdminState: AdminActive, probed: probed}}}
}

func TestRegistryRecordStatus_TransitionsAndChecks(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := testRegistry(HistoryConfig{Checks: 3, Transitions: 2, SeriesStep: 15 * time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour}, &clock)

	steps := []struct {
		state, reason string
	}{
		{StateHealthy, ""},
		{StateUnhealthy, "timeout after 2s"},
		{StateUnhealthy, "timeout after 2s"},
		{StateHealthy, ""},
		{StateDraining, "draining, no new connections"},
	}
	var last Status
	for i, s := range steps {
		clock = clock.Add(20 * time.Second)
		last = statusFor(s.state, s.reason, i, &CheckRecord{At: clock, OK: s.state == StateHealthy, Reason: s.reason})
		r.RecordStatus(&last)
	}
	h := r.History(last, 0)[0]
	if len(h.Transitions) != 2 {
		t.Fatalf("transitions bounded to 2, got %d: %+v", len(h.Transitions), h.Transitions)
	}
	if h.Transitions[0].From != StateUnhealthy || h.Transitions[0].To != StateHealthy || h.Transitions[0].Reason != "checks passing" {
		t.Errorf("transition[0] = %+v", h.Transitions[0])
	}
	if h.Transitions[1].To != StateDraining || h.Transitions[1].Reason != "draining, no new connections" {
		t.Errorf("transition[1] = %+v", h.Transitions[1])
	}
	if len(h.Checks) != 3 {
		t.Fatalf("checks ring bounded to 3, got %d", len(h.Checks))
	}
	if !h.Checks[2].At.After(h.Checks[0].At) {
		t.Errorf("checks must be newest last: %+v", h.Checks)
	}
	if got := r.History(last, 2)[0].Checks; len(got) != 2 {
		t.Errorf("limit 2 returned %d checks", len(got))
	}
	if last.Upstreams[0].LastChangedAt == nil || !last.Upstreams[0].LastChangedAt.Equal(clock) {
		t.Errorf("last_changed_at = %v, want %v", last.Upstreams[0].LastChangedAt, clock)
	}
}

func TestRegistryRecordStatus_SeriesBoundsAndStep(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := testRegistry(HistoryConfig{Checks: 5, Transitions: 5, SeriesStep: 15 * time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour}, &clock)
	var last Status
	for i := 0; i < 100; i++ {
		clock = clock.Add(5 * time.Second)
		last = statusFor(StateHealthy, "", i, nil)
		r.RecordStatus(&last)
	}
	s := r.History(last, 0)[0].Series
	if maxPoints := 5; len(s.Connections) > maxPoints {
		t.Fatalf("series must hold at most window/step+1 = %d points, got %d", maxPoints, len(s.Connections))
	}
	if len(s.Connections) < 2 || s.Connections[1].At.Sub(s.Connections[0].At) < 15*time.Second {
		t.Errorf("samples closer than the step: %+v", s.Connections)
	}
	if len(s.LatencyMs) != 0 {
		t.Errorf("latency needs a probe, got %d points", len(s.LatencyMs))
	}
	if len(s.Fails) != len(s.Connections) {
		t.Errorf("fails and connections share sample times")
	}
}

func TestRegistryRecordStatus_DropsGoneUpstreamsAndRetain(t *testing.T) {
	clock := time.Now()
	r := testRegistry(HistoryConfig{Checks: 5, Transitions: 5, SeriesStep: time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour}, &clock)
	st := statusFor(StateHealthy, "", 1, nil)
	r.RecordStatus(&st)
	empty := Status{Service: "web"}
	r.RecordStatus(&empty)
	if len(r.hist["web"]) != 0 {
		t.Errorf("gone upstream kept history: %v", r.hist["web"])
	}
	r.RecordStatus(&st)
	r.Retain(map[string]bool{})
	if len(r.hist) != 0 {
		t.Errorf("Retain must drop history for removed services")
	}
}

func TestRegistryRecordStatus_MaxAgePrunes(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := testRegistry(HistoryConfig{Checks: 50, Transitions: 50, SeriesStep: time.Second, SeriesWindow: time.Hour, MaxAge: time.Minute}, &clock)
	st := statusFor(StateHealthy, "", 0, &CheckRecord{At: clock, OK: true})
	r.RecordStatus(&st)
	clock = clock.Add(10 * time.Minute)
	st = statusFor(StateUnhealthy, "connection refused", 0, &CheckRecord{At: clock, Reason: "connection refused"})
	r.RecordStatus(&st)
	h := r.History(st, 0)[0]
	if len(h.Checks) != 1 {
		t.Errorf("old check should be pruned by age, got %d", len(h.Checks))
	}
}

func TestHistoryConfigFromEnv(t *testing.T) {
	t.Setenv("APP_LB_HISTORY_CHECKS", "7")
	t.Setenv("APP_LB_HISTORY_STEP", "bogus")
	cfg := HistoryConfigFromEnv()
	if cfg.Checks != 7 || cfg.SeriesStep != 15*time.Second || cfg.SeriesWindow != 30*time.Minute {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestBuildStatus_AdminStates(t *testing.T) {
	now := time.Now()
	obs := Observation{Service: "web", ObservedAt: now, Config: Config{ActiveHealth: &ActiveHealth{Path: "/h", Timeout: "1s"}}, Upstreams: []UpstreamObservation{
		{Upstream: Upstream{ID: "web#0", Dial: "a:1"}, Running: true, AdminState: AdminDisabled},
		{Upstream: Upstream{ID: "web#1", Dial: "b:1"}, Running: true, AdminState: AdminDraining},
		{Upstream: Upstream{ID: "web#2", Dial: "c:1"}, Running: true},
	}}
	got := BuildStatus(context.Background(), obs, nil, fakeProber{results: map[string]ProbeResult{"c:1": {OK: true, StatusCode: 200, CheckedAt: now}}})
	want := []struct{ state, admin, reason string }{
		{StateDisabled, AdminDisabled, "disabled by operator"},
		{StateDraining, AdminDraining, "draining, no new connections"},
		{StateHealthy, AdminActive, ""},
	}
	for i, w := range want {
		u := got.Upstreams[i]
		if u.State != w.state || u.AdminState != w.admin || u.Reason != w.reason {
			t.Errorf("upstream %d = %+v, want %+v", i, u, w)
		}
	}
	if !(UpstreamObservation{Running: true}).InPool() || (UpstreamObservation{Running: true, AdminState: AdminDraining}).InPool() || (UpstreamObservation{AdminState: AdminActive}).InPool() {
		t.Error("InPool must require running and active")
	}
}

func TestParseUpstreamReplica(t *testing.T) {
	tests := []struct {
		id string
		n  int
		ok bool
	}{{"web#2", 2, true}, {"web#x", 0, false}, {"other#1", 0, false}, {"web#-1", 0, false}, {"web", 0, false}}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.id), func(t *testing.T) {
			n, ok := ParseUpstreamReplica("web", tt.id)
			if n != tt.n || ok != tt.ok {
				t.Errorf("got %d,%v want %d,%v", n, ok, tt.n, tt.ok)
			}
		})
	}
}
