package main

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

func TestRun_BackupsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "delete", "main", "--backup", "bkh_1", "--api-url", srv.URL})
	if *gotMethod != http.MethodDelete || *gotPath != "/api/v1/databases/main/backups/bkh_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/databases/main/backups/bkh_1", *gotMethod, *gotPath)
	}
	if !strings.Contains(stdout, "bkh_1") {
		t.Errorf("stdout = %q, want the deleted backup id confirmed", stdout)
	}
}

func TestRun_BackupsDelete_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"backup not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"backups", "delete", "main", "--backup", "bkh_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "backup not found") {
		t.Errorf("stderr = %q, want the server's not-found message", stderr)
	}
}

func TestRun_BackupsDelete_WrongDatabase(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"backup \"bkh_1\" was taken from database \"other\", not \"main\""}`)

	stderr := runCLIExpectAPIError(t, []string{"backups", "delete", "main", "--backup", "bkh_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "was taken from database") {
		t.Errorf("stderr = %q, want the server's ownership-mismatch message", stderr)
	}
}

func TestRun_BackupsDelete_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "delete", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_BackupsDelete_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "delete"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsDelete_JSONOutput(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "delete", "main", "--backup", "bkh_1", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"deleted"`) || !strings.Contains(stdout, "true") {
		t.Errorf("stdout = %q, want {\"deleted\": true}", stdout)
	}
}
