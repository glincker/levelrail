package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppsStreamsCreate exercises "apps streams create" end to end
// against a fake control plane, the same run()-plus-httptest.Server
// shape TestRun_AppsRestart already establishes.
func TestRun_AppsStreamsCreate(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(appStreamResource{
			ID: "stream_abc", App: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: "tcp", CreatedAt: "2026-10-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "create", "pg", "--container-port", "5433", "--host-port", "15432", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/pg/streams" {
		t.Errorf("path = %q, want /api/v1/apps/pg/streams", gotPath)
	}
	if !strings.Contains(gotBody, `"container_port":5433`) || !strings.Contains(gotBody, `"host_port":15432`) {
		t.Errorf("request body = %q, want container_port and host_port", gotBody)
	}
	if !strings.Contains(stdout.String(), `"id": "stream_abc"`) {
		t.Errorf("stdout = %q, want the created stream as JSON", stdout.String())
	}
}

func TestRun_AppsStreamsCreate_Human(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(appStreamResource{
			ID: "stream_abc", App: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: "tcp", CreatedAt: "2026-10-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "create", "pg", "--container-port", "5433", "--host-port", "15432", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stream_abc") || !strings.Contains(stdout.String(), "15432") {
		t.Errorf("stdout = %q, want a human summary naming the stream and host port", stdout.String())
	}
}

func TestRun_AppsStreamsCreate_MissingFlags(t *testing.T) {
	tests := [][]string{
		{"apps", "streams", "create", "pg", "--host-port", "15432"},
		{"apps", "streams", "create", "pg", "--container-port", "5433"},
	}
	for _, args := range tests {
		var stdout, stderr bytes.Buffer
		got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		if got != exitValidation {
			t.Errorf("args %v: exit = %d, want %d", args, got, exitValidation)
		}
	}
}

func TestRun_AppsStreamsList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/pg/streams" || r.Method != http.MethodGet {
			t.Errorf("request = %s %s, want GET /api/v1/apps/pg/streams", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]appStreamResource{
			{ID: "stream_abc", App: "pg", ContainerPort: 5433, HostPort: 15432, Protocol: "tcp", CreatedAt: "2026-10-01T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "list", "pg", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stream_abc") {
		t.Errorf("stdout = %q, want the listed stream", stdout.String())
	}
}

func TestRun_AppsStreamsDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "delete", "pg", "stream_abc", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/pg/streams/stream_abc" {
		t.Errorf("path = %q, want /api/v1/apps/pg/streams/stream_abc", gotPath)
	}
	if !strings.Contains(stdout.String(), "removed") {
		t.Errorf("stdout = %q, want a removed confirmation", stdout.String())
	}
}

func TestRun_AppsStreamsDelete_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"app stream not found"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "delete", "pg", "stream_ghost", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitAPIError, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "app stream not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsStreamsDelete_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "delete", "pg"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires an app name and a stream id") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_AppsStreams_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps streams") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsStreams_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "streams", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps streams subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
