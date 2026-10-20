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

func TestJobsNext(t *testing.T) {
	from := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		schedule  string
		tz        string
		count     int
		wantLines []string
		wantCode  int
	}{
		{"hourly shortcut", "@hourly", "", 2, []string{"2026-03-07T13:00:00Z", "2026-03-07T14:00:00Z"}, exitOK},
		{"daily in a zone", "0 9 * * *", "America/New_York", 1, []string{"2026-03-07T09:00:00-05:00"}, exitOK},
		{"dst spring forward keeps wall time", "30 2 * * *", "America/New_York", 2, []string{"2026-03-08T03:30:00-04:00", "2026-03-09T02:30:00-04:00"}, exitOK},
		{"bad cron", "nope", "", 5, nil, exitUsage},
		{"bad zone", "@daily", "Mars/Base", 5, nil, exitUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			code := jobsNext(tc.schedule, tc.tz, tc.count, from, &out, &errb)
			if code != tc.wantCode {
				t.Fatalf("code = %d (%s), want %d", code, errb.String(), tc.wantCode)
			}
			if tc.wantLines == nil {
				return
			}
			if got := strings.Fields(out.String()); strings.Join(got, ",") != strings.Join(tc.wantLines, ",") {
				t.Errorf("lines = %v, want %v", got, tc.wantLines)
			}
		})
	}
}

func TestRun_JobsCreateHTTPAndPause(t *testing.T) {
	var created scheduledTaskRequest
	var pausedPath string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		job := scheduledTaskResource{ID: "sct_1", ServiceName: "web", Name: "tick", Mode: "http", Schedule: "@hourly", Enabled: true}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/jobs":
			_ = json.NewDecoder(r.Body).Decode(&created)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(job)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/disable"):
			pausedPath = r.URL.Path
			job.Enabled = false
			_ = json.NewEncoder(w).Encode(job)
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	var out, errb bytes.Buffer
	args := []string{"create", "web", "tick", "--api-url", api.URL, "--token", "t", "--mode", "http", "--schedule", "@hourly", "--http-path", "/cron/tick", "--background", "--concurrency", "queue"}
	if code := runJobs("levelrail-cli", args, &out, &errb, func(string) (string, bool) { return "", false }); code != exitOK {
		t.Fatalf("create code = %d: %s", code, errb.String())
	}
	if created.Mode != "http" || created.HTTP == nil || created.HTTP.Path != "/cron/tick" || !created.HTTP.Background || created.ConcurrencyPolicy != "queue" {
		t.Errorf("request = %+v http %+v, want http mode, path, background and queue", created, created.HTTP)
	}

	out.Reset()
	args = []string{"pause", "web", "tick", "--api-url", api.URL, "--token", "t"}
	if code := runJobs("levelrail-cli", args, &out, &errb, func(string) (string, bool) { return "", false }); code != exitOK {
		t.Fatalf("pause code = %d: %s", code, errb.String())
	}
	if pausedPath != "/api/v1/apps/web/jobs/tick/disable" || !strings.Contains(out.String(), "paused") {
		t.Errorf("pause hit %q and printed %q", pausedPath, out.String())
	}

	if code := runJobs("levelrail-cli", []string{"create", "web", "x", "--api-url", api.URL, "--token", "t", "--mode", "http", "--schedule", "@daily"}, &out, &errb, func(string) (string, bool) { return "", false }); code == exitOK {
		t.Error("http create without --http-path succeeded, want a validation error")
	}
}
