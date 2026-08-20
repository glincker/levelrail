package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsScheduledTasksCreate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody scheduledTaskRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1", ServiceName: "web", Command: gotBody.Command, Schedule: gotBody.Schedule, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--schedule", "*/5 * * * *", "--api-url", srv.URL, "--json", "--", "echo", "hello"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/scheduled-tasks" {
		t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks", gotPath)
	}
	if gotBody.Schedule != "*/5 * * * *" || !gotBody.Enabled {
		t.Errorf("request body = %+v, want schedule=*/5 * * * * enabled=true", gotBody)
	}
	if len(gotBody.Command) != 2 || gotBody.Command[0] != "echo" || gotBody.Command[1] != "hello" {
		t.Errorf("request body Command = %v, want [echo hello]", gotBody.Command)
	}
	if !strings.Contains(stdout.String(), `"id": "sct_1"`) {
		t.Errorf("stdout = %q, want the created task as JSON", stdout.String())
	}
}

func TestRun_AppsScheduledTasksCreate_Disabled(t *testing.T) {
	var gotBody scheduledTaskRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1", ServiceName: "web", Command: gotBody.Command, Schedule: gotBody.Schedule, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--schedule", "0 3 * * *", "--disabled", "--api-url", srv.URL, "--", "echo", "hi"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotBody.Enabled {
		t.Errorf("request body Enabled = true, want false with --disabled")
	}
	if !strings.Contains(stdout.String(), `scheduled task "sct_1" created for app "web"`) {
		t.Errorf("stdout = %q, want a creation confirmation", stdout.String())
	}
}

func TestRun_AppsScheduledTasksCreate_MissingSchedule(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--", "echo", "hi"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--schedule is required") {
		t.Errorf("stderr = %q, want a missing --schedule error", stderr.String())
	}
}

func TestRun_AppsScheduledTasksCreate_MissingCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "create", "web", "--schedule", "*/5 * * * *"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires") {
		t.Errorf("stderr = %q, want a usage error about the missing command", stderr.String())
	}
}

func TestRun_AppsScheduledTasksList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/scheduled-tasks" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/scheduled-tasks", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]scheduledTaskResource{
			{ID: "sct_1", ServiceName: "web", Command: []string{"echo", "hi"}, Schedule: "*/5 * * * *", Enabled: true},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "sct_1") || !strings.Contains(stdout.String(), "echo hi") {
		t.Errorf("stdout = %q, want the task listed with its command", stdout.String())
	}
}

func TestRun_AppsScheduledTasksList_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]scheduledTaskResource{})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "list", "web", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "no scheduled tasks") {
		t.Errorf("stdout = %q, want the empty-list message", stdout.String())
	}
}

func TestRun_AppsScheduledTasksGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/apps/web/scheduled-tasks/sct_1" {
			t.Errorf("request = %s %s, want GET /api/v1/apps/web/scheduled-tasks/sct_1", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{
			ID: "sct_1", ServiceName: "web", Command: []string{"echo", "hi"}, Schedule: "*/5 * * * *", Enabled: true,
			LastRunStatus: "success", LastRunOutput: "hi\n",
		})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "scheduled-tasks", "get", "web", "sct_1", "--api-url", srv.URL})
	if !strings.Contains(stdout, "id:        sct_1") {
		t.Errorf("stdout = %q, want the task id line", stdout)
	}
	if !strings.Contains(stdout, "last run:  never") {
		t.Errorf("stdout = %q, want a 'never' last-run summary when last_run_at is unset", stdout)
	}
}

func TestRun_AppsScheduledTasksGet_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotFound, `{"error":"scheduled task not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "scheduled-tasks", "get", "web", "sct_missing", "--api-url", srv.URL})
	if !strings.Contains(stderr, "scheduled task not found") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsScheduledTasksUpdate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody scheduledTaskRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1", ServiceName: "web", Command: gotBody.Command, Schedule: gotBody.Schedule, Enabled: gotBody.Enabled})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "update", "web", "sct_1", "--schedule", "0 0 * * *", "--api-url", srv.URL, "--", "echo", "updated"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/scheduled-tasks/sct_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks/sct_1", gotPath)
	}
	if gotBody.Schedule != "0 0 * * *" {
		t.Errorf("request body Schedule = %q, want 0 0 * * *", gotBody.Schedule)
	}
	if len(gotBody.Command) != 2 || gotBody.Command[1] != "updated" {
		t.Errorf("request body Command = %v, want [echo updated]", gotBody.Command)
	}
	if !strings.Contains(stdout.String(), `scheduled task "sct_1" updated`) {
		t.Errorf("stdout = %q, want an update confirmation", stdout.String())
	}
}

func TestRun_AppsScheduledTasksDelete(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "delete", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/scheduled-tasks/sct_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks/sct_1", gotPath)
	}
	if !strings.Contains(stdout.String(), `scheduled task "sct_1" deleted`) {
		t.Errorf("stdout = %q, want a deletion confirmation", stdout.String())
	}
}

func TestRun_AppsScheduledTasksRun(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(scheduledTaskResource{ID: "sct_1", ServiceName: "web", Command: []string{"echo", "hi"}, Schedule: "*/5 * * * *", Enabled: true})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "run", "web", "sct_1", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/scheduled-tasks/sct_1/run" {
		t.Errorf("path = %q, want /api/v1/apps/web/scheduled-tasks/sct_1/run", gotPath)
	}
	if !strings.Contains(stdout.String(), "run started for scheduled task") {
		t.Errorf("stdout = %q, want a run-started confirmation", stdout.String())
	}
}

func TestRun_AppsScheduledTasksRun_NotConfigured(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusNotImplemented, `{"error":"scheduled task execution is not configured on this control plane"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "scheduled-tasks", "run", "web", "sct_1", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not configured") {
		t.Errorf("stderr = %q, want the server's error message", stderr)
	}
}

func TestRun_AppsScheduledTasks_NoAppName(t *testing.T) {
	assertUsageErrorMissingName(t, "scheduled-tasks", []string{"list"})
}

func TestRun_AppsScheduledTasks_MissingTaskID(t *testing.T) {
	tests := []string{"get", "delete", "run"}
	for _, verb := range tests {
		t.Run(verb, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", verb, "web"}, &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "requires") {
				t.Errorf("stderr = %q, want a usage error", stderr.String())
			}
		})
	}
}

func TestRun_AppsScheduledTasks_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "apps scheduled-tasks") {
		t.Errorf("stdout = %q, want usage text", stdout.String())
	}
}

func TestRun_AppsScheduledTasks_UnknownVerb(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "scheduled-tasks", "frobnicate"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown apps scheduled-tasks subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
