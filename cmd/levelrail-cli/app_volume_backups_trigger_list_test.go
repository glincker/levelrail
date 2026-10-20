package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppVolumeBackupsTrigger(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody triggerBackupRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(backupHistoryResource{ID: "bkh_1", ServiceName: "web", VolumeName: "data", TargetID: gotBody.TargetID, Status: "running"})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "trigger", "web", "data", "--target", "tgt_1", "--api-url", srv.URL})

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/volumes/data/backups" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/backups", gotPath)
	}
	if gotBody.TargetID != "tgt_1" {
		t.Errorf("request target_id = %q, want tgt_1", gotBody.TargetID)
	}
	if !strings.Contains(stdout, "bkh_1") {
		t.Errorf("stdout = %q, want the started backup id", stdout)
	}
}

func TestRun_AppVolumeBackupsTrigger_MissingTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "trigger", "web", "data"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Errorf("stderr = %q, want a missing --target error", stderr.String())
	}
}

func TestRun_AppVolumeBackupsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []backupHistoryResource{
		{ID: "bkh_1", ServiceName: "web", VolumeName: "data", TargetID: "tgt_1", Status: "succeeded"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "list", "web", "data", "--api-url", srv.URL})

	if gotPath != "/api/v1/apps/web/volumes/data/backups" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/backups", gotPath)
	}
	if !strings.Contains(stdout, "bkh_1") {
		t.Errorf("stdout = %q, want the backup id in the table", stdout)
	}
}
