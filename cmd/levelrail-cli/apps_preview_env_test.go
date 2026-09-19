package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsPreviewEnv_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody struct {
		Value string `json:"value"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appPreviewEnvOverride{Key: "SHARED", Value: gotBody.Value})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "preview-env", "set", "web", "SHARED", "--value", "preview-value", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/preview-env/SHARED" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/preview-env/SHARED", gotMethod, gotPath)
	}
	if gotBody.Value != "preview-value" {
		t.Errorf("request body = %+v, want value=preview-value", gotBody)
	}
	if !strings.Contains(stdout, "value=preview-value") {
		t.Errorf("stdout = %q, want a value line", stdout)
	}
}

func TestRun_AppsPreviewEnv_Set_MissingFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "preview-env", "set", "web", "SHARED", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --value") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsPreviewEnv_Set_MissingPositionalArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "preview-env", "set", "web", "--value", "v", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsPreviewEnv_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "preview-env", "set", "web", "SHARED", "--value", "preview-value", "--api-url", srv.URL})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_AppsPreviewEnv_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "preview-env", "clear", "web", "SHARED", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/preview-env/SHARED" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/preview-env/SHARED", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "cleared for app") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsPreviewEnv_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "preview-env", "-h"})
	if !strings.Contains(stdout, "apps preview-env set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsPreviewEnv_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "preview-env", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps preview-env subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
