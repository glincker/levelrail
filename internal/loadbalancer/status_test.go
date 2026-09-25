package loadbalancer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeProber struct{ results map[string]ProbeResult }

func (f fakeProber) Probe(_ context.Context, dial, _ string, _ bool, _ time.Duration) ProbeResult {
	return f.results[dial]
}

func TestBuildStatus(t *testing.T) {
	now := time.Now()
	obs := Observation{
		Service: "web", ObservedAt: now, Ready: true, Reason: "Balancing",
		Config: Config{
			ActiveHealth:  &ActiveHealth{Path: "/h", Timeout: "1s"},
			PassiveHealth: &PassiveHealth{MaxFails: 2},
		},
		Upstreams: []UpstreamObservation{
			{Upstream: Upstream{ID: "web#0", Dial: "a:1", Replica: 0}, Running: true, Weight: 1},
			{Upstream: Upstream{ID: "web#1", Dial: "b:1", Replica: 1}, Running: false},
			{Upstream: Upstream{ID: "web#2", Dial: "c:1", Replica: 2}, Running: true},
			{Upstream: Upstream{ID: "web#3", Dial: "d:1", Replica: 3}, Running: true},
			{Upstream: Upstream{ID: "web#4", Dial: "e:1", Replica: 4}, Running: true, Draining: true, Note: "previous release"},
		},
	}
	stats := map[string]Stats{"a:1": {NumRequests: 4}, "c:1": {Fails: 2}}
	prober := fakeProber{results: map[string]ProbeResult{
		"a:1": {OK: true, StatusCode: 200, Latency: 12 * time.Millisecond, CheckedAt: now},
		"d:1": {OK: false, Err: "connection refused", CheckedAt: now},
		"e:1": {OK: true, StatusCode: 200, CheckedAt: now},
	}}

	got := BuildStatus(context.Background(), obs, stats, prober)
	want := []struct {
		state string
		conns int
	}{{StateHealthy, 4}, {StateUnhealthy, 0}, {StateUnhealthy, 0}, {StateUnhealthy, 0}, {StateDraining, 0}}
	for i, w := range want {
		if got.Upstreams[i].State != w.state || got.Upstreams[i].ActiveConns != w.conns {
			t.Errorf("upstream %d = %+v, want state %s conns %d", i, got.Upstreams[i], w.state, w.conns)
		}
	}
	if got.Upstreams[0].LatencyMs != 12 || got.Upstreams[3].Reason != "connection refused" || got.Upstreams[2].Reason == "" {
		t.Errorf("unexpected details: %+v", got.Upstreams)
	}
	if got.Algorithm != AlgoRoundRobin || !got.Ready {
		t.Errorf("status header = %+v", got)
	}
}

func TestRegistryFirstSeenAndRetain(t *testing.T) {
	r := NewRegistry()
	t0 := time.Unix(100, 0)
	if first := r.FirstSeen("web", []string{"web#0", "web#1"}, t0); !first["web#0"].IsZero() {
		t.Fatalf("first call must treat upstreams as long-running, got %v", first)
	}
	r.FirstSeen("web", []string{"web#1"}, t0)
	got := r.FirstSeen("web", []string{"web#1", "web#2"}, t0.Add(time.Minute))
	if !got["web#1"].IsZero() || !got["web#2"].Equal(t0.Add(time.Minute)) || len(got) != 2 {
		t.Fatalf("FirstSeen = %v", got)
	}
	r.Record(Observation{Service: "web"})
	r.Record(Observation{Service: "gone"})
	r.Retain(map[string]bool{"web": true})
	if _, ok := r.Get("gone"); ok {
		t.Error("gone must be dropped")
	}
	if len(r.List()) != 1 {
		t.Errorf("List = %v", r.List())
	}
}

func TestCaddyAdminStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reverse_proxy/upstreams" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"address":"a:1","num_requests":3,"fails":1}]`))
	}))
	defer srv.Close()

	c := CaddyAdminStats{Addr: strings.TrimPrefix(srv.URL, "http://")}
	got, err := c.UpstreamStats(context.Background())
	if err != nil || got["a:1"] != (Stats{NumRequests: 3, Fails: 1}) {
		t.Fatalf("UpstreamStats = %v, %v", got, err)
	}

	bad := CaddyAdminStats{Addr: "127.0.0.1:1"}
	if _, err := bad.UpstreamStats(context.Background()); err == nil {
		t.Fatal("unreachable admin must error")
	}
}

func TestHTTPProber(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")

	if res := (HTTPProber{}).Probe(context.Background(), addr, "/ok", false, time.Second); !res.OK || res.StatusCode != 200 {
		t.Errorf("ok probe = %+v", res)
	}
	if res := (HTTPProber{}).Probe(context.Background(), addr, "/bad", false, time.Second); res.OK || res.Err == "" {
		t.Errorf("bad probe = %+v", res)
	}
	if res := (HTTPProber{}).Probe(context.Background(), "127.0.0.1:1", "/", false, 200*time.Millisecond); res.OK || res.Err == "" {
		t.Errorf("refused probe = %+v", res)
	}
}
