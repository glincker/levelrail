package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestLogsQuery(t *testing.T) {
	now := time.Now().UTC()
	var gotQuery map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = map[string]string{}
		for k := range r.URL.Query() {
			gotQuery[k] = r.URL.Query().Get(k)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 40, "entries": []map[string]any{
			{"timestamp": now.Format(time.RFC3339), "message": "db down", "level": "error"},
		}})
	}))
	t.Cleanup(srv.Close)
	env := func(string) (string, bool) { return "", false }

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
	}{
		{"table", []string{"logs", "query", "web", "--level", "error", "--since", "30m", "--api-url", srv.URL, "--token", "t"}, exitOK, "ERROR db down"},
		{"notice", []string{"logs", "query", "web", "--api-url", srv.URL, "--token", "t"}, exitOK, "# showing 1 of 40 matching lines"},
		{"json", []string{"logs", "query", "web", "--json", "--api-url", srv.URL, "--token", "t"}, exitOK, `"matched": 40`},
		{"bad since", []string{"logs", "query", "web", "--since", "zzz", "--api-url", srv.URL, "--token", "t"}, exitValidation, ""},
		{"missing app", []string{"logs", "query", "--api-url", srv.URL, "--token", "t"}, exitUsage, ""},
		{"negative cap", []string{"logs", "query", "web", "--max-bytes", "-1", "--api-url", srv.URL, "--token", "t"}, exitValidation, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli", tc.args, &stdout, &stderr, env)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want to contain %q", stdout.String(), tc.wantOut)
			}
		})
	}
	if gotQuery["limit"] != "100" {
		t.Errorf("default max lines not sent: %v", gotQuery)
	}
}

func TestLogsQueryHonorsEnvByteCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		entries := make([]map[string]any, 50)
		for i := range entries {
			entries[i] = map[string]any{"timestamp": time.Now().UTC().Format(time.RFC3339), "message": strings.Repeat("z", 100)}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 50, "entries": entries})
	}))
	t.Cleanup(srv.Close)
	env := func(k string) (string, bool) {
		if k == apiclient.EnvLogMaxBytes {
			return "1024", true
		}
		return "", false
	}
	var stdout, stderr bytes.Buffer
	if code := run("levelrail-cli", []string{"logs", "query", "web", "--json", "--api-url", srv.URL, "--token", "t"}, &stdout, &stderr, env); code != exitOK {
		t.Fatalf("exit = %d: %s", code, stderr.String())
	}
	var ex apiclient.LogExcerpt
	if err := json.Unmarshal(stdout.Bytes(), &ex); err != nil {
		t.Fatal(err)
	}
	if ex.Shown >= 50 || ex.Notice == "" {
		t.Errorf("env cap not applied: shown=%d notice=%q", ex.Shown, ex.Notice)
	}
}
