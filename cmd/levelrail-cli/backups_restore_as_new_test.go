package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRun_BackupsRestoreAsNew_CallsAPI is the positive case: the request
// reaches the server with the shape this command is supposed to send, no
// confirmation gate involved (unlike backups restore, see
// backups_restore_as_new.go's own doc comment for why).
func TestRun_BackupsRestoreAsNew_CallsAPI(t *testing.T) {
	var gotPath string
	var gotBody triggerCloneRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneRestoreResource{
			ID: "clr_1", SourceDatabaseName: "main", NewDatabaseName: gotBody.NewName,
			BackupHistoryID: gotBody.BackupID, Status: "running",
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore-as-new", "main", "--backup", "bkh_1", "--new-name", "main-staging", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/restore-as-new" {
		t.Errorf("path = %q, want /api/v1/databases/main/restore-as-new", gotPath)
	}
	if gotBody.BackupID != "bkh_1" || gotBody.NewName != "main-staging" {
		t.Errorf("request body = %+v, want backup_id=bkh_1 new_name=main-staging", gotBody)
	}
	if !strings.Contains(stdout.String(), "clr_1") {
		t.Errorf("stdout = %q, want the started clone-restore id", stdout.String())
	}
}

func TestRun_BackupsRestoreAsNew_WithVersionAndProject(t *testing.T) {
	var gotBody triggerCloneRestoreRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneRestoreResource{ID: "clr_1", Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"backups", "restore-as-new", "main",
		"--backup", "bkh_1", "--new-name", "main-staging",
		"--version", "17", "--project", "prj_1",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Version != "17" || gotBody.ProjectID != "prj_1" {
		t.Errorf("request body = %+v, want version=17 project_id=prj_1", gotBody)
	}
}

func TestRun_BackupsRestoreAsNew_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore-as-new", "main", "--new-name", "main-staging"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_BackupsRestoreAsNew_MissingNewName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore-as-new", "main", "--backup", "bkh_1"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--new-name is required") {
		t.Errorf("stderr = %q, want a missing --new-name error", stderr.String())
	}
}

func TestRun_BackupsRestoreAsNew_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore-as-new"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

// TestRun_BackupsRestoreAsNew_JSONOutput proves --json prints exactly the
// decoded response and nothing else, the same contract every other
// --json-capable command in this CLI carries.
func TestRun_BackupsRestoreAsNew_JSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(cloneRestoreResource{ID: "clr_1", SourceDatabaseName: "main", NewDatabaseName: "main-staging", BackupHistoryID: "bkh_1", Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "restore-as-new", "main", "--backup", "bkh_1", "--new-name", "main-staging", "--json", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	var got2 cloneRestoreResource
	if err := json.Unmarshal(stdout.Bytes(), &got2); err != nil {
		t.Fatalf("decode stdout as JSON: %v (stdout=%q)", err, stdout.String())
	}
	if got2.ID != "clr_1" || got2.NewDatabaseName != "main-staging" {
		t.Errorf("decoded stdout = %+v, want id=clr_1 new_database_name=main-staging", got2)
	}
}
