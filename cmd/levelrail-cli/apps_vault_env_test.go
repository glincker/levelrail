package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsVaultEnv_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody appVaultEnvRef
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appVaultEnvRef{Path: gotBody.Path, Key: gotBody.Key})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "vault-env", "set", "web", "API_KEY", "--path", "myapp/config", "--key", "api_key", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/vault-env/API_KEY" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/vault-env/API_KEY", gotMethod, gotPath)
	}
	if gotBody.Path != "myapp/config" || gotBody.Key != "api_key" {
		t.Errorf("request body = %+v, want path=myapp/config key=api_key", gotBody)
	}
	if !strings.Contains(stdout, "path=myapp/config") || !strings.Contains(stdout, "key=api_key") {
		t.Errorf("stdout = %q, want path/key line", stdout)
	}
}

func TestRun_AppsVaultEnv_Set_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "vault-env", "set", "web", "API_KEY", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --path and --key") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsVaultEnv_Set_MissingPositionalArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "vault-env", "set", "web", "--path", "p", "--key", "k", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsVaultEnv_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"env var \"API_KEY\" is already secret-backed, remove it from secret_env first"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "vault-env", "set", "web", "API_KEY", "--path", "myapp/config", "--key", "api_key", "--api-url", srv.URL})
	if !strings.Contains(stderr, "already secret-backed") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_AppsVaultEnv_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "vault-env", "clear", "web", "API_KEY", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/vault-env/API_KEY" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/vault-env/API_KEY", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "cleared for app") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsVaultEnv_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "vault-env", "-h"})
	if !strings.Contains(stdout, "apps vault-env set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsVaultEnv_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "vault-env", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps vault-env subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
