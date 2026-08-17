package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsScheduledTasksCreate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/scheduled-tasks" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/scheduled-tasks", r.Method, r.URL.Path)
		}
		var req scheduledTaskRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.Name != "cron" || req.Command != "php artisan schedule:run" || req.Schedule != "* * * * *" || !req.Enabled {
			t.Errorf("request = %+v, unexpected fields", req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1", ServiceName: "web", Name: req.Name, Command: req.Command, Schedule: req.Schedule, Enabled: req.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--api-url", srv.URL, "--name", "cron", "--command", "php artisan schedule:run", "--schedule", "* * * * *"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sct_1") {
		t.Errorf("stdout = %q, want it to mention the created task id", stdout.String())
	}
}

func TestRun_AppsScheduledTasksCreate_Disabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req scheduledTaskRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Enabled {
			t.Errorf("request.Enabled = true, want false when --disabled is passed")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--api-url", srv.URL, "--name", "cron", "--command", "true", "--schedule", "* * * * *", "--disabled"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
}

func TestRun_AppsScheduledTasksCreate_MissingRequiredFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--name is required") {
		t.Errorf("stderr = %q, want a --name validation error", stderr.String())
	}
}

func TestRun_AppsScheduledTasksList_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/scheduled-tasks" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/scheduled-tasks", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]scheduledTaskResource{{ID: "sct_1", Name: "cron", Command: "true", Schedule: "* * * * *", Enabled: true}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sct_1") {
		t.Errorf("stdout = %q, want the task listed", stdout.String())
	}
}

func TestRun_AppsScheduledTasksList_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]scheduledTaskResource{{ID: "sct_1"}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "list", "web", "--api-url", srv.URL, "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), `"id": "sct_1"`) {
		t.Errorf("stdout = %q, want the JSON array", stdout.String())
	}
}

func TestRun_AppsScheduledTasksUpdate_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/apps/web/scheduled-tasks/sct_1" {
			t.Errorf("request = %s %s, want PUT /api/v1/apps/web/scheduled-tasks/sct_1", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "update", "web", "sct_1", "--api-url", srv.URL, "--name", "cron2", "--command", "true", "--schedule", "* * * * *"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
}

func TestRun_AppsScheduledTasksDelete_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/api/v1/apps/web/scheduled-tasks/sct_1" {
			t.Errorf("request = %s %s, want DELETE /api/v1/apps/web/scheduled-tasks/sct_1", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "delete", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "deleted") {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout.String())
	}
}

func TestRun_AppsScheduledTasksRun_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/apps/web/scheduled-tasks/sct_1/run" {
			t.Errorf("request = %s %s, want POST /api/v1/apps/web/scheduled-tasks/sct_1/run", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(scheduledTaskRunResource{ID: "sctr_1", ScheduledTaskID: "sct_1", Status: "running"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "run", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sctr_1") {
		t.Errorf("stdout = %q, want it to mention the started run id", stdout.String())
	}
}

func TestRun_AppsScheduledTasksRun_NotConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`{"error":"scheduled task execution is not configured on this control plane"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "run", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d", got, exitAPIError)
	}
	if !strings.Contains(stderr.String(), "not configured") {
		t.Errorf("stderr = %q, want the server's error message", stderr.String())
	}
}

func TestRun_AppsScheduledTasksRuns_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/scheduled-tasks/sct_1/runs" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/scheduled-tasks/sct_1/runs", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]scheduledTaskRunResource{{ID: "sctr_1", Status: "succeeded", ExitCode: 0}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "runs", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "sctr_1") {
		t.Errorf("stdout = %q, want the run listed", stdout.String())
	}
}

func TestRun_AppsScheduledTasks_NoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}

func TestRun_AppsScheduledTasks_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "scheduled-tasks") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsScheduledTasks_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
