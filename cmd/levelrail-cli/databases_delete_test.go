package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_DatabasesDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "delete", "main", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/databases/main" {
		t.Errorf("request = %s %s, want DELETE /api/v1/databases/main", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `database "main" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_DatabasesDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "delete", "main", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_DatabasesDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"database not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"databases", "delete", "ghost", "--api-url", srv.URL})
	if !strings.Contains(stderr, "database not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_DatabasesDelete_NoName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"databases", "delete"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_DatabasesDelete_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"databases", "delete", "-h"})
	if !strings.Contains(stderr, "databases delete") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

func TestRun_DatabasesDelete_Force(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"databases", "delete", "main", "--force", "--api-url", srv.URL})
	if gotQuery != "force=true" {
		t.Errorf("query = %q, want force=true", gotQuery)
	}
}

func TestRun_DatabasesSetVersion(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte(`{"name":"main","engine":"postgres","version":"16.4"}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"databases", "set-version", "main", "16.4", "--api-url", srv.URL})
	if gotMethod != http.MethodPut || gotPath != "/api/v1/databases/main/version" || !strings.Contains(gotBody, `"version":"16.4"`) {
		t.Errorf("request = %s %s %s", gotMethod, gotPath, gotBody)
	}
	if !strings.Contains(stdout, "16.4") {
		t.Errorf("stdout = %q, want the new version", stdout)
	}
}
