package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun_AppsProjectsTopology(t *testing.T) {
	var gotPath string
	label := "healthy"
	srv := newListEchoServer(t, &gotPath, projectTopologyResource{
		Nodes: []projectTopologyNodeResource{
			{ID: "app:web", Kind: "app", Label: "web", Status: &projectTopologyNodeStatus{Label: label, Variant: "success"}},
			{ID: "database:main", Kind: "database", Label: "main"},
		},
		Edges: []projectTopologyEdgeResource{{From: "app:web", To: "database:main", Kind: "database_binding"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "projects", "topology", "proj_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/projects/proj_1/topology" {
		t.Errorf("path = %q, want /api/v1/projects/proj_1/topology", gotPath)
	}
	if !strings.Contains(stdout, "app:web") || !strings.Contains(stdout, "database:main") {
		t.Errorf("stdout = %q, want both nodes listed", stdout)
	}
	if !strings.Contains(stdout, "database_binding") {
		t.Errorf("stdout = %q, want the edge kind listed", stdout)
	}
}

func TestRun_AppsProjectsTopology_NoID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "projects", "topology"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "project id") {
		t.Errorf("stderr = %q, want a missing project id error", stderr.String())
	}
}
