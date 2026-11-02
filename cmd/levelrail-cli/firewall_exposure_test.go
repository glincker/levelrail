package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FirewallRestrictTrialKeepForceAndConfirm(t *testing.T) {
	var calls []string
	var last map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		last = map[string]any{}
		_ = json.NewDecoder(r.Body).Decode(&last)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"commands":[],"port":8108,"protocol":"tcp","applied":true,"pending":true,"revert_at":"2026-01-01T00:01:00Z"}`))
	}))
	defer srv.Close()
	run1 := func(args ...string) string {
		t.Helper()
		var stdout, stderr bytes.Buffer
		if got := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v exit = %d (stderr=%q)", args, got, stderr.String())
		}
		return stdout.String()
	}

	out := run1("firewall", "restrict", "--port", "8108", "--allow", "10.0.0.2", "--apply")
	if last["auto_revert"] != true || last["force_lockout"] != nil || !strings.Contains(out, "firewall confirm --port 8108") {
		t.Errorf("default apply: body=%v out=%q, want auto_revert and the confirm hint", last, out)
	}
	run1("firewall", "restrict", "--port", "8108", "--allow", "10.0.0.2", "--apply", "--keep", "--force-lockout")
	if last["auto_revert"] != nil || last["force_lockout"] != true {
		t.Errorf("--keep --force-lockout body = %v", last)
	}
	run1("firewall", "confirm", "--port", "8108")
	if got := calls[len(calls)-1]; got != "POST /api/v1/firewall/exposure/restrictions/tcp/8108/confirm" {
		t.Errorf("confirm call = %q", got)
	}
}
