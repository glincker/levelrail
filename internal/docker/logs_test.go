package docker

import (
	"testing"
	"time"
)

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantTS      time.Time
		wantMessage string
	}{
		{
			name:        "well-formed timestamp prefix",
			raw:         "2026-08-12T10:00:00.123456789Z hello world",
			wantTS:      time.Date(2026, 8, 12, 10, 0, 0, 123456789, time.UTC),
			wantMessage: "hello world",
		},
		{
			name:        "message itself contains spaces, only the first space splits",
			raw:         `2026-08-12T10:00:00.000000000Z {"level":"info","msg":"a b c"}`,
			wantTS:      time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC),
			wantMessage: `{"level":"info","msg":"a b c"}`,
		},
		{
			name:        "no space at all: not Docker's timestamped format, returned as-is",
			raw:         "no-timestamp-here",
			wantTS:      time.Time{},
			wantMessage: "no-timestamp-here",
		},
		{
			name:        "space present but prefix isn't a valid timestamp",
			raw:         "not-a-timestamp hello",
			wantTS:      time.Time{},
			wantMessage: "not-a-timestamp hello",
		},
		{
			name:        "empty line",
			raw:         "",
			wantTS:      time.Time{},
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTS, gotMessage := parseLogLine(tt.raw)
			if !gotTS.Equal(tt.wantTS) {
				t.Errorf("parseLogLine() ts = %v, want %v", gotTS, tt.wantTS)
			}
			if gotMessage != tt.wantMessage {
				t.Errorf("parseLogLine() message = %q, want %q", gotMessage, tt.wantMessage)
			}
		})
	}
}

// TestClient_Logs_NoSuchContainer proves Logs surfaces a real ContainerLogs
// error (e.g. no such container, or no daemon reachable) on the error
// channel rather than hanging or panicking, without needing a live daemon
// to observe: NewClient succeeds even with nothing listening at
// DOCKER_HOST, since it only negotiates lazily on first real call.
func TestClient_Logs_NoSuchContainer(t *testing.T) {
	c, err := NewClient()
	if err != nil {
		t.Skipf("could not construct a docker client at all: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	lines, errs := c.Logs(t.Context(), "levelrail-logs-test-no-such-container-id", false, time.Time{})

	select {
	case _, ok := <-lines:
		if ok {
			t.Fatal("expected no lines for a nonexistent container")
		}
	case err := <-errs:
		if err == nil {
			t.Fatal("expected a non-nil error for a nonexistent container")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for Logs() to report an error or close")
	}
}
