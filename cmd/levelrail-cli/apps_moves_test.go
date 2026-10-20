package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_AppsMoves_List(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []appVolumeMoveResource{
		{ID: "avm_1", ServiceName: "web", FromNodeID: "node-1", ToNodeID: "node-2", Status: "succeeded", StartedAt: "2026-09-20T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "moves", "list", "web", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web/moves" {
		t.Errorf("path = %s, want /api/v1/apps/web/moves", gotPath)
	}
	if !strings.Contains(stdout, "avm_1") || !strings.Contains(stdout, "node-2") {
		t.Errorf("stdout = %q, want the move id and destination node", stdout)
	}
}

func TestRun_AppsMoves_List_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []appVolumeMoveResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "moves", "list", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no moves") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

func TestRun_AppsMoves_Get(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, appVolumeMoveResource{
		ID: "avm_1", ServiceName: "web", FromNodeID: "node-1", ToNodeID: "node-2", Status: "succeeded",
		Steps: []appVolumeMoveStepResource{{Name: "stop_app", Status: "succeeded"}},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "moves", "get", "web", "avm_1", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web/moves/avm_1" {
		t.Errorf("path = %s, want /api/v1/apps/web/moves/avm_1", gotPath)
	}
	if !strings.Contains(stdout, "avm_1") || !strings.Contains(stdout, "stop_app") {
		t.Errorf("stdout = %q, want the move id and step name", stdout)
	}
}

func TestRun_AppsMoves_Get_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"move not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "moves", "get", "web", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "move not found") {
		t.Errorf("stderr = %q, want the server's error", stderr)
	}
}

func TestRun_AppsMoves_Get_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "moves", "get", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an app name and a move id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsMoves_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "moves", "-h"})
	if !strings.Contains(stdout, "apps moves list") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsMoves_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "moves", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps moves subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
