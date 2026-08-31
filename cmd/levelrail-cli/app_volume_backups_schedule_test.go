package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppVolumeBackupsScheduleSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody setVolumeBackupScheduleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(volumeBackupScheduleResource{
			ServiceName: "web", VolumeName: "data",
			TargetID: gotBody.TargetID, Schedule: gotBody.Schedule, Retain: gotBody.Retain,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"app-volume-backups", "schedule", "set", "web", "data",
		"--target", "tgt_1", "--cron", "0 3 * * *", "--retain", "7",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/volumes/data/backup-schedule" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/backup-schedule", gotPath)
	}
	if gotBody.TargetID != "tgt_1" || gotBody.Schedule != "0 3 * * *" || gotBody.Retain != 7 {
		t.Errorf("request body = %+v, want target tgt_1, schedule '0 3 * * *', retain 7", gotBody)
	}
	if !strings.Contains(stdout, "0 3 * * *") {
		t.Errorf("stdout = %q, want the saved schedule", stdout)
	}
}

func TestRun_AppVolumeBackupsScheduleClear(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"app-volume-backups", "schedule", "clear", "web", "data", "--api-url", srv.URL})

	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/volumes/data/backup-schedule" {
		t.Errorf("path = %q, want /api/v1/apps/web/volumes/data/backup-schedule", gotPath)
	}
	if !strings.Contains(stdout, "removed") {
		t.Errorf("stdout = %q, want a removal confirmation", stdout)
	}
}

func TestRun_AppVolumeBackupsScheduleSet_MissingTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"app-volume-backups", "schedule", "set", "web", "data", "--cron", "0 3 * * *"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--target is required") {
		t.Errorf("stderr = %q, want a missing --target error", stderr.String())
	}
}
