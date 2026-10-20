package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestResolveVolumeRestoreConfirmation is a pure, table-driven test of
// the confirmation gate itself, mirroring TestResolveRestoreConfirmation
// (backups_restore_test.go) for the "<app>/<volume>" composite target.
func TestResolveVolumeRestoreConfirmation(t *testing.T) {
	tests := []struct {
		name        string
		appName     string
		volumeName  string
		confirmFlag string
		stdin       string
		wantErr     bool
	}{
		{name: "flag matches exactly", appName: "web", volumeName: "data", confirmFlag: "web/data", wantErr: false},
		{name: "flag mismatch refuses", appName: "web", volumeName: "data", confirmFlag: "web/cache", wantErr: true},
		{name: "interactive prompt matches", appName: "web", volumeName: "data", confirmFlag: "", stdin: "web/data\n", wantErr: false},
		{name: "interactive prompt mismatch refuses", appName: "web", volumeName: "data", confirmFlag: "", stdin: "not-web/data\n", wantErr: true},
		{name: "interactive prompt empty refuses", appName: "web", volumeName: "data", confirmFlag: "", stdin: "\n", wantErr: true},
		{name: "interactive prompt EOF refuses", appName: "web", volumeName: "data", confirmFlag: "", stdin: "", wantErr: true},
		{name: "case sensitive", appName: "Web", volumeName: "data", confirmFlag: "web/data", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			err := resolveVolumeRestoreConfirmation(tt.appName, tt.volumeName, tt.confirmFlag, strings.NewReader(tt.stdin), &stderr)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveVolumeRestoreConfirmation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRun_AppVolumeBackupsRestore_MismatchedConfirmNeverCallsAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected request to the API: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore", "web", "data", "--backup", "bkh_1", "--confirm", "wrong", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "does not match") {
		t.Errorf("stderr = %q, want a confirmation-mismatch error", stderr.String())
	}
}

func TestRun_AppVolumeBackupsRestore_ConfirmedCallsAPI(t *testing.T) {
	var gotPath string
	var gotBody triggerRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(restoreHistoryResource{ID: "rsh_1", ServiceName: "web", VolumeName: "data", BackupHistoryID: gotBody.BackupID, Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore", "web", "data", "--backup", "bkh_1", "--confirm", "web/data", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/apps/web/volumes/data/restore" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/restore", gotPath)
	}
	if gotBody.BackupID != "bkh_1" {
		t.Errorf("request backup_id = %q, want bkh_1", gotBody.BackupID)
	}
	if !strings.Contains(stdout.String(), "rsh_1") {
		t.Errorf("stdout = %q, want the started restore id", stdout.String())
	}
}

func TestRun_AppVolumeBackupsRestore_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore", "web", "data", "--confirm", "web/data"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_AppVolumeBackupsRestore_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
