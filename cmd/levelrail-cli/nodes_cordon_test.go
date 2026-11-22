package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesCordon(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{ID: "nd_1", Name: "web-1", Schedulable: false})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "cordon", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/cordon" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/nd_1/cordon", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `schedulable: false`) {
		t.Errorf("stdout = %q, want the new schedulable state", stdout)
	}
}

func TestRun_NodesUncordon(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeResource{ID: "nd_1", Name: "web-1", Schedulable: true})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "uncordon", "nd_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/uncordon" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/nd_1/uncordon", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `schedulable: true`) {
		t.Errorf("stdout = %q, want the new schedulable state", stdout)
	}
}

func TestRun_NodesCordon_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "cordon", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesCordon_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "cordon")
}

func TestRun_NodesUncordon_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "uncordon")
}

func TestRun_NodesCordon_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "cordon", "-h"})
	if !strings.Contains(stderr, "nodes cordon") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_NodesUncordon_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "uncordon", "-h"})
	if !strings.Contains(stderr, "nodes uncordon") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_NodesBulkCordonByZone(t *testing.T) {
	var cordoned []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]nodeResource{{ID: "nd_1", Name: "a", Zone: "eu-1"}, {ID: "nd_2", Name: "b", Zone: "eu-2"}, {ID: "nd_3", Name: "c", Zone: "eu-1"}})
			return
		}
		cordoned = append(cordoned, r.URL.Path)
		_ = json.NewEncoder(w).Encode(nodeResource{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "bulk", "cordon", "--where", "zone=eu-1", "--dry-run", "--api-url", srv.URL})
	if len(cordoned) != 0 || !strings.Contains(stdout, "would cordon") {
		t.Fatalf("dry run changed nodes: %v %q", cordoned, stdout)
	}
	runCLIExpectOK(t, []string{"nodes", "bulk", "cordon", "--where", "zone=eu-1", "--api-url", srv.URL})
	if len(cordoned) != 2 || cordoned[0] != "/api/v1/nodes/nd_1/cordon" || cordoned[1] != "/api/v1/nodes/nd_3/cordon" {
		t.Errorf("cordoned = %v", cordoned)
	}
}
