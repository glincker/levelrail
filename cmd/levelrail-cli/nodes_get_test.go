package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_NodesGet(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{
			ID: "nd_1", Name: "web-1", Address: "10.0.0.1", Status: "ready",
			Schedulable: true, AcceptsAppWorkloads: true,
			CreatedAt: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "get", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/api/v1/nodes/nd_1" {
		t.Errorf("path = %q, want /api/v1/nodes/nd_1", gotPath)
	}
	if !strings.Contains(stdout, "id:") || !strings.Contains(stdout, "nd_1") {
		t.Errorf("stdout = %q, want the node's id field", stdout)
	}
	if !strings.Contains(stdout, "schedulable:") {
		t.Errorf("stdout = %q, want the schedulable field", stdout)
	}
}

func TestRun_NodesGet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{ID: "nd_1", Name: "web-1", Status: "ready"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "get", "nd_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"id": "nd_1"`) {
		t.Errorf("stdout = %q, want the node as JSON", stdout)
	}
}

func TestRun_NodesGet_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "get", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesGet_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "get")
}

func TestRun_NodesGet_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "get", "-h"})
	if !strings.Contains(stderr, "nodes get") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

// assertUsageErrorMissingID runs "nodes <verb> <no args>" and asserts a
// missing-id usage error, the same shape assertUsageErrorMissingName
// (clitest_helpers_test.go) checks for "apps" subcommands.
func assertUsageErrorMissingID(t *testing.T, verb string) {
	t.Helper()
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	got := run("levelrail-cli-test", []string{"nodes", verb}, stdout, stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-id usage error", stderr.String())
	}
}
