package monitor

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

func testConfig() Config {
	cfg, err := ConfigFromEnv(func(string) (string, bool) { return "", false })
	if err != nil {
		panic(err)
	}
	return cfg
}

func httpMon(url string) Monitor {
	return Normalize(Monitor{ID: "m1", Name: "t", Kind: KindHTTP, Interval: time.Minute, Timeout: 500 * time.Millisecond, Enabled: true,
		Spec: Spec{URL: url}})
}

func TestSchedulerFiresAtIntervalAndRespectsFloor(t *testing.T) {
	cfg := testConfig()
	s := NewScheduler(cfg, func(time.Duration) time.Duration { return 0 })
	t0 := time.Unix(1000, 0)
	m := Monitor{ID: "a", Kind: KindHTTP, Interval: 5 * time.Second, Enabled: true}
	s.Add(m, t0)
	if got := s.Due(t0); len(got) != 1 {
		t.Fatalf("first probe should run at start, got %d", len(got))
	}
	s.Done("a")
	if got := s.Due(t0.Add(cfg.MinIntervalHTTP - time.Second)); len(got) != 0 {
		t.Fatalf("fired below the floor: %v", got)
	}
	if got := s.Due(t0.Add(cfg.MinIntervalHTTP)); len(got) != 1 {
		t.Fatalf("want 1 due at the floor, got %d", len(got))
	}
	if got := s.Due(t0.Add(2 * cfg.MinIntervalHTTP)); len(got) != 0 {
		t.Fatal("a probe still in flight must not be dispatched twice")
	}
	s.Done("a")
	if got := s.Due(t0.Add(3 * cfg.MinIntervalHTTP)); len(got) != 1 {
		t.Fatalf("want 1 due after Done, got %d", len(got))
	}
	s.Remove("a")
	if s.Len() != 0 {
		t.Fatal("remove left an entry")
	}
}

func TestValidateNamesTheEnvVar(t *testing.T) {
	cfg := testConfig()
	m := httpMon("https://example.com")
	m.Interval = time.Second
	err := Validate(m, cfg)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "APP_MONITOR_MIN_INTERVAL_HTTP") {
		t.Fatalf("want floor error naming the env var, got %v", err)
	}
	m = httpMon("https://example.com")
	m.Timeout = cfg.MaxTimeout + time.Second
	if err := Validate(m, cfg); err == nil || !strings.Contains(err.Error(), "APP_MONITOR_MAX_TIMEOUT") {
		t.Fatalf("want timeout cap error, got %v", err)
	}
}

func TestHTTPCheck(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("hello healthy world")) })
	mux.HandleFunc("/boom", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	mux.HandleFunc("/redir", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok", http.StatusFound) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, _ *http.Request) { <-release })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	defer close(release)

	cfg := testConfig()
	cfg.MaxRedirects = 2
	c := NewChecker(cfg)
	ctx := context.Background()
	cases := []struct {
		name   string
		mutate func(*Monitor)
		path   string
		ok     bool
		reason string
	}{
		{"status match", nil, "/ok", true, ""},
		{"status mismatch", nil, "/boom", false, ReasonStatus},
		{"explicit range", func(m *Monitor) { m.Spec.Expect = []StatusRange{{500, 500}} }, "/boom", true, ""},
		{"keyword found", func(m *Monitor) { m.Spec.Contains = "healthy" }, "/ok", true, ""},
		{"keyword missing", func(m *Monitor) { m.Spec.Contains = "absent" }, "/ok", false, ReasonKeyword},
		{"redirect not followed counts as 3xx", nil, "/redir", true, ""},
		{"redirect followed", func(m *Monitor) { m.Spec.FollowRedirects = true; m.Spec.Expect = []StatusRange{{200, 200}} }, "/redir", true, ""},
		{"redirect loop hits the cap", func(m *Monitor) { m.Spec.FollowRedirects = true }, "/loop", false, ReasonRedirects},
		{"timeout", func(m *Monitor) { m.Timeout = 100 * time.Millisecond }, "/slow", false, ReasonTimeout},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := httpMon(srv.URL + tc.path)
			if tc.mutate != nil {
				tc.mutate(&m)
			}
			got := c.Check(ctx, m, map[string]string{"X-Probe": "1"})
			if got.OK != tc.ok || got.Reason != tc.reason {
				t.Fatalf("got ok=%v reason=%q code=%d, want ok=%v reason=%q", got.OK, got.Reason, got.Code, tc.ok, tc.reason)
			}
		})
	}
}

func TestTCPCheck(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	m := Monitor{ID: "t", Kind: KindTCP, Timeout: time.Second, Spec: Spec{Host: "127.0.0.1", Port: port}}
	c := NewChecker(testConfig())
	if r := c.Check(context.Background(), m, nil); !r.OK {
		t.Fatalf("listening port should be up, got %q", r.Reason)
	}
	_ = ln.Close()
	if r := c.Check(context.Background(), m, nil); r.OK || r.Reason != ReasonConnect {
		t.Fatalf("closed port should fail to connect, got ok=%v reason=%q", r.OK, r.Reason)
	}
}

func TestPrivateAddressRefused(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "false")
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	c := NewChecker(testConfig())
	if r := c.Check(context.Background(), httpMon(srv.URL), nil); r.OK || r.Reason != ReasonBlocked {
		t.Fatalf("http to loopback: ok=%v reason=%q", r.OK, r.Reason)
	}
	tcp := Monitor{ID: "t", Kind: KindTCP, Timeout: time.Second, Spec: Spec{Host: "127.0.0.1", Port: 9}}
	if r := c.Check(context.Background(), tcp, nil); r.OK || r.Reason != ReasonBlocked {
		t.Fatalf("tcp to loopback: ok=%v reason=%q", r.OK, r.Reason)
	}
	if err := ValidateTarget(context.Background(), httpMon(srv.URL)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("create-time target check should refuse loopback, got %v", err)
	}
}

func TestAggregateRollupMath(t *testing.T) {
	base := int64(7200)
	got := Aggregate([]Result{
		{MonitorID: "a", At: base + 1, OK: true, LatencyMS: 100},
		{MonitorID: "a", At: base + 61, OK: true, LatencyMS: 300},
		{MonitorID: "a", At: base + 121, OK: false, LatencyMS: 9000},
		{MonitorID: "a", At: base + 3600, OK: true, LatencyMS: 50},
		{MonitorID: "b", At: base + 5, OK: true, LatencyMS: 10},
	})
	if len(got) != 3 {
		t.Fatalf("want 3 buckets, got %d", len(got))
	}
	a := got[0]
	if a.Total != 3 || a.Failed != 1 || a.LatSum != 400 || a.LatMax != 300 || a.Bucket != base {
		t.Fatalf("bucket math wrong: %+v", a)
	}
}

type failingSink struct{ calls int }

func (f *failingSink) InsertResults(context.Context, []Result) error {
	f.calls++
	return errors.New("busy")
}

func TestBatcherBoundedAndRequeuesOnFailure(t *testing.T) {
	sink := &failingSink{}
	b := NewBatcher(sink, 3)
	for i := 0; i < 5; i++ {
		b.Add(Result{MonitorID: "a", At: int64(i)})
	}
	if b.Pending() != 3 || b.Dropped() != 2 {
		t.Fatalf("pending=%d dropped=%d", b.Pending(), b.Dropped())
	}
	if err := b.Flush(context.Background()); err == nil {
		t.Fatal("want flush error")
	}
	if b.Pending() != 3 {
		t.Fatalf("failed flush must keep results, pending=%d", b.Pending())
	}
}
