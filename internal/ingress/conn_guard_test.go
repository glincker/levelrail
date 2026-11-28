package ingress

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/glincker/levelrail/internal/trafficpolicy"
)

func TestConnGuard_Rendering(t *testing.T) {
	route := ProxyRoute{Hosts: []string{"a.test"}, BackendDial: "127.0.0.1:9000"}
	off := renderLimitsRoute(t, route, &Hardening{})
	if strings.Contains(off, "levelrail_conn_guard") {
		t.Fatalf("guard rendered while both limits are off: %s", off)
	}
	on := renderLimitsRoute(t, route, &Hardening{BodyReadTimeout: 30 * time.Minute, MaxConnsPerIP: 256})
	for _, want := range []string{`"handler":"levelrail_conn_guard"`, `"max_per_ip":256`, `"body_read_timeout":"30m0s"`} {
		if !strings.Contains(on, want) {
			t.Errorf("missing %s in %s", want, on)
		}
	}
}

func TestConnGuard_CapsPerIPButNotStreams(t *testing.T) {
	m := &connGuardModule{MaxPerIP: 1}
	if err := m.Provision(caddyCtx()); err != nil {
		t.Fatal(err)
	}
	m.limiter = trafficpolicy.NewIPLimiter(16, nil)
	hold, release := make(chan struct{}), make(chan struct{})
	first := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error {
		close(hold)
		<-release
		return nil
	})
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "http://a.test/", nil)
		r.RemoteAddr = "203.0.113.7:4000"
		return r
	}
	done := make(chan struct{})
	go func() {
		_ = m.ServeHTTP(httptest.NewRecorder(), req(), first)
		close(done)
	}()
	<-hold
	pass := caddyhttp.HandlerFunc(func(http.ResponseWriter, *http.Request) error { return nil })
	if err := m.ServeHTTP(httptest.NewRecorder(), req(), pass); err == nil {
		t.Fatal("second in-flight request from the same address was allowed")
	}
	ws := req()
	ws.Header.Set("Upgrade", "websocket")
	if err := m.ServeHTTP(httptest.NewRecorder(), ws, pass); err != nil {
		t.Fatalf("websocket counted against the cap: %v", err)
	}
	sse := req()
	sse.Header.Set("Accept", "text/event-stream")
	if err := m.ServeHTTP(httptest.NewRecorder(), sse, pass); err != nil {
		t.Fatalf("event stream counted against the cap: %v", err)
	}
	close(release)
	<-done
}

func TestCacheModule_DropsOverrideHeadersAndGetBody(t *testing.T) {
	h := newCacheHarness(allPaths)
	var gotHeader, gotLen = "x", int64(-1)
	h.resp = func(_ http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Original-Url") + r.Header.Get("X-Http-Method-Override") + r.Header.Get("X-Rewrite-Url") + r.Header.Get("X-Forwarded-Prefix")
		gotLen = r.ContentLength
	}
	r := httptest.NewRequest(http.MethodGet, "http://a.test/p", strings.NewReader("payload"))
	for _, k := range cacheOverrideHeaders {
		r.Header.Set(k, "/admin")
	}
	next := caddyhttp.HandlerFunc(func(w http.ResponseWriter, r *http.Request) error { h.resp(w, r); return nil })
	if err := h.m.ServeHTTP(httptest.NewRecorder(), r, next); err != nil {
		t.Fatal(err)
	}
	if gotHeader != "" || gotLen != 0 {
		t.Fatalf("forwarded header %q body length %d", gotHeader, gotLen)
	}
}
