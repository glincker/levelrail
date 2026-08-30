package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsStart(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "levelrail/web:1", Port: 3000})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "start", "web", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/start" {
		t.Errorf("path = %q, want /api/v1/apps/web/start", gotPath)
	}
	if !strings.Contains(stdout.String(), `"name": "web"`) {
		t.Errorf("stdout = %q, want it to contain the started app as JSON", stdout.String())
	}
}

func TestRun_AppsStart_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "start", "ghost", "--api-url", srv.URL})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsStart_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "start"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsStart_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"apps", "start", "-h"})
	if !strings.Contains(stderr, "apps start") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
