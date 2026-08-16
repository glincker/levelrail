package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveRestoreConfirmation is a pure, table-driven test of the
// confirmation gate itself: no HTTP, no flag parsing, exactly the
// "structure the destructive check as its own testable function"
// resolveRestoreConfirmation's own doc comment describes.
func TestResolveRestoreConfirmation(t *testing.T) {
	tests := []struct {
		name        string
		dbName      string
		confirmFlag string
		stdin       string
		wantErr     bool
	}{
		{name: "flag matches exactly", dbName: "main", confirmFlag: "main", wantErr: false},
		{name: "flag mismatch refuses", dbName: "main", confirmFlag: "mian", wantErr: true},
		{name: "interactive prompt matches", dbName: "main", confirmFlag: "", stdin: "main\n", wantErr: false},
		{name: "interactive prompt mismatch refuses", dbName: "main", confirmFlag: "", stdin: "not-main\n", wantErr: true},
		{name: "interactive prompt empty refuses", dbName: "main", confirmFlag: "", stdin: "\n", wantErr: true},
		{name: "interactive prompt EOF refuses", dbName: "main", confirmFlag: "", stdin: "", wantErr: true},
		{name: "case sensitive", dbName: "Main", confirmFlag: "main", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			err := resolveRestoreConfirmation(tt.dbName, tt.confirmFlag, strings.NewReader(tt.stdin), &stderr)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveRestoreConfirmation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestRun_BackupsRestore_MismatchedConfirmNeverCallsAPI is the
// integration-level version of the same requirement: a wrong --confirm
// value must refuse before ever reaching the network. The fake server
// fails the test outright if it receives any request at all.
func TestRun_BackupsRestore_MismatchedConfirmNeverCallsAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to the API: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore", "main", "--backup", "bkp_1", "--confirm", "wrong-name", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "does not match") {
		t.Errorf("stderr = %q, want a confirmation-mismatch error", stderr.String())
	}
}

// TestRun_BackupsRestore_MissingConfirmPromptsAndRefusesOnEOF: no
// --confirm given and no input available (stdin at EOF) must also
// refuse before calling the API, not hang waiting for a real terminal.
func TestRun_BackupsRestore_MissingConfirmPromptsAndRefusesOnEOF(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to the API: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := runBackupsRestore("levelrail-cli-test", []string{"main", "--backup", "bkp_1", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil), strings.NewReader(""))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
}

// TestRun_BackupsRestore_ConfirmedCallsAPI is the positive case: a
// matching --confirm does reach the server, with the request body this
// command is supposed to send.
func TestRun_BackupsRestore_ConfirmedCallsAPI(t *testing.T) {
	var gotPath string
	var gotBody triggerRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(restoreHistoryResource{ID: "rsh_1", DatabaseName: "main", BackupHistoryID: gotBody.BackupID, Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore", "main", "--backup", "bkp_1", "--confirm", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/restore" {
		t.Errorf("path = %q, want /api/v1/databases/main/restore", gotPath)
	}
	if gotBody.BackupID != "bkp_1" {
		t.Errorf("request backup_id = %q, want bkp_1", gotBody.BackupID)
	}
	if !strings.Contains(stdout.String(), "rsh_1") {
		t.Errorf("stdout = %q, want the started restore id", stdout.String())
	}
}

func TestRun_BackupsRestore_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore", "main", "--confirm", "main"}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_BackupsRestore_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore"}, &stdout, &stderr, envMap(nil))
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
