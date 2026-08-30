package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRun_AppsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web" {
		t.Errorf("request = %s %s, want DELETE /api/v1/apps/web", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `app "web" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_AppsDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "delete", "web", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_AppsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "delete", "ghost", "--api-url", srv.URL})
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
