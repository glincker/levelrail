package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRun_AppsSetNode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppNodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web", NodeID: gotBody.NodeID})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set-node", "web", "node_1", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/node" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/node", gotMethod, gotPath)
	}
	if gotBody.NodeID != "node_1" {
		t.Errorf("request body NodeID = %q, want node_1", gotBody.NodeID)
	}
	if !strings.Contains(stdout, `app "web" moved to node "node_1"`) {
		t.Errorf("stdout = %q, want a move confirmation", stdout)
	}
}

func TestRun_AppsSetNode_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "set-node", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and a node id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsClearNode(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setAppNodeRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appResource{Name: "web"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "clear-node", "web", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/node" {
		t.Errorf("request = %s %s, want PUT /api/v1/apps/web/node", gotMethod, gotPath)
	}
	if gotBody.NodeID != "" {
		t.Errorf("request body NodeID = %q, want empty", gotBody.NodeID)
	}
	if !strings.Contains(stdout, `app "web" moved to node "this control plane (local)"`) {
		t.Errorf("stdout = %q, want a local-move confirmation", stdout)
	}
}

// TestRun_AppsSetNode_WithVolumes_Immediate covers the server returning
// 200 with an already-succeeded move record (the no-volumes/same-node
// shortcut server-side): the CLI must not poll in that case, one request
// is enough.
func TestRun_AppsSetNode_WithVolumes_Immediate(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appVolumeMoveResource{
			ID: "avm_1", ServiceName: "web", ToNodeID: "node_1", Status: "succeeded",
			Steps: []appVolumeMoveStepResource{{Name: "update_placement", Status: "succeeded"}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set-node", "web", "node_1", "--with-volumes", "--api-url", srv.URL})
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want exactly 1 (no polling for an already-finished move)", got)
	}
	if !strings.Contains(stdout, `app "web" moved to node "node_1", volumes included`) {
		t.Errorf("stdout = %q, want a volumes-included move confirmation", stdout)
	}
}

// TestRun_AppsSetNode_WithVolumes_PollsUntilDone proves the CLI polls
// GET .../moves/{id} while the trigger response is still "running", and
// stops once it flips to "succeeded".
func TestRun_AppsSetNode_WithVolumes_PollsUntilDone(t *testing.T) {
	var pollCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(appVolumeMoveResource{
				ID: "avm_1", ServiceName: "web", ToNodeID: "node_1", Status: "running", Steps: []appVolumeMoveStepResource{},
			})
			return
		}
		n := atomic.AddInt32(&pollCalls, 1)
		status := "running"
		if n >= 2 {
			status = "succeeded"
		}
		_ = json.NewEncoder(w).Encode(appVolumeMoveResource{
			ID: "avm_1", ServiceName: "web", ToNodeID: "node_1", Status: status,
			Steps: []appVolumeMoveStepResource{{Name: "move_volume:app-web-data", Status: status}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "set-node", "web", "node_1", "--with-volumes", "--api-url", srv.URL})
	if got := atomic.LoadInt32(&pollCalls); got < 2 {
		t.Errorf("poll calls = %d, want at least 2 (kept polling until succeeded)", got)
	}
	if !strings.Contains(stdout, `app "web" moved to node "node_1", volumes included`) {
		t.Errorf("stdout = %q, want a volumes-included move confirmation", stdout)
	}
}

// TestRun_AppsSetNode_WithVolumes_Failure proves a failed move is reported
// as an API-shaped error, not treated as success.
func TestRun_AppsSetNode_WithVolumes_Failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appVolumeMoveResource{
			ID: "avm_1", ServiceName: "web", ToNodeID: "node_1", Status: "failed", Error: "docker daemon unreachable",
			Steps: []appVolumeMoveStepResource{
				{Name: "stop_app", Status: "succeeded"},
				{Name: "move_volume:app-web-data", Status: "failed", Error: "docker daemon unreachable"},
			},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "set-node", "web", "node_1", "--with-volumes", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = %d, want a non-zero exit for a failed move (stderr=%q)", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "docker daemon unreachable") {
		t.Errorf("stderr = %q, want the move's own failure reason", stderr.String())
	}
}
