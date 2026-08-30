package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_NodesHealth(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]conditionResource{
			{Type: "Heartbeat", Status: "True", Reason: "Alive", Message: "node is reachable", LastTransitionTime: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "health", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/nodes/nd_1/health" {
		t.Errorf("request = %s %s, want GET /api/v1/nodes/nd_1/health", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "Heartbeat") || !strings.Contains(stdout, "node is reachable") {
		t.Errorf("stdout = %q, want the condition listed", stdout)
	}
}

func TestRun_NodesHealth_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]conditionResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "health", "nd_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no conditions reported yet") {
		t.Errorf("stdout = %q, want the no-conditions message", stdout)
	}
}

func TestRun_NodesHealth_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]conditionResource{
			{Type: "Heartbeat", Status: "True"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "health", "nd_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"Type": "Heartbeat"`) {
		t.Errorf("stdout = %q, want the conditions as JSON", stdout)
	}
}

func TestRun_NodesHealth_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "health", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesHealth_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "health")
}

func TestRun_NodesHealth_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "health", "-h"})
	if !strings.Contains(stderr, "nodes health") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
