package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func TestRequestStats_ObserveAndDrainByApp(t *testing.T) {
	s := NewRequestStats()
	s.SetHostOwners(map[string]string{"a.test": "web", "b.test": "web", "c.test": "api"})

	s.Observe("a.test", 200, 3*time.Millisecond, 10, 100, false)
	s.Observe("a.test", 404, 30*time.Millisecond, 0, 5, false)
	s.Observe("b.test", 503, 20*time.Second, 0, 0, true)
	s.Observe("c.test", 302, 500*time.Millisecond, 0, 0, false)
	s.Observe("unmapped.test", 200, time.Millisecond, 0, 0, false)

	got := s.DrainByApp()
	if len(got) != 2 {
		t.Fatalf("apps = %d, want 2 (unmapped dropped): %v", len(got), got)
	}
	web := got["web"]
	if web.Requests != 3 || web.Status2xx != 1 || web.Status4xx != 1 || web.Status5xx != 1 || web.UpstreamErrors != 1 {
		t.Errorf("web window = %+v", web)
	}
	if web.BytesIn != 10 || web.BytesOut != 105 {
		t.Errorf("web bytes = %d/%d", web.BytesIn, web.BytesOut)
	}
	if web.Latency[0] != 1 || web.Latency[3] != 1 || web.Latency[11] != 1 {
		t.Errorf("web latency buckets = %v", web.Latency)
	}
	if got["api"].Status3xx != 1 || got["api"].Latency[6] != 1 {
		t.Errorf("api window = %+v", got["api"])
	}
	if again := s.DrainByApp(); len(again) != 0 {
		t.Errorf("second drain = %v, want empty", again)
	}
}

func TestLatencyBucketBoundaries(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want int
	}{
		{0, 0}, {5 * time.Millisecond, 0}, {6 * time.Millisecond, 1},
		{10 * time.Second, 10}, {11 * time.Second, 11},
	}
	for _, tc := range tests {
		if got := latencyBucket(tc.d); got != tc.want {
			t.Errorf("latencyBucket(%v) = %d, want %d", tc.d, got, tc.want)
		}
	}
}

func TestRequestStatsModule_StatusFromWriterAndError(t *testing.T) {
	tests := []struct {
		name string
		next caddyhttp.HandlerFunc
		want func(w windowView) bool
	}{
		{"implicit 200", func(w http.ResponseWriter, _ *http.Request) error {
			_, _ = w.Write([]byte("hello"))
			return nil
		}, func(w windowView) bool { return w.s2 == 1 && w.out == 5 }},
		{"explicit 404", func(w http.ResponseWriter, _ *http.Request) error {
			w.WriteHeader(http.StatusNotFound)
			return nil
		}, func(w windowView) bool { return w.s4 == 1 }},
		{"proxy failure error", func(http.ResponseWriter, *http.Request) error {
			return caddyhttp.Error(http.StatusBadGateway, errors.New("dial failed"))
		}, func(w windowView) bool { return w.s5 == 1 && w.up == 1 }},
		{"plain error is 500 not upstream", func(http.ResponseWriter, *http.Request) error {
			return errors.New("boom")
		}, func(w windowView) bool { return w.s5 == 1 && w.up == 0 }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stats := DefaultRequestStats()
			stats.SetHostOwners(map[string]string{"mod.test": "modapp"})
			stats.DrainByApp()

			m := requestStatsModule{Key: "mod.test"}
			rr := httptest.NewRecorder()
			_ = m.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/secret/path?token=x", nil), tc.next)

			w := stats.DrainByApp()["modapp"]
			v := windowView{s2: w.Status2xx, s4: w.Status4xx, s5: w.Status5xx, up: w.UpstreamErrors, out: w.BytesOut}
			if !tc.want(v) {
				t.Errorf("window = %+v", w)
			}
		})
	}
}

type windowView struct{ s2, s4, s5, up, out uint64 }

func TestBuildRoutesConfig_RequestStatsHandler(t *testing.T) {
	build := func(on bool) string {
		cfg, err := BuildRoutesConfig(RoutesOptions{
			ServerName: "s", ListenAddr: ":8080", RequestStats: on,
			Routes:       []ProxyRoute{{Hosts: []string{"a.test", "b.test"}, BackendDial: "127.0.0.1:1"}},
			StaticRoutes: []StaticRoute{{Hosts: []string{"s.test"}, RootDir: "/srv"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(cfg)
		return string(b)
	}
	if strings.Contains(build(false), "request_stats") {
		t.Error("request_stats present with RequestStats off")
	}
	on := build(true)
	if strings.Count(on, `"handler":"request_stats"`) != 2 || !strings.Contains(on, `"key":"a.test"`) || !strings.Contains(on, `"key":"s.test"`) {
		t.Errorf("config = %s", on)
	}
	if strings.Index(on, "request_stats") > strings.Index(on, "reverse_proxy") {
		t.Error("request_stats must wrap the proxy handler, not follow it")
	}
}

func TestDriver_RequestStats_RealCaddy(t *testing.T) {
	skipUnderLoad(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, "ok")
	}))
	defer backend.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	deadBackend := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "red", ListenAddr: addr, RequestStats: true, StorageDir: t.TempDir(),
		Routes: []ProxyRoute{
			{Hosts: []string{"live.test"}, BackendDial: backend.Listener.Addr().String()},
			{Hosts: []string{"dead.test"}, BackendDial: deadBackend},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Apps.HTTP.Servers["red"].AutomaticHTTPS = &AutoHTTPSConfig{Disabled: true}
	stats := DefaultRequestStats()
	stats.SetHostOwners(map[string]string{"live.test": "liveapp", "dead.test": "deadapp"})
	stats.DrainByApp()

	d := New(testLogger(t))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := d.Apply(ctx, cfg); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	defer func() { _ = d.Stop(ctx) }()

	get := func(host, path string) int {
		var lastErr error
		for i := 0; i < 20; i++ {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+path, nil)
			req.Host = host
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				return resp.StatusCode
			}
			lastErr = err
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatalf("request failed: %v", lastErr)
		return 0
	}
	get("live.test", "/")
	get("live.test", "/")
	get("live.test", "/missing")
	if code := get("dead.test", "/"); code != http.StatusBadGateway {
		t.Fatalf("dead backend status = %d, want 502", code)
	}

	got := stats.DrainByApp()
	live, dead := got["liveapp"], got["deadapp"]
	if live.Requests != 3 || live.Status2xx != 2 || live.Status4xx != 1 || live.BytesOut == 0 {
		t.Errorf("live window = %+v", live)
	}
	if dead.Requests != 1 || dead.Status5xx != 1 || dead.UpstreamErrors != 1 {
		t.Errorf("dead window = %+v", dead)
	}
}
