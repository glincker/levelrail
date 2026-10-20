package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_BackupsScheduleSet(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody setBackupScheduleRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(backupScheduleResource{
			DatabaseName: "main",
			TargetID:     gotBody.TargetID,
			Schedule:     gotBody.Schedule,
			Retain:       gotBody.Retain,
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"backups", "schedule", "set", "main",
		"--target", "tgt_1", "--cron", "0 3 * * *", "--retain", "7",
		"--api-url", srv.URL,
	})

	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/databases/main/backup-schedule" {
		t.Errorf("path = %q, want /api/v1/databases/main/backup-schedule", gotPath)
	}
	if gotBody.TargetID != "tgt_1" || gotBody.Schedule != "0 3 * * *" || gotBody.Retain != 7 {
		t.Errorf("request body = %+v, want target tgt_1, schedule '0 3 * * *', retain 7", gotBody)
	}
	if !strings.Contains(stdout, "0 3 * * *") {
		t.Errorf("stdout = %q, want the saved schedule", stdout)
	}
}

func TestRun_BackupsScheduleSet_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(backupScheduleResource{DatabaseName: "main", TargetID: "tgt_1", Schedule: "0 3 * * *", Retain: 7})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{
		"backups", "schedule", "set", "main",
		"--target", "tgt_1", "--cron", "0 3 * * *",
		"--json", "--api-url", srv.URL,
	})

	var got backupScheduleResource
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if got.DatabaseName != "main" {
		t.Errorf("database_name = %q, want main", got.DatabaseName)
	}
}

func TestRun_BackupsScheduleSet_MissingFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing target",
			args: []string{"backups", "schedule", "set", "main", "--cron", "0 3 * * *"},
			want: "--target is required",
		},
		{
			name: "missing cron",
			args: []string{"backups", "schedule", "set", "main", "--target", "tgt_1"},
			want: "--cron is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", tt.args, &stdout, &stderr, envMap())
			if got != exitValidation {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String()+stderr.String(), tt.want) {
				t.Errorf("output = %q, want %q", stdout.String()+stderr.String(), tt.want)
			}
		})
	}
}

func TestRun_BackupsScheduleSet_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "schedule", "set", "--target", "tgt_1", "--cron", "0 3 * * *"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsScheduleClear(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "schedule", "clear", "main", "--api-url", srv.URL})

	if *gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", *gotMethod)
	}
	if *gotPath != "/api/v1/databases/main/backup-schedule" {
		t.Errorf("path = %q, want /api/v1/databases/main/backup-schedule", *gotPath)
	}
	if !strings.Contains(stdout, "main") {
		t.Errorf("stdout = %q, want the database name mentioned", stdout)
	}
}

func TestRun_BackupsScheduleClear_JSON(t *testing.T) {
	srv, _, _ := newNoContentEchoServer(t)
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"backups", "schedule", "clear", "main", "--api-url", srv.URL, "--json"})

	var got map[string]bool
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not valid JSON: %v (stdout=%q)", err, stdout)
	}
	if !got["cleared"] {
		t.Errorf("stdout = %q, want {\"cleared\": true}", stdout)
	}
}

func TestRun_BackupsScheduleClear_NoName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "schedule", "clear"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_BackupsScheduleSet_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusBadRequest, `{"error":"invalid schedule: bad cron"}`)

	stderr := runCLIExpectAPIError(t, []string{
		"backups", "schedule", "set", "main",
		"--target", "tgt_1", "--cron", "not-a-cron",
		"--api-url", srv.URL,
	})

	if !strings.Contains(stderr, "bad cron") {
		t.Errorf("stderr = %q, want the server's validation error", stderr)
	}
}

func TestRun_BackupsSchedule_Help(t *testing.T) {
	stdout, _ := runCLIExpectOK(t, []string{"backups", "schedule", "-h"})
	if !strings.Contains(stdout, "backups schedule set") {
		t.Errorf("stdout = %q, want usage text", stdout)
	}
}

func TestRun_BackupsSchedule_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"backups", "schedule", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
