package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsDelete(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The undo hint lists recently deleted apps after the delete, so record only the DELETE.
		if r.Method == http.MethodDelete {
			gotPath, gotMethod = r.URL.Path, r.Method
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL, "--yes"})
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/apps/web" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web", gotMethod, gotPath)
	}
	if !strings.Contains(stdout, `app "web" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_AppsDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL, "--json", "--yes"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_AppsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "delete", "ghost", "--api-url", srv.URL, "--yes"})
	if !strings.Contains(stderr, "app not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsDelete_NoName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "delete"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsDelete_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"apps", "delete", "-h"})
	if !strings.Contains(stderr, "apps delete") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_AppsDelete_TeardownPending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"teardown_pending","message":"m","error":"node offline"}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL, "--yes"})
	if !strings.Contains(stdout, "not removed yet") || !strings.Contains(stdout, "node offline") {
		t.Errorf("stdout = %q, want a pending-teardown message with the cause", stdout)
	}
}

func TestRun_AppsDelete_NeedsYesWithoutTerminal(t *testing.T) {
	srv, gotPath, _ := newNoContentEchoServer(t)
	defer srv.Close()

	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "delete", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "pass --yes") || *gotPath != "" {
		t.Errorf("stderr = %q, path = %q, want a fail-fast naming --yes and no request", stderr.String(), *gotPath)
	}
}

func TestRun_AppsDelete_PrintsUndoHint(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{{"kind": "app", "name": "web", "expires_at": expires}})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	_, stderr := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL, "--yes"})
	if !strings.Contains(stderr, "undo: levelrail-cli-test apps restore web") || !strings.Contains(stderr, "until 2030-01-02 03:04") {
		t.Errorf("stderr = %q, want the restore command and the undo deadline", stderr)
	}
}
