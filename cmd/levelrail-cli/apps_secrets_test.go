package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSecretsList(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]secretKeyResource{{Key: "API_KEY", Locked: true}, {Key: "DB_PASSWORD", Locked: false}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "list", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps/web/secrets" {
		t.Errorf("request = %s %s, want GET /api/v1/apps/web/secrets", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "API_KEY") || !strings.Contains(stdout, "DB_PASSWORD") {
		t.Errorf("stdout = %q, want both secret keys listed", stdout)
	}
	if strings.Contains(stdout, "secret-value") {
		t.Errorf("stdout = %q, must never contain a secret value", stdout)
	}
}

func TestRun_AppsSecretsList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]secretKeyResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no secrets set") {
		t.Errorf("stdout = %q, want the empty-state message", stdout)
	}
}

func TestRun_AppsSecretsSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/secrets/API_KEY" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/secrets/API_KEY", gotMethod, gotPath)
	}
	if gotBody["value"] != "s3cr3t" {
		t.Errorf("body value = %v, want s3cr3t", gotBody["value"])
	}
	if gotBody["overwrite_locked"] != false {
		t.Errorf("body overwrite_locked = %v, want false by default", gotBody["overwrite_locked"])
	}
	if !strings.Contains(stdout, `secret "API_KEY" set for app "web"`) {
		t.Errorf("stdout = %q, want a set confirmation", stdout)
	}
}

func TestRun_AppsSecretsSet_Force(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--force", "--api-url", srv.URL})
	if gotBody["overwrite_locked"] != true {
		t.Errorf("body overwrite_locked = %v, want true with --force", gotBody["overwrite_locked"])
	}
}

func TestRun_AppsSecretsSet_Locked(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"secret is locked, set overwrite_locked to overwrite"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "secrets", "set", "web", "API_KEY", "s3cr3t", "--api-url", srv.URL})
	if !strings.Contains(stderr, "locked") {
		t.Errorf("stderr = %q, want the server's locked-conflict message", stderr)
	}
}

func TestRun_AppsSecretsSet_MissingArgs(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "secrets", "set", "web", "API_KEY"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsSecretsLock(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "lock", "web", "API_KEY", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/secrets/API_KEY/lock" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/secrets/API_KEY/lock", gotMethod, gotPath)
	}
	if gotBody["locked"] != true {
		t.Errorf("body locked = %v, want true by default", gotBody["locked"])
	}
	if !strings.Contains(stdout, `secret "API_KEY" on app "web" locked`) {
		t.Errorf("stdout = %q, want a locked confirmation", stdout)
	}
}

func TestRun_AppsSecretsLock_Unlock(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "lock", "web", "API_KEY", "--locked=false", "--api-url", srv.URL})
	if gotBody["locked"] != false {
		t.Errorf("body locked = %v, want false with --locked=false", gotBody["locked"])
	}
	if !strings.Contains(stdout, `unlocked`) {
		t.Errorf("stdout = %q, want an unlocked confirmation", stdout)
	}
}

func TestRun_AppsSecrets_UnknownSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "secrets", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps secrets subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}

func TestRun_AppsSecrets_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "secrets", "-h"})
	if !strings.Contains(stdout, "apps secrets") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
