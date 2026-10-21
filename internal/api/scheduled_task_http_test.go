package api

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestValidateScheduledTaskInput_HTTPMode(t *testing.T) {
	tests := []struct {
		name       string
		res        scheduledTaskResource
		wantErr    string
		wantMethod string
	}{
		{"defaults method to POST", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@hourly", HTTP: &jobHTTPResource{Path: "/cron"}}, "", "POST"},
		{"keeps GET", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "*/5 * * * *", HTTP: &jobHTTPResource{Method: "get", Path: "/r"}}, "", "GET"},
		{"missing http block", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@daily"}, "http.path is required", ""},
		{"relative path", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@daily", HTTP: &jobHTTPResource{Path: "cron"}}, "single /", ""},
		{"host smuggling path", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@daily", HTTP: &jobHTTPResource{Path: "//evil.example"}}, "single /", ""},
		{"bad method", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@daily", HTTP: &jobHTTPResource{Method: "TRACE", Path: "/x"}}, "http.method", ""},
		{"oversized body", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "@daily", HTTP: &jobHTTPResource{Path: "/x", Body: strings.Repeat("a", maxJobHTTPBody+1)}}, "http.body", ""},
		{"exec still needs a command", scheduledTaskResource{Mode: store.ScheduledTaskModeExec, Schedule: "@daily"}, "command is required", ""},
		{"bad schedule", scheduledTaskResource{Mode: store.ScheduledTaskModeHTTP, Schedule: "nope", HTTP: &jobHTTPResource{Path: "/x"}}, "schedule", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateScheduledTaskInput(&tc.res)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tc.res.HTTP.Method != tc.wantMethod {
					t.Errorf("method = %q, want %q", tc.res.HTTP.Method, tc.wantMethod)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestNormalizeConcurrencyPolicy_Queue(t *testing.T) {
	got, err := normalizeScheduledTaskConcurrencyPolicy(store.ScheduledTaskConcurrencyQueue)
	if err != nil || got != store.ScheduledTaskConcurrencyQueue {
		t.Errorf("queue = %q, %v; want accepted", got, err)
	}
}
