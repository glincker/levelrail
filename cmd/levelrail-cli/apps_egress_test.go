package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsEgress_Get(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appEgressPolicyResource{
			AppName: "web", Mode: "allowlist",
			Allow: []appEgressAllow{{Host: "api.anthropic.com", Port: 443}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "egress", "get", "web", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/egress-policy" {
		t.Errorf("method/path = %s %s, want GET /api/v1/apps/web/egress-policy", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "mode:     allowlist") || !strings.Contains(stdout, "api.anthropic.com:443") {
		t.Errorf("stdout = %q, want mode and allow entry lines", stdout)
	}
}

func TestRun_AppsEgress_Get_Unconfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appEgressPolicyResource{AppName: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "egress", "get", "web", "--api-url", srv.URL})

	if !strings.Contains(stdout, "unrestricted") {
		t.Errorf("stdout = %q, want an unrestricted-egress message", stdout)
	}
}

func TestRun_AppsEgress_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppEgressPolicyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appEgressPolicyResource{AppName: "web", Mode: gotBody.Mode, Allow: gotBody.Allow})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"apps", "egress", "set", "web",
		"--allow", "api.anthropic.com:443",
		"--allow", "github.com:443",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/egress-policy" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/egress-policy", gotMethod, gotPath)
	}
	if gotBody.Mode != "allowlist" {
		t.Errorf("request Mode = %q, want allowlist", gotBody.Mode)
	}
	if len(gotBody.Allow) != 2 || gotBody.Allow[0] != (appEgressAllow{Host: "api.anthropic.com", Port: 443}) || gotBody.Allow[1] != (appEgressAllow{Host: "github.com", Port: 443}) {
		t.Errorf("request Allow = %+v, want both entries in order", gotBody.Allow)
	}
	if !strings.Contains(stdout, "github.com:443") {
		t.Errorf("stdout = %q, want the resulting policy printed", stdout)
	}
}

func TestRun_AppsEgress_Set_MissingAllow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "egress", "set", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires at least one --allow") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsEgress_Set_InvalidAllowFormat(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "egress", "set", "web", "--allow", "not-a-host-port", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "invalid host:port") {
		t.Errorf("stderr = %q, want an invalid host:port error", stderr.String())
	}
}

func TestRun_AppsEgress_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"allow entries require a non-empty host"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "egress", "set", "web", "--allow", "api.example.com:443", "--api-url", srv.URL})
	if !strings.Contains(stderr, "allow entries require a non-empty host") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_AppsEgress_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "egress", "clear", "web", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/egress-policy" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/egress-policy", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "egress policy cleared") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsEgress_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "egress", "-h"})
	if !strings.Contains(stdout, "apps egress set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsEgress_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "egress", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps egress subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
