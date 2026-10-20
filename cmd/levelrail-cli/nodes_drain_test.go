package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_NodesDrain(t *testing.T) {
	var gotMethod, gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.NodeDrainResource{ID: "dr1", NodeID: "nd_1", TargetNodeID: "nd_2", Status: "running"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "drain", "nd_1", "--target", "nd_2", "--api-url", srv.URL})
	if gotMethod != http.MethodPost || gotPath != "/api/v1/nodes/nd_1/drain" || gotQuery != "target_node_id=nd_2" {
		t.Errorf("request = %s %s?%s", gotMethod, gotPath, gotQuery)
	}
	if !strings.Contains(stdout, "drain dr1 of node nd_1: running") || !strings.Contains(stdout, "nodes drains nd_1") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_NodesDrain_WaitFailureExitsNonZero(t *testing.T) {
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		d := apiclient.NodeDrainResource{ID: "dr1", NodeID: "nd_1", Status: "running"}
		if r.Method == http.MethodGet {
			if polls.Add(1) > 1 {
				d.Status, d.Error = "failed", "moving database main failed: not ready"
				d.Moves = []apiclient.NodeMoveResource{{Kind: "database", Resource: "main", ToNodeID: "nd_2", Status: "rolled_back", Phase: "done"}}
			}
		}
		_ = json.NewEncoder(w).Encode(d)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes", "drain", "nd_1", "--wait", "--poll-interval", "1ms", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d; stdout %s", got, exitAPIError, stdout.String())
	}
	if !strings.Contains(stdout.String(), "rolled_back") || !strings.Contains(stdout.String(), "moving database main failed") {
		t.Errorf("stdout = %q", stdout.String())
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

func TestRun_NodesMove(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotPath, gotBody = r.URL.Path, buf.String()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(apiclient.NodeMoveResource{ID: "mv1", Kind: "database", Resource: "main", ToNodeID: "nd_2", Status: "running", Phase: "preflight"})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "move", "db", "main", "--to", "nd_2", "--api-url", srv.URL})
	if gotPath != "/api/v1/databases/main/move" || !strings.Contains(gotBody, `"node_id":"nd_2"`) {
		t.Errorf("request = %s %s", gotPath, gotBody)
	}
	if !strings.Contains(stdout, "move mv1") {
		t.Errorf("stdout = %q", stdout)
	}
	var stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"nodes", "move", "volume", "x", "--api-url", srv.URL}, &bytes.Buffer{}, &stderr, envMap()); code != exitUsage {
		t.Errorf("bad kind exit = %d, want usage", code)
	}
}

func TestRun_NodesAdd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req apiclient.CreateNodeJoinTokenRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(apiclient.CreateNodeJoinTokenResponse{Token: "tok", JoinCommands: &apiclient.JoinCommands{
			Docker: "docker run ... -e APP_NODE_NAME=" + req.NodeName, Binary: "sudo ...",
		}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "add", "--name", "w2", "--mesh", "--api-url", srv.URL})
	if !strings.Contains(stdout, "APP_NODE_NAME=w2") || !strings.Contains(stdout, "single use") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRun_NodesLabels(t *testing.T) {
	var gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(bytes.Buffer)
		_, _ = buf.ReadFrom(r.Body)
		gotMethod, gotBody = r.Method, buf.String()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.NodeLabels{Labels: map[string]string{"disk": "ssd"}})
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"nodes", "labels", "nd_1", "disk=ssd", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || !strings.Contains(gotBody, `"disk":"ssd"`) || !strings.Contains(stdout, "disk=ssd") {
		t.Errorf("%s %s -> %q", gotMethod, gotBody, stdout)
	}
	var stderr bytes.Buffer
	if code := run("levelrail-cli-test", []string{"nodes", "labels", "nd_1", "Bad=Key", "--api-url", srv.URL}, &bytes.Buffer{}, &stderr, envMap()); code != exitUsage {
		t.Errorf("invalid label exit = %d, want usage", code)
	}
}
