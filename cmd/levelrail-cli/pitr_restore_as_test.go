package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_PITRRestoreAs_SendsNewNameWithoutConfirmation(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"pitr_1","database_name":"main","status":"running"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	args := []string{"pitr", "restore", "main", "--base-backup", "bb_1", "--target-time", "2026-09-01T00:00:00Z", "--restore-as", "main-copy", "--api-url", srv.URL}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code != exitOK {
		t.Fatalf("exit = %d (stdout=%q stderr=%q)", code, stdout.String(), stderr.String())
	}
	if got["restore_as"] != "main-copy" || got["base_backup_id"] != "bb_1" {
		t.Errorf("request body = %v", got)
	}
	if strings.Contains(stderr.String(), "Type") {
		t.Errorf("a restore into a new database asked for confirmation: %q", stderr.String())
	}
}

func TestRun_PITRRestore_WithoutRestoreAsStillRequiresConfirmation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("the API must not be called without a confirmation")
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	args := []string{"pitr", "restore", "main", "--base-backup", "bb_1", "--target-time", "2026-09-01T00:00:00Z", "--confirm", "other", "--api-url", srv.URL}
	if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code == exitOK {
		t.Fatal("a mismatched confirmation must fail")
	}
}
