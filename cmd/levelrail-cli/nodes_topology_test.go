package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesTopology(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(networkTopologyResource{
			Zone:        "levelrail.internal",
			MeshEnabled: true,
			Nodes:       []networkTopologyNodeResource{{ID: "node_1", Name: "primary", Status: "online", IsLocal: true}},
			Apps:        []networkTopologyAppResource{{Name: "web", NodeID: "node_1", DNSName: "web.levelrail.internal"}},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "topology", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/network/topology" {
		t.Errorf("request = %s %s, want GET /api/v1/network/topology", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "primary") || !strings.Contains(stdout, "web.levelrail.internal") {
		t.Errorf("stdout = %q, want the node and app listed", stdout)
	}
}

func TestRun_NodesTopology_RejectsArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"nodes", "topology", "extra-arg"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "takes no arguments") {
		t.Errorf("stderr = %q, want a no-arguments error", stderr.String())
	}
}

func TestRun_NodesTraffic(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(networkProxyResource{
			Domains: []networkProxyDomainResource{
				{Domain: "app.example.com", App: "web", NodeID: "node_2", Reachable: false, FixCommand: "levelrail-cli apps set-node web node_1"},
			},
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"nodes", "traffic", "--api-url", srv.URL})
	if gotMethod != http.MethodGet || gotPath != "/api/v1/network/proxy" {
		t.Errorf("request = %s %s, want GET /api/v1/network/proxy", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, "app.example.com") || !strings.Contains(stdout, "set-node") {
		t.Errorf("stdout = %q, want the unreachable domain and its fix command", stdout)
	}
}
