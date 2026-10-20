package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogsSignaturesAndSaved(t *testing.T) {
	now := time.Now().UTC()
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotPath, gotBody = r.URL.RequestURI(), string(body)
		switch {
		case strings.Contains(r.URL.Path, "/logs/signatures"):
			_ = json.NewEncoder(w).Encode(map[string]any{"scanned": 7, "total": 1, "signatures": []map[string]any{
				{"id": "abc123", "template": "db timeout after <num>ms", "count": 5, "first_seen": now, "last_seen": now, "new": true},
			}})
		case r.URL.Path == "/api/v1/logs/queries" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"queries": []map[string]any{
				{"id": "slq_1", "name": "errors", "app": "web", "visibility": "instance", "spec": map[string]any{"q": "boom", "level": "error"}},
			}})
		case r.URL.Path == "/api/v1/logs/queries" && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "slq_2", "name": "mine"})
		case strings.HasSuffix(r.URL.Path, "/alerts") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "rule1", "name": "boom alert", "kind": "log_pattern"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	env := func(string) (string, bool) { return "", false }
	common := []string{"--api-url", srv.URL, "--token", "t"}

	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantReq  string
	}{
		{"signatures", []string{"logs", "signatures", "web", "--new", "--since", "2h"}, exitOK, "db timeout after <num>ms", "baseline=true"},
		{"saved list", []string{"logs", "saved", "list", "--app", "web"}, exitOK, "errors", "app=web"},
		{"saved create", []string{"logs", "saved", "create", "mine", "--app", "web", "--q", "boom", "--share", "--pin", "--field", "status>=500"}, exitOK, "slq_2", `"visibility":"instance"`},
		{"alert-from count", []string{"logs", "alert-from", "web", "--name", "boom alert", "--q", "boom", "--count", "10", "--window", "5m", "--cooldown", "15m"}, exitOK, "created log_pattern", `"count":10`},
		{"alert-from rate", []string{"logs", "alert-from", "web", "--name", "rate", "--per-minute", "3"}, exitOK, "created log_level_rate", `"per_minute":3`},
		{"alert-from needs one threshold", []string{"logs", "alert-from", "web", "--name", "x", "--count", "1", "--per-minute", "2"}, exitValidation, "", ""},
		{"alert-from needs name", []string{"logs", "alert-from", "web", "--count", "1"}, exitValidation, "", ""},
		{"saved unknown", []string{"logs", "saved", "frob"}, exitUsage, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run("levelrail-cli", append(append([]string{}, tc.args...), common...), &stdout, &stderr, env)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), tc.wantOut) {
				t.Errorf("stdout = %q, want to contain %q", stdout.String(), tc.wantOut)
			}
			if tc.wantReq != "" && !strings.Contains(gotPath+gotBody, tc.wantReq) {
				t.Errorf("request %q %q lacks %q", gotPath, gotBody, tc.wantReq)
			}
		})
	}
}
