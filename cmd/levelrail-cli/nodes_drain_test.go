package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesDrain(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(drainNodeResponse{
			TargetNodeID:   "nd_2",
			MovedServices:  []string{"web"},
			MovedDatabases: []string{"main"},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "drain", "nd_1", "--target", "nd_2", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/drain" {
		t.Errorf("request = %s %s, want POST /api/v1/nodes/nd_1/drain", gotMethod, gotPath)
	}
	if gotQuery != "target_node_id=nd_2" {
		t.Errorf("query = %q, want target_node_id=nd_2", gotQuery)
	}
	if !strings.Contains(stdout, "web") || !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want the moved service and database listed", stdout)
	}
}

func TestRun_NodesDrain_DefaultTarget(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(drainNodeResponse{})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "drain", "nd_1", "--api-url", srv.URL})
	if gotQuery != "" {
		t.Errorf("query = %q, want no target_node_id param when --target is omitted", gotQuery)
	}
	if !strings.Contains(stdout, "(local)") {
		t.Errorf("stdout = %q, want the local-node target label", stdout)
	}
}

// TestRun_NodesDrain_PartialFailure proves a drain that moved some
// resources but not others still prints what did move, surfaces the
// per-resource errors, and exits non-zero.
func TestRun_NodesDrain_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_ = json.NewEncoder(w).Encode(drainNodeResponse{
			TargetNodeID:  "",
			MovedServices: []string{"web"},
			Errors:        []string{"database main: connection refused"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes", "drain", "nd_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "web") {
		t.Errorf("stdout = %q, want the moved service listed", stdout.String())
	}
	if !strings.Contains(stdout.String(), "database main: connection refused") {
		t.Errorf("stdout = %q, want the per-resource error listed", stdout.String())
	}
}

func TestRun_NodesDrain_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"node not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"nodes", "drain", "nd_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "node not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_NodesDrain_NoID(t *testing.T) {
	assertUsageErrorMissingID(t, "drain")
}

func TestRun_NodesDrain_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"nodes", "drain", "-h"})
	if !strings.Contains(stderr, "nodes drain") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
