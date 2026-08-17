package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupHistoryResource{
			{ID: "bkp_1", DatabaseName: "main", TargetID: "tgt_1", Status: "succeeded", StartedAt: "2026-08-15T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "list", "main", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/backups" {
		t.Errorf("path = %q, want /api/v1/databases/main/backups", gotPath)
	}
	if !strings.Contains(stdout.String(), "bkp_1") {
		t.Errorf("stdout = %q, want the backup id listed", stdout.String())
	}
}

func TestRun_BackupsList_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "list"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsTrigger(t *testing.T) {
	var gotPath string
	var gotBody triggerBackupRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(backupHistoryResource{ID: "bkp_2", DatabaseName: "main", TargetID: gotBody.TargetID, Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "trigger", "main", "--target", "tgt_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/backups" {
		t.Errorf("path = %q, want /api/v1/databases/main/backups", gotPath)
	}
	if gotBody.TargetID != "tgt_1" {
		t.Errorf("request target_id = %q, want tgt_1", gotBody.TargetID)
	}
	if !strings.Contains(stdout.String(), "bkp_2") {
		t.Errorf("stdout = %q, want the started backup id", stdout.String())
	}
}

func TestRun_BackupsTrigger_MissingTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "trigger", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Errorf("stderr = %q, want a missing --target error", stderr.String())
	}
}

func TestRun_Backups_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "backups list") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}
