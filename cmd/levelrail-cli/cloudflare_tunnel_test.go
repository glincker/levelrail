package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_CloudflareTunnel_Get(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareTunnelResource{Enabled: true, HasToken: true, Status: "connected"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"cloudflare-tunnel", "get", "--api-url", srv.URL})

	if gotMethod != http.MethodGet || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want GET /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:   true") || !strings.Contains(stdout, "status:    connected") {
		t.Errorf("stdout = %q, want enabled and status lines", stdout)
	}
}

func TestRun_CloudflareTunnel_Get_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareTunnelResource{Enabled: true, HasToken: true, Status: "connected"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"cloudflare-tunnel", "get", "--json", "--api-url", srv.URL})

	var got cloudflareTunnelResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if !got.Enabled || got.Status != "connected" {
		t.Errorf("got = %+v, want Enabled=true Status=connected", got)
	}
}

func TestRun_CloudflareTunnel_Get_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"cloudflare-tunnel", "get", "--api-url", srv.URL})

	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_CloudflareTunnel_Set(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody updateCloudflareTunnelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareTunnelResource{Enabled: true, HasToken: true, Status: "disconnected"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"cloudflare-tunnel", "set", "--cf-tunnel-token", "cf-tunnel-token", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if !gotBody.Enabled || gotBody.Token != "cf-tunnel-token" {
		t.Errorf("request body = %+v, want Enabled=true Token=cf-tunnel-token", gotBody)
	}
	if !strings.Contains(stdout, "enabled:   true") {
		t.Errorf("stdout = %q, want enabled: true", stdout)
	}
}

func TestRun_CloudflareTunnel_Set_Disable(t *testing.T) {
	var gotBody updateCloudflareTunnelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareTunnelResource{Enabled: false, HasToken: true, Status: "disconnected"})
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"cloudflare-tunnel", "set", "--enabled=false", "--api-url", srv.URL})

	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false")
	}
	if gotBody.Token != "" {
		t.Errorf("request body Token = %q, want empty (kept unchanged)", gotBody.Token)
	}
}

func TestRun_CloudflareTunnel_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"token is required the first time cloudflare tunnel is enabled"}`)

	stderr := runCLIExpectAPIError(t, []string{"cloudflare-tunnel", "set", "--api-url", srv.URL})

	if !strings.Contains(stderr, "token is required") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_CloudflareTunnel_Disconnect(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cloudflareTunnelResource{Enabled: false, HasToken: false, Status: "disconnected"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"cloudflare-tunnel", "disconnect", "--api-url", srv.URL})

	if gotMethod != http.MethodDelete || gotPath != "/api/v1/settings/cloudflare-tunnel" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/settings/cloudflare-tunnel", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "enabled:   false") {
		t.Errorf("stdout = %q, want enabled: false", stdout)
	}
}

func TestRun_CloudflareTunnel_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"cloudflare-tunnel", "-h"})
	if !strings.Contains(stdout, "cloudflare-tunnel set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_CloudflareTunnel_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"cloudflare-tunnel"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_CloudflareTunnel_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"cloudflare-tunnel", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
