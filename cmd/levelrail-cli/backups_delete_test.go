package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestRun_BackupsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "delete", "mydb", "bkh_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/databases/mydb/backups/bkh_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/databases/mydb/backups/bkh_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, `backup "bkh_1" of database "mydb" deleted`) {
		t.Errorf("stdout = %q, want a deleted confirmation", stdout)
	}
}

func TestRun_BackupsDelete_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "delete", "mydb", "bkh_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted": true`) {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}

func TestRun_BackupsDelete_StillRunning(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusConflict, `{"error":"backup \"bkh_1\" has status \"running\", cannot delete a backup that is still running"}`)

	stderr := runCLIExpectAPIError(t, []string{"backups", "delete", "mydb", "bkh_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "still running") {
		t.Errorf("stderr = %q, want the server's own conflict message", stderr)
	}
}

func TestRun_BackupsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"backup not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"backups", "delete", "mydb", "bkh_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_BackupsDelete_MissingArgs(t *testing.T) {
	tests := [][]string{
		{"backups", "delete"},
		{"backups", "delete", "mydb"},
	}
	for _, args := range tests {
		var stdout, stderr strings.Builder
		got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		if got != exitUsage {
			t.Fatalf("args=%v exit = %d, want %d", args, got, exitUsage)
		}
		if !strings.Contains(stderr.String(), "requires a database name and a backup id") {
			t.Errorf("args=%v stderr = %q, want a missing-args usage error", args, stderr.String())
		}
	}
}

func TestRun_BackupsDelete_Help(t *testing.T) {
	_, stderr := runCLIExpectOK(t, []string{"backups", "delete", "-h"})
	if !strings.Contains(stderr, "backups delete") {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}
