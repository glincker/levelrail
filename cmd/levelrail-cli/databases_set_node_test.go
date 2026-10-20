package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesSetNode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppNodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db", NodeID: gotBody.NodeID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "set-node", "db", "node_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/node" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/db/node", gotMethod, gotPath)
	}
	if gotBody.NodeID != "node_1" {
		t.Errorf("request body NodeID = %q, want node_1", gotBody.NodeID)
	}
	if !strings.Contains(stdout, `database "db" moved to node "node_1"`) {
		t.Errorf("stdout = %q, want a move confirmation", stdout)
	}
}

func TestRun_DatabasesSetNode_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "set-node", "db"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires a database name and a node id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DatabasesClearNode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppNodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(databaseResource{Name: "db"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "clear-node", "db", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/db/node" {
		t.Errorf("request = %s %s, want PUT /api/v1/databases/db/node", gotMethod, gotPath)
	}
	if gotBody.NodeID != "" {
		t.Errorf("request body NodeID = %q, want empty", gotBody.NodeID)
	}
	if !strings.Contains(stdout, `database "db" moved to node "this control plane (local)"`) {
		t.Errorf("stdout = %q, want a local-move confirmation", stdout)
	}
}

func TestRun_DatabasesClearNode_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"databases", "clear-node"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires a database name") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DatabasesSetNode_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "set-node", "-h"})
	if !strings.Contains(stderr, "databases set-node") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_DatabasesClearNode_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "clear-node", "-h"})
	if !strings.Contains(stderr, "databases clear-node") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
