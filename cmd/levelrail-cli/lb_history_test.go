package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func lbHistoryServer(t *testing.T, requests *[]string, bodies *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests = append(*requests, r.Method+" "+r.URL.EscapedPath()+"?"+r.URL.RawQuery)
		b, _ := io.ReadAll(r.Body)
		*bodies = append(*bodies, string(b))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/history"):
			_, _ = w.Write([]byte(`{"upstreams":[{"id":"web#0","dial":"127.0.0.1:1","admin_state":"active","checks":[{"at":"2026-01-01T00:00:00Z","ok":true,"status_code":200,"latency_ms":5,"reason":""},{"at":"2026-01-01T00:00:15Z","ok":false,"status_code":503,"latency_ms":6,"reason":"expected 200, got 503"}],"transitions":[{"at":"2026-01-01T00:00:15Z","from":"healthy","to":"unhealthy","reason":"expected 200, got 503"}],"series":{"connections":[],"latency_ms":[],"fails":[]}}]}`))
		case strings.HasSuffix(r.URL.Path, "/check"):
			_, _ = w.Write([]byte(`{"results":[{"id":"web#0","dial":"127.0.0.1:1","ok":false,"status_code":503,"latency_ms":6,"reason":"expected 200, got 503"}],"note":"probed GET /"}`))
		case strings.Contains(r.URL.Path, "/upstreams/"):
			_, _ = w.Write([]byte(`{"id":"web#0","dial":"127.0.0.1:1","state":"draining","admin_state":"draining","reason":"draining, no new connections"}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRun_LB_CheckHistoryUpstream(t *testing.T) {
	var requests, bodies []string
	srv := lbHistoryServer(t, &requests, &bodies)
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"lb", "check", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "FAIL") || !strings.Contains(out, "expected 200, got 503") || !strings.Contains(out, "note: probed GET /") {
		t.Errorf("check output = %s", out)
	}
	out, _ = runCLIExpectOK(t, []string{"lb", "history", "web", "--limit", "5", "--api-url", srv.URL})
	if !strings.Contains(out, "checks=2 passed=1") || !strings.Contains(out, "healthy -> unhealthy") {
		t.Errorf("history output = %s", out)
	}
	out, _ = runCLIExpectOK(t, []string{"lb", "upstream", "web", "web#0", "--state", "draining", "--api-url", srv.URL})
	if !strings.Contains(out, "admin_state=draining") {
		t.Errorf("upstream output = %s", out)
	}

	want := []string{
		"POST /api/v1/apps/web/loadbalancer/check?",
		"GET /api/v1/apps/web/loadbalancer/history?limit=5",
		"PUT /api/v1/apps/web/loadbalancer/upstreams/web%230?",
	}
	for i, w := range want {
		if requests[i] != w {
			t.Errorf("request %d = %q, want %q", i, requests[i], w)
		}
	}
	if bodies[2] != `{"admin_state":"draining"}`+"\n" && bodies[2] != `{"admin_state":"draining"}` {
		t.Errorf("upstream body = %q", bodies[2])
	}
}

func TestRun_LB_UpstreamAndHistoryUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"missing state", []string{"lb", "upstream", "web", "web#0"}},
		{"bad state", []string{"lb", "upstream", "web", "web#0", "--state", "paused"}},
		{"missing id", []string{"lb", "upstream", "web", "--state", "active"}},
		{"bad limit", []string{"lb", "history", "web", "--limit", "0"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errb strings.Builder
			if code := run("cli", tt.args, &out, &errb, envMap()); code != exitUsage {
				t.Errorf("exit = %d, want %d (stderr %s)", code, exitUsage, errb.String())
			}
		})
	}
}
