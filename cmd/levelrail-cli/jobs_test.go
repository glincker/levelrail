package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRun_Jobs(t *testing.T) {
	started := time.Date(2026, 8, 15, 3, 0, 0, 0, time.UTC)
	exit := 2
	var created scheduledTaskRequest
	var statusFilter, limitFilter string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		job := scheduledTaskResource{ID: "sct_1", ServiceName: "web", Name: "nightly", Mode: "fresh", Command: []string{"sh", "-c", "echo hi"}, Schedule: "0 3 * * *", Enabled: true, ConcurrencyPolicy: "forbid"}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/jobs":
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(job)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/jobs":
			_ = json.NewEncoder(w).Encode([]scheduledTaskResource{job})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/jobs/nightly":
			_ = json.NewEncoder(w).Encode(job)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/jobs/nightly/run":
			job.RunID = "jr_9"
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(job)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/jobs/nightly/disable":
			job.Enabled = false
			_ = json.NewEncoder(w).Encode(job)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/job-runs":
			statusFilter, limitFilter = r.URL.Query().Get("status"), r.URL.Query().Get("limit")
			_ = json.NewEncoder(w).Encode([]jobRunResource{{ID: "jr_9", TaskID: "sct_1", JobName: "nightly", Status: "failed", Trigger: "schedule", StartedAt: started, Attempts: 3, ExitCode: &exit}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/jobs/nightly/runs":
			_ = json.NewEncoder(w).Encode([]jobRunResource{{ID: "jr_9", Status: "failed", StartedAt: started}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/jobs/nightly/runs/jr_9":
			_ = json.NewEncoder(w).Encode(jobRunResource{ID: "jr_9", Status: "failed", Trigger: "schedule", Output: "boom: exit 2", StartedAt: started, Attempts: 3})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/apps/web/jobs/nightly":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)

	call := func(args ...string) (string, string, int) {
		var stdout, stderr bytes.Buffer
		args = append(append(args[:2:2], "--token", "t", "--api-url", api.URL), args[2:]...)
		code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		return stdout.String(), stderr.String(), code
	}

	out, errOut, code := call("jobs", "create", "web", "nightly", "--schedule", "0 3 * * *", "--timezone", "Europe/Berlin", "--retries", "2", "--timeout", "5m", "--memory", "256", "--cpu", "0.5", "--", "sh", "-c", "echo hi")
	if code != exitOK || !strings.Contains(out, "nightly") {
		t.Fatalf("create = %d %q %q", code, out, errOut)
	}
	if created.Name != "nightly" || created.Schedule != "0 3 * * *" || created.Timezone != "Europe/Berlin" || created.MaxRetries != 2 ||
		created.TimeoutSeconds != 300 || created.MemoryLimitMB != 256 || created.CPULimitMillis != 500 || !created.Enabled ||
		strings.Join(created.Command, "|") != "sh|-c|echo hi" {
		t.Errorf("create request = %+v", created)
	}

	if out, _, code := call("jobs", "list", "web"); code != exitOK || !strings.Contains(out, "nightly") || !strings.Contains(out, "0 3 * * *") {
		t.Errorf("list = %d %q", code, out)
	}
	if out, _, code := call("jobs", "run", "web", "nightly"); code != exitOK || !strings.Contains(out, "jr_9") {
		t.Errorf("run = %d %q", code, out)
	}
	if out, _, code := call("jobs", "disable", "web", "nightly"); code != exitOK || !strings.Contains(out, "disabled") {
		t.Errorf("disable = %d %q", code, out)
	}
	out, _, code = call("jobs", "history", "web", "--status", "failed", "--limit", "5")
	if code != exitOK || !strings.Contains(out, "jr_9") || !strings.Contains(out, "failed") || statusFilter != "failed" || limitFilter != "5" {
		t.Errorf("history = %d %q status=%q limit=%q", code, out, statusFilter, limitFilter)
	}
	if out, _, code := call("jobs", "logs", "web", "nightly"); code != exitOK || !strings.Contains(out, "boom: exit 2") {
		t.Errorf("logs = %d %q", code, out)
	}
	if _, _, code := call("jobs", "delete", "web", "nightly"); code != exitOK {
		t.Errorf("delete code = %d", code)
	}
	if _, _, code := call("jobs", "create", "web", "x", "--schedule", "* * * * *"); code == exitOK {
		t.Error("create without a command succeeded, want a usage error")
	}
}
