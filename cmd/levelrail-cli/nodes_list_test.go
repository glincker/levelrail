package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRun_NodesList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []nodeResource{
		{ID: "nd_1", Name: "web-1", Address: "10.0.0.1", Status: "ready", Schedulable: true, CreatedAt: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/nodes" {
		t.Errorf("path = %q, want /api/v1/nodes", gotPath)
	}
	if !strings.Contains(stdout, "nd_1") || !strings.Contains(stdout, "web-1") {
		t.Errorf("stdout = %q, want the node listed", stdout)
	}
}

func TestRun_NodesList_Empty(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []nodeResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "list", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no nodes") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_NodesList_JSON(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []nodeResource{
		{ID: "nd_1", Name: "web-1", Status: "ready", CreatedAt: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "list", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"id": "nd_1"`) {
		t.Errorf("stdout = %q, want the node as JSON", stdout)
	}
}

func TestRun_NodesList_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"internal error"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "list", "--api-url", srv.URL})
	if !strings.Contains(stderr, "internal error") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesList_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "list", "-h"})
	if !strings.Contains(stderr, "nodes list") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
