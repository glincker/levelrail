package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsPreviewsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []previewEnvironmentResource{
		{PRNumber: 42, PreviewAppID: "web-pr-42", Branch: "feature-x", Status: "active", Domain: "pr-42.web.example.com", UpdatedAt: "2026-08-20T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "list", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/previews" {
		t.Errorf("path = %q, want /api/v1/apps/web/previews", gotPath)
	}
	if !strings.Contains(stdout, "web-pr-42") || !strings.Contains(stdout, "feature-x") {
		t.Errorf("stdout = %q, want the preview app and branch listed", stdout)
	}
}

func TestRun_AppsPreviewsList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []previewEnvironmentResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no active preview environments") {
		t.Errorf("stdout = %q, want an empty-state message", stdout)
	}
}

func TestRun_AppsPreviewsTeardown(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "teardown", "web", "42", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/apps/web/previews/42/teardown" {
		t.Errorf("request = %s %s, want POST /api/v1/apps/web/previews/42/teardown", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "torn down") {
		t.Errorf("stdout = %q, want a teardown confirmation", stdout)
	}
}

func TestRun_AppsPreviewsTeardown_BadPRNumber(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "previews", "teardown", "web", "not-a-number"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
}

func TestRun_AppsPreviewsEnable(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setPreviewEnabledRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "enable", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/preview-settings" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/preview-settings", gotMethod, gotPath)
	}
	if !gotBody.Enabled {
		t.Errorf("request body Enabled = false, want true")
	}
	if !strings.Contains(stdout, "enabled") {
		t.Errorf("stdout = %q, want an enabled confirmation", stdout)
	}
}

func TestRun_AppsPreviewsDisable(t *testing.T) {
	var gotBody setPreviewEnabledRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "previews", "disable", "web", "--api-url", srv.URL})
	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false")
	}
	if !strings.Contains(stdout, "disabled") {
		t.Errorf("stdout = %q, want a disabled confirmation", stdout)
	}
}
