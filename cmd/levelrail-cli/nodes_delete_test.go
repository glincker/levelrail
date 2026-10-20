package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRun_NodesDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "delete", "nd_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/nodes/nd_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/nodes/nd_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `node "nd_1" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_NodesDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "delete", "nd_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_NodesDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "delete", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

// TestRun_NodesDelete_Conflict proves the server's real 409 message
// (which already tells the operator to drain first) reaches stderr
// as-is, not replaced by a generic failure string.
func TestRun_NodesDelete_Conflict(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict,
		`{"error":"node \"nd_1\" still has 2 service(s) and 1 database(s) placed on it; drain it first via POST /api/v1/nodes/nd_1/drain"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "delete", "nd_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "drain it first") {
		t.Errorf("stderr = %q, want the server's drain-first message", stderr)
	}
	if !strings.Contains(stderr, "2 service(s)") {
		t.Errorf("stderr = %q, want the server's placement counts", stderr)
	}
}

func TestRun_NodesDelete_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "delete")
}

func TestRun_NodesDelete_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "delete", "-h"})
	if !strings.Contains(stderr, "nodes delete") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
