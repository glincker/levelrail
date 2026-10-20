package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsVerify(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(backupVerificationResource{ID: "bkv_1", BackupHistoryID: "bkh_1", Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verify", "main", "--backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/backups/bkh_1/verify" {
		t.Errorf("path = %q, want /api/v1/databases/main/backups/bkh_1/verify", gotPath)
	}
	if !strings.Contains(stdout.String(), "bkv_1") {
		t.Errorf("stdout = %q, want the started verification id", stdout.String())
	}
}

func TestRun_BackupsVerify_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verify", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_BackupsVerify_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verify"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsVerifications(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupVerificationResource{
			{ID: "bkv_1", BackupHistoryID: "bkh_1", Status: "passed", ChecksumMatch: true, SizeMatch: true, FormatValid: true, CheckedBy: "alice", StartedAt: "2026-08-15T00:00:00Z"},
			{ID: "bkv_2", BackupHistoryID: "bkh_1", Status: "failed", ChecksumMatch: false, SizeMatch: true, FormatValid: true, CheckedBy: "bob", StartedAt: "2026-08-16T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verifications", "main", "--backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/databases/main/backups/bkh_1/verifications" {
		t.Errorf("path = %q, want /api/v1/databases/main/backups/bkh_1/verifications", gotPath)
	}
	out := stdout.String()
	if !strings.Contains(out, "bkv_1") || !strings.Contains(out, "bkv_2") {
		t.Errorf("stdout = %q, want both verification ids listed", out)
	}
	if !strings.Contains(out, "FAIL") {
		t.Errorf("stdout = %q, want the failed checksum check rendered as FAIL", out)
	}
}

func TestRun_BackupsVerifications_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]backupVerificationResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verifications", "main", "--backup", "bkh_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "no verifications") {
		t.Errorf("stdout = %q, want the empty-state message", stdout.String())
	}
}

func TestRun_BackupsVerifications_MissingBackupID(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verifications", "main"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--backup is required") {
		t.Errorf("stderr = %q, want a missing --backup error", stderr.String())
	}
}

func TestRun_BackupsVerifications_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "verifications"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
