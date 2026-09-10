package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsStorage_Set(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appStorageResource{AppName: "web", StorageTargetID: gotBody["storage_target_id"]})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "storage", "set", "web", "--storage-target-id", "target-1", "--api-url", srv.URL})

	if gotMethod != http.MethodPut || gotPath != "/api/v1/apps/web/storage" {
		t.Errorf("method/path = %s %s, want PUT /api/v1/apps/web/storage", gotMethod, gotPath)
	}
	if gotBody["storage_target_id"] != "target-1" {
		t.Errorf("request body = %+v, want storage_target_id=target-1", gotBody)
	}
	if !strings.Contains(stdout, "storage_target_id: target-1") {
		t.Errorf("stdout = %q, want storage_target_id line", stdout)
	}
}

func TestRun_AppsStorage_Set_MissingTargetID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "storage", "set", "web", "--api-url", "http://unused"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires --storage-target-id") {
		t.Errorf("stderr = %q, want a missing-flag usage error", stderr.String())
	}
}

func TestRun_AppsStorage_Set_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"unknown storage_target_id"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "storage", "set", "web", "--storage-target-id", "bogus", "--api-url", srv.URL})
	if !strings.Contains(stderr, "unknown storage_target_id") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_AppsStorage_Clear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "storage", "clear", "web", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/apps/web/storage" {
		t.Errorf("method/path = %s %s, want DELETE /api/v1/apps/web/storage", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `"cleared": true`) && !strings.Contains(stdout, "storage detached") {
		t.Errorf("stdout = %q, want a clear confirmation", stdout)
	}
}

func TestRun_AppsStorage_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"apps", "storage", "-h"})
	if !strings.Contains(stdout, "apps storage set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_AppsStorage_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "storage", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps storage subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
