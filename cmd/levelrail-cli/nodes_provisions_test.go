package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesProvisionsList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]nodeProvisionResource{{ID: "npv_1", Provider: "hetzner", Name: "web-1", Status: "ready"}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "provisions", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/node-provisions" {
		t.Errorf("path = %q, want /api/v1/node-provisions", gotPath)
	}
	if !strings.Contains(stdout, "npv_1") || !strings.Contains(stdout, "ready") {
		t.Errorf("stdout = %q, want the provision row", stdout)
	}
}

func TestRun_NodesProvisionsShow(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodeProvisionResource{ID: "npv_1", Provider: "hetzner", Name: "web-1", Status: "enrolling"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "provisions", "show", "npv_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/node-provisions/npv_1" {
		t.Errorf("path = %q, want /api/v1/node-provisions/npv_1", gotPath)
	}
	if !strings.Contains(stdout, "enrolling") {
		t.Errorf("stdout = %q, want the status", stdout)
	}
}

func TestRun_NodesProvisionsShow_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node provision not found"}`)
	stderr := runCLIExpectAPIError(t, []string{"nodes", "provisions", "show", "missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node provision not found") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRun_NodesProvisions_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "provisions", "-h"})
	if !strings.Contains(stdout, "nodes provisions") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}
