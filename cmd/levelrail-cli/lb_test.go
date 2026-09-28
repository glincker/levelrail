package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type lbFake struct {
	cfg      *apiclient.LoadBalancerConfig
	requests []string
}

func (f *lbFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/loadbalancer":
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerResource{AppName: "web", Configured: f.cfg != nil, Config: f.cfg})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/loadbalancer":
			var cfg apiclient.LoadBalancerConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				t.Errorf("decode PUT: %v", err)
			}
			f.cfg = &cfg
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerResource{AppName: "web", Configured: true, Config: &cfg})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/status"):
			_, _ = w.Write([]byte(`{"service":"web","algorithm":"round_robin","ready":true,"reason":"Balancing","message":"ok","upstreams":[{"id":"web#0","dial":"127.0.0.1:1","state":"healthy","weight":1,"active_connections":4}]}`))
		case strings.HasSuffix(r.URL.Path, "/export"):
			_ = json.NewEncoder(w).Encode(apiclient.LoadBalancerArtifact{Format: r.URL.Query().Get("format"), Filename: "web-lb.tf", Body: "resource \"aws_lb\" {}\n", Warnings: []string{"heads up"}})
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_LB_ShowStatusClear(t *testing.T) {
	f := &lbFake{cfg: &apiclient.LoadBalancerConfig{Algorithm: "weighted", Weights: []int{3, 1}, ActiveHealth: &apiclient.LoadBalancerActiveHealth{Path: "/healthz", Interval: "5s"}}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"lb", "show", "web", "--api-url", srv.URL})
	for _, want := range []string{"algorithm:  weighted", "weights:    [3 1]", "GET /healthz every 5s"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q: %s", want, out)
		}
	}
	out, _ = runCLIExpectOK(t, []string{"lb", "status", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "127.0.0.1:1") || !strings.Contains(out, "healthy") || !strings.Contains(out, "active=4") {
		t.Errorf("status output = %s", out)
	}
	out, _ = runCLIExpectOK(t, []string{"lb", "clear", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "cleared") {
		t.Errorf("clear output = %s", out)
	}
}

func TestRun_LB_ShowUnconfigured(t *testing.T) {
	srv := httptest.NewServer((&lbFake{}).handler(t))
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"lb", "show", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "not configured") {
		t.Errorf("output = %s", out)
	}
}

func TestRun_LB_SetOverlaysOnlyPassedFlags(t *testing.T) {
	f := &lbFake{cfg: &apiclient.LoadBalancerConfig{Algorithm: "weighted", Weights: []int{3, 1}, DrainTimeout: "10s"}}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	runCLIExpectOK(t, []string{"lb", "set", "web", "--api-url", srv.URL, "--health-path", "/up", "--health-interval", "3s", "--rate-limit-rps", "20", "--upstream-tls", "--upstream-tls-server-name", "app.internal"})
	c := f.cfg
	if c.Algorithm != "weighted" || len(c.Weights) != 2 || c.DrainTimeout != "10s" {
		t.Errorf("untouched fields changed: %+v", c)
	}
	if c.ActiveHealth == nil || c.ActiveHealth.Path != "/up" || c.ActiveHealth.Interval != "3s" || c.RateLimit == nil || c.RateLimit.RPS != 20 || c.UpstreamTLS == nil || c.UpstreamTLS.ServerName != "app.internal" {
		t.Errorf("flags not applied: %+v", c)
	}

	runCLIExpectOK(t, []string{"lb", "set", "web", "--api-url", srv.URL, "--algorithm", "least_conn", "--upstream-tls=false"})
	if f.cfg.Algorithm != "least_conn" || f.cfg.Weights != nil || f.cfg.UpstreamTLS != nil {
		t.Errorf("changing algorithm must drop weights and tls=false must clear it: %+v", f.cfg)
	}
}

func TestRun_LB_SetRejectsBadWeights(t *testing.T) {
	var out, errBuf strings.Builder
	if code := run("cli", []string{"lb", "set", "web", "--weights", "3,x"}, &out, &errBuf, envMap()); code != exitUsage {
		t.Errorf("exit = %d, want usage", code)
	}
}

func TestRun_LB_Export(t *testing.T) {
	f := &lbFake{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	out, errOut := runCLIExpectOK(t, []string{"lb", "export", "web", "--format", "terraform", "--api-url", srv.URL})
	if !strings.Contains(out, "aws_lb") || !strings.Contains(errOut, "warning: heads up") {
		t.Errorf("stdout=%q stderr=%q", out, errOut)
	}
	if got := f.requests[len(f.requests)-1]; got != "GET /api/v1/apps/web/loadbalancer/export?format=terraform" {
		t.Errorf("request = %s", got)
	}

	file := filepath.Join(t.TempDir(), "lb.tf")
	runCLIExpectOK(t, []string{"lb", "export", "web", "--format", "cdk", "--out", file, "--api-url", srv.URL})
	if b, err := os.ReadFile(file); err != nil || !strings.Contains(string(b), "aws_lb") { //nolint:gosec // test temp path
		t.Errorf("out file = %q err %v", b, err)
	}

	var o, e strings.Builder
	if code := run("cli", []string{"lb", "export", "web"}, &o, &e, envMap()); code != exitUsage {
		t.Errorf("missing --format exit = %d, want usage", code)
	}
}

func TestRun_LB_Import(t *testing.T) {
	f := &lbFake{}
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	dir := t.TempDir()
	good := filepath.Join(dir, "app.yaml")
	if err := os.WriteFile(good, []byte("version: 1\nservices:\n  web:\n    build: { type: dockerfile }\n    port: 80\n    loadbalancer: { algorithm: uri_hash, drain_timeout: 5s }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runCLIExpectOK(t, []string{"lb", "import", "web", "--file", good, "--api-url", srv.URL})
	if f.cfg == nil || f.cfg.Algorithm != "uri_hash" || f.cfg.DrainTimeout != "5s" {
		t.Errorf("imported config = %+v", f.cfg)
	}

	none := filepath.Join(dir, "none.yaml")
	if err := os.WriteFile(none, []byte("version: 1\nservices:\n  web: { build: { type: dockerfile }, port: 80 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var o, e strings.Builder
	if code := run("cli", []string{"lb", "import", "web", "--file", none, "--api-url", srv.URL}, &o, &e, envMap()); code == exitOK || !strings.Contains(e.String(), "no loadbalancer block") {
		t.Errorf("import without block: exit=%d stderr=%q", code, e.String())
	}
}

func TestRun_LB_Usage(t *testing.T) {
	var o, e strings.Builder
	if code := run("cli", []string{"lb"}, &o, &e, envMap()); code != exitUsage || !strings.Contains(e.String(), "lb export") {
		t.Errorf("bare lb: exit=%d stderr=%q", code, e.String())
	}
	o.Reset()
	e.Reset()
	if code := run("cli", []string{"lb", "bogus"}, &o, &e, envMap()); code != exitUsage {
		t.Errorf("unknown verb exit = %d", code)
	}
	if code := run("cli", []string{"lb", "help"}, &o, &e, envMap()); code != exitOK || !strings.Contains(o.String(), "lb status") {
		t.Errorf("help: exit=%d", code)
	}
}
