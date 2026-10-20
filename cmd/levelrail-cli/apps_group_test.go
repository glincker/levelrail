package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsGroup(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(appGroupResource{
			AppID: "myapp",
			Services: []appResource{
				{Name: "myapp-web", Image: "ghcr.io/x/myapp-web:abc", Port: 3000},
				{Name: "myapp-worker", Image: "ghcr.io/x/myapp-worker:abc", Port: 4000},
			},
			Status: appStatusSummary{Label: "Healthy", Variant: "success"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "group", "myapp-web", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/apps/myapp-web/group" {
		t.Errorf("path = %q, want /api/v1/apps/myapp-web/group", gotPath)
	}
	if !strings.Contains(stdout.String(), `"app_id": "myapp"`) {
		t.Errorf("stdout = %q, want the app group as JSON", stdout.String())
	}
}

func TestRun_AppsGroup_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(appGroupResource{
			AppID: "myapp",
			Services: []appResource{
				{Name: "myapp-web", Image: "ghcr.io/x/myapp-web:abc", Port: 3000},
			},
			Status: appStatusSummary{Label: "Healthy", Variant: "success"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "group", "myapp-web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "app_id:  myapp") {
		t.Errorf("stdout = %q, want app_id line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Healthy") {
		t.Errorf("stdout = %q, want the status label", stdout.String())
	}
	if !strings.Contains(stdout.String(), "myapp-web") {
		t.Errorf("stdout = %q, want the service listed", stdout.String())
	}
}

func TestRun_AppsGroup_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "group"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsGroup_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "group", "myapp", "--api-url", srv.URL})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsGroup_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "group", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps group") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
