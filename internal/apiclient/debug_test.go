package apiclient

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTraceRequest_Disabled(t *testing.T) {
	SetDebugTrace(nil)
	t.Cleanup(func() { SetDebugTrace(nil) })

	TraceRequest("GET", "http://x/api/v1/apps", "200 OK", time.Millisecond, nil)
}

func TestTraceRequest_Success(t *testing.T) {
	var buf bytes.Buffer
	SetDebugTrace(&buf)
	t.Cleanup(func() { SetDebugTrace(nil) })

	TraceRequest("GET", "http://x/api/v1/apps", "200 OK", 5*time.Millisecond, nil)

	got := buf.String()
	for _, want := range []string{"GET", "http://x/api/v1/apps", "200 OK"} {
		if !strings.Contains(got, want) {
			t.Errorf("trace line %q missing %q", got, want)
		}
	}
}

func TestTraceRequest_Error(t *testing.T) {
	var buf bytes.Buffer
	SetDebugTrace(&buf)
	t.Cleanup(func() { SetDebugTrace(nil) })

	TraceRequest("GET", "http://x/api/v1/apps", "", time.Millisecond, errors.New("connection refused"))

	got := buf.String()
	if !strings.Contains(got, "error: connection refused") {
		t.Errorf("trace line %q missing error detail", got)
	}
}

func TestTraceRequest_RedactsSensitiveQueryParams(t *testing.T) {
	var buf bytes.Buffer
	SetDebugTrace(&buf)
	t.Cleanup(func() { SetDebugTrace(nil) })

	TraceRequest("GET", "http://x/api/v1/audit-log?token=SUPERSECRET&before=abc", "200 OK", time.Millisecond, nil)

	got := buf.String()
	if strings.Contains(got, "SUPERSECRET") {
		t.Fatalf("trace line leaked a token value: %q", got)
	}
	if !strings.Contains(got, "REDACTED") || !strings.Contains(got, "before=abc") {
		t.Errorf("trace line %q did not redact token while preserving other params", got)
	}
}

func TestRedactURL_InvalidURLReturnedAsIs(t *testing.T) {
	raw := "http://[::1"
	if got := redactURL(raw); got != raw {
		t.Errorf("redactURL(%q) = %q, want unchanged", raw, got)
	}
}
