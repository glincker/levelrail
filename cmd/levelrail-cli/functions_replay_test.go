package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_FunctionsReplayAndInvocations(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/functions/fn/invocations/abc/replay":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": 200, "body": "again", "duration_ms": 12, "cold_start": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/functions/fn/invocations":
			_ = json.NewEncoder(w).Encode(map[string]any{"invocations": []map[string]any{{"id": "abc", "method": "GET", "path": "/", "status": 200, "cold_start": true, "replayable": true}}})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(api.Close)
	call := func(args ...string) (string, int) {
		var stdout, stderr bytes.Buffer
		args = append(args, "--token", "t", "--api-url", api.URL)
		code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
		return stdout.String(), code
	}
	if out, code := call("functions", "invoke", "fn", "--replay", "abc"); code != exitOK || out != "again" {
		t.Fatalf("replay = %d %q", code, out)
	}
	if out, code := call("functions", "invocations", "fn"); code != exitOK || !strings.Contains(out, "abc") || !strings.Contains(out, "replayable") {
		t.Fatalf("invocations = %d %q", code, out)
	}
}
