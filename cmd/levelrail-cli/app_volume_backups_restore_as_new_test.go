package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_AppVolumeBackupsRestoreAsNew_CallsAPI is the positive case: the
// request reaches the server with the shape this command is supposed to
// send, no confirmation gate involved, the same reasoning
// runBackupsRestoreAsNew's own test file establishes for the database
// resource kind.
func TestRun_AppVolumeBackupsRestoreAsNew_CallsAPI(t *testing.T) {
	var gotPath string
	var gotBody triggerVolumeCloneRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(volumeCloneRestoreResource{
			ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data",
			NewVolumeName: "clone-web-data-abc123", BackupHistoryID: gotBody.BackupID, Status: "running",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore-as-new", "web", "data", "--backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/apps/web/volumes/data/restore-as-new" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/restore-as-new", gotPath)
	}
	if gotBody.BackupID != "bkh_1" {
		t.Errorf("request body = %+v, want backup_id=bkh_1", gotBody)
	}
	if !strings.Contains(stdout.String(), "vcr_1") {
		t.Errorf("stdout = %q, want the started clone-restore id", stdout.String())
	}
}

func TestRun_AppVolumeBackupsRestoreAsNew_WithExplicitNewVolumeName(t *testing.T) {
	var gotBody triggerVolumeCloneRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(volumeCloneRestoreResource{ID: "vcr_1", NewVolumeName: gotBody.NewVolumeName, Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"app-volume-backups", "restore-as-new", "web", "data",
		"--backup", "bkh_1", "--new-volume-name", "web-data-inspect",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.NewVolumeName != "web-data-inspect" {
		t.Errorf("request body = %+v, want new_volume_name=web-data-inspect", gotBody)
	}
}

func TestRun_AppVolumeBackupsRestoreAsNew_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore-as-new", "web", "data"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_AppVolumeBackupsRestoreAsNew_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore-as-new", "web", "--backup", "bkh_1"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitUsage, stdout.String(), stderr.String())
	}
}

// TestRun_AppVolumeBackupsRestoreAsNew_JSONOutput proves --json prints
// exactly the decoded response and nothing else, the same contract every
// other --json-capable command in this CLI carries.
func TestRun_AppVolumeBackupsRestoreAsNew_JSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(volumeCloneRestoreResource{
			ID: "vcr_1", SourceServiceName: "web", SourceVolumeName: "data",
			NewVolumeName: "clone-web-data-abc123", BackupHistoryID: "bkh_1", Status: "running",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "restore-as-new", "web", "data", "--backup", "bkh_1", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var got2 volumeCloneRestoreResource
	if err := json.Unmarshal(stdout.Bytes(), &got2); err != nil {
		t.Fatalf("decode stdout as JSON: %v (stdout=%q)", err, stdout.String())
	}
	if got2.ID != "vcr_1" || got2.NewVolumeName != "clone-web-data-abc123" {
		t.Errorf("decoded stdout = %+v, want id=vcr_1 new_volume_name=clone-web-data-abc123", got2)
	}
}
