package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_LogsAccessSanitizesTerminalText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/apps/") {
			_, _ = w.Write([]byte(`[{"time":"2026-10-15T10:00:00Z","method":"GET","path":"/x\u001b[2Jy","status":500,"request_id":"r\u001b1"}]`))
			return
		}
		if q := r.URL.Query(); q.Get("status") != "5xx" || q.Get("min_ms") != "2" {
			t.Errorf("filters did not reach the API: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"records":[{"time":"2026-10-15T10:00:00Z","app":"web","host":"a.example.com","method":"GET","path":"/x\u001b]8;;https://evil\u0007y","status":500,"duration_ms":3,"request_id":"r1"}],"truncated":false}`))
	}))
	defer srv.Close()
	for _, args := range [][]string{
		{"logs", "access", "--status", "5xx", "--path", "/x", "--min-ms", "2"},
		{"apps", "access", "web"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run("levelrail-cli-test", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap()); code != exitOK {
			t.Fatalf("%v: exit %d: %s", args, code, stderr.String())
		}
		if strings.ContainsAny(stdout.String(), "\x1b\x07") {
			t.Fatalf("%v: escape sequence reached the terminal: %q", args, stdout.String())
		}
	}
}
