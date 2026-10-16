package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSleep(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		if r.URL.Path == "/api/v1/sleep" {
			_, _ = w.Write([]byte(`{"enabled":1,"sleeping":1,"reclaimed_bytes":268435456,"apps":[{"name":"web","idle_minutes":45,"sleeping":true,"memory_limit_bytes":268435456}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(appSleepResource{Enabled: true, IdleMinutes: 45})
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		args               []string
		method, path, body string
	}{
		{[]string{"enable", "web", "--after", "45m"}, http.MethodPut, "/api/v1/apps/web/sleep", `{"idle_minutes":45}`},
		{[]string{"disable", "web"}, http.MethodPut, "/api/v1/apps/web/sleep", `{"idle_minutes":0}`},
		{[]string{"status", "web"}, http.MethodGet, "/api/v1/apps/web/sleep/history", ""},
		{[]string{"history", "web", "--since", "24h"}, http.MethodGet, "/api/v1/apps/web/sleep/history", ""},
		{[]string{"wake", "web"}, http.MethodPost, "/api/v1/apps/web/sleep/wake", ""},
		{[]string{"now", "web"}, http.MethodPost, "/api/v1/apps/web/sleep/now", ""},
		{[]string{"list"}, http.MethodGet, "/api/v1/apps/web/sleep/history", ""},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"apps", "sleep"}, tc.args...)
		args = append(args, "--token", "t", "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v: exit = %d (stderr=%q)", tc.args, got, stderr.String())
		}
		if gotMethod != tc.method || gotPath != tc.path || strings.TrimSpace(gotBody) != tc.body {
			t.Errorf("%v: %s %s %q, want %s %s %q", tc.args, gotMethod, gotPath, gotBody, tc.method, tc.path, tc.body)
		}
	}
}

func TestRun_AppsSleepHistoryTable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"enabled":true,"sleeping":false,"events":[` +
			`{"id":2,"kind":"woke","at":"2026-10-15T10:00:00Z","reason":"request","trigger_path":"/api","trigger_source":"web.example.com","duration_seconds":3600,"wake_latency_ms":1800,"actor":"system"},` +
			`{"id":1,"kind":"slept","at":"2026-10-15T09:00:00Z","reason":"idle_timeout","idle_minutes":30,"actor":"system"}],"stats":{"asleep_seconds_7d":3600,"asleep_seconds_30d":3600,"wake_count_7d":1}}`))
	}))
	t.Cleanup(srv.Close)
	var stdout, stderr bytes.Buffer
	args := []string{"apps", "sleep", "history", "web", "--token", "t", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d (stderr=%q)", got, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"WHEN", "DURATION/LATENCY", "woke", "request", "slept 1h0m0s, ready in 1.8s", "/api; from web.example.com", "after 30 min idle"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRun_AppsSleepEnableFlagsAndRefusal(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		if r.Method == http.MethodPut {
			gotBody = b.String()
			if strings.Contains(gotBody, `"port_wake":false`) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"sleep when idle cannot be turned on: Pinned host port needs port wake"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"enabled":false,"idle_minutes":0,"eligible":false,"blockers":[{"code":"pinned_port","title":"Pinned host port needs port wake","detail":"Traffic on host port 3000 bypasses the ingress.","fix":{"kind":"enable_port_wake","label":"Turn on port wake","method":"PUT","path":"/api/v1/apps/grafana/sleep"}}],"warnings":[{"code":"no_domain","title":"No domain attached","detail":"No ingress request is ever seen."}],"port_wake":{"supported":true,"active":false,"ports":[3000]},"activity":{"open_connections":0,"idle_for_seconds":0}}`))
	}))
	t.Cleanup(srv.Close)
	run1 := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		args = append(append([]string{"apps", "sleep"}, args...), "--token", "t", "--api-url", srv.URL)
		code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		return code, stdout.String(), stderr.String()
	}
	if code, _, errOut := run1("enable", "grafana", "--after", "30m", "--port-wake", "--ignore-path", "/metrics", "--ignore-path", "/up"); code != exitOK {
		t.Fatalf("enable exit = %d (%s)", code, errOut)
	}
	if want := `{"idle_minutes":30,"ignore_paths":["/metrics","/up"],"port_wake":true}`; strings.TrimSpace(gotBody) != want {
		t.Errorf("body = %s, want %s", gotBody, want)
	}
	code, _, errOut := run1("enable", "grafana", "--after", "30m", "--no-port-wake")
	if code == exitOK || !strings.Contains(errOut, "Pinned host port needs port wake") || !strings.Contains(errOut, "fix: Turn on port wake") {
		t.Errorf("refusal must list the blocker and its fix, got code %d: %s", code, errOut)
	}
	if code, _, _ := run1("enable", "grafana", "--port-wake", "--no-port-wake"); code != exitUsage {
		t.Errorf("both port wake flags = %d, want usage error", code)
	}
	code, out, _ := run1("status", "grafana")
	if code != exitOK {
		t.Fatalf("status exit = %d", code)
	}
	for _, want := range []string{"eligible: false", "blocker: Pinned host port needs port wake", "apps sleep enable grafana --port-wake", "warning: No domain attached", "ports=[3000]"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
}
