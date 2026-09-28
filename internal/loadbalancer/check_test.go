package loadbalancer

import (
	"context"
	"testing"
	"time"
)

func TestCheckNow_ConfiguredHealth(t *testing.T) {
	now := time.Now()
	clock := now
	reg := testRegistry(HistoryConfig{Checks: 10, Transitions: 10, SeriesStep: time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour}, &clock)
	obs := Observation{Service: "web", ObservedAt: now, Config: Config{ActiveHealth: &ActiveHealth{Path: "/h", Timeout: "2s", ExpectStatus: 200}}, Upstreams: []UpstreamObservation{
		{Upstream: Upstream{ID: "web#0", Dial: "a:1"}, Running: true},
		{Upstream: Upstream{ID: "web#1", Dial: "b:1"}, Running: true},
		{Upstream: Upstream{ID: "web#2"}, Running: false, Note: "not created yet"},
	}}
	prober := fakeProber{results: map[string]ProbeResult{
		"a:1": {OK: true, StatusCode: 200, Latency: 9 * time.Millisecond, CheckedAt: now},
		"b:1": {StatusCode: 503, Latency: 4 * time.Millisecond, CheckedAt: now, Err: "status 503"},
	}}
	out := CheckNow(context.Background(), reg, obs, nil, prober)
	if out.Note != "" {
		t.Errorf("note = %q, want empty with active health configured", out.Note)
	}
	if r := out.Results[0]; !r.OK || r.LatencyMs != 9 || r.StatusCode != 200 {
		t.Errorf("result 0 = %+v", r)
	}
	if r := out.Results[1]; r.OK || r.Reason != "expected 200, got 503" {
		t.Errorf("result 1 = %+v", r)
	}
	if r := out.Results[2]; r.OK || r.Reason != "not created yet" {
		t.Errorf("result 2 = %+v", r)
	}
	h := reg.History(out.Status, 0)
	if len(h[0].Checks) != 1 || len(h[1].Checks) != 1 || len(h[2].Checks) != 0 {
		t.Errorf("checks recorded = %d,%d,%d", len(h[0].Checks), len(h[1].Checks), len(h[2].Checks))
	}
	if out.Status.Upstreams[1].State != StateUnhealthy {
		t.Errorf("registry status state = %s", out.Status.Upstreams[1].State)
	}
}

func TestCheckNow_NoActiveHealthProbesRoot(t *testing.T) {
	now := time.Now()
	clock := now
	reg := testRegistry(HistoryConfig{Checks: 10, Transitions: 10, SeriesStep: time.Second, SeriesWindow: time.Minute, MaxAge: time.Hour}, &clock)
	obs := Observation{Service: "web", ObservedAt: now, Upstreams: []UpstreamObservation{{Upstream: Upstream{ID: "web#0", Dial: "a:1"}, Running: true}}}
	var gotPath string
	p := recordingProber{path: &gotPath, res: ProbeResult{OK: true, StatusCode: 204, CheckedAt: now}}
	out := CheckNow(context.Background(), reg, obs, nil, p)
	if gotPath != "/" || out.Note == "" || !out.Results[0].OK {
		t.Errorf("path %q note %q results %+v", gotPath, out.Note, out.Results)
	}
}

type recordingProber struct {
	path *string
	res  ProbeResult
}

func (p recordingProber) Probe(_ context.Context, _, path string, _ *UpstreamTLS, _ time.Duration) ProbeResult {
	*p.path = path
	return p.res
}

func TestCheckGate(t *testing.T) {
	g := NewCheckGate(2 * time.Second)
	t0 := time.Now()
	if _, ok := g.Allow("web", t0); !ok {
		t.Fatal("first check must pass")
	}
	if wait, ok := g.Allow("web", t0.Add(500*time.Millisecond)); ok || wait != 1500*time.Millisecond {
		t.Errorf("second check = wait %v ok %v", wait, ok)
	}
	if _, ok := g.Allow("other", t0.Add(500*time.Millisecond)); !ok {
		t.Error("gate is per service")
	}
	if _, ok := g.Allow("web", t0.Add(2*time.Second)); !ok {
		t.Error("check after the interval must pass")
	}
}
