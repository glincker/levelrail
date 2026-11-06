package ingress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serve(t *testing.T, info HostInfo, accept string) (int, string) {
	t.Helper()
	h := DiagnosticHandler(func(context.Context, string) HostInfo { return info }, "Levelrail", "https://d.example")
	req := httptest.NewRequest(http.MethodGet, "http://shop.example:8443/", nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestDiagnosticStates(t *testing.T) {
	cases := []struct {
		info   HostInfo
		status int
		head   string
		cmd    string
	}{
		{HostInfo{StateUnattached, ""}, 404, "not attached to an app", "levelrail-cli domains add &lt;app&gt; &lt;host&gt;"},
		{HostInfo{StateBuilding, "web"}, 503, "still building", "levelrail-cli apps deploys list web"},
		{HostInfo{StateStopped, "web"}, 503, "The app is stopped", "levelrail-cli apps start web"},
		{HostInfo{StateStarting, "web"}, 503, "The app is starting", "levelrail-cli logs web"},
		{HostInfo{StateUnhealthy, "web"}, 503, "no healthy replica", "levelrail-cli logs web"},
		{HostInfo{StateUnknown, ""}, 503, "No app is serving this address", "levelrail-cli domains list"},
	}
	for _, c := range cases {
		code, body := serve(t, c.info, "text/html")
		if code != c.status || !strings.Contains(body, c.head) || !strings.Contains(body, c.cmd) {
			t.Errorf("%s: code %d, body missing %q / %q", c.info.State, code, c.head, c.cmd)
		}
		_, plain := serve(t, c.info, "")
		if strings.Contains(plain, "<") || strings.Contains(plain, "web") {
			t.Errorf("%s: plain text must be short and name no app: %q", c.info.State, plain)
		}
	}
}

func TestDiagnosticEscapesAppName(t *testing.T) {
	_, body := serve(t, HostInfo{StateBuilding, `<b>x`}, "text/html")
	if strings.Contains(body, "<b>x") {
		t.Error("app name must be escaped")
	}
}

func TestCachedLookupTTL(t *testing.T) {
	var calls atomic.Int32
	inner := func(context.Context, string) HostInfo { calls.Add(1); return HostInfo{State: StateBuilding} }
	ttl := time.Hour
	l := CachedLookup(inner, func(context.Context) time.Duration { return ttl })
	l(context.Background(), "a.example")
	l(context.Background(), "a.example")
	if calls.Load() != 1 {
		t.Fatalf("cached call count %d", calls.Load())
	}
	ttl = 0
	l(context.Background(), "a.example")
	if calls.Load() != 2 {
		t.Fatalf("ttl 0 must bypass the cache, got %d", calls.Load())
	}
}
