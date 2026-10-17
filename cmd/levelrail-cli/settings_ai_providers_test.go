package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SettingsAIProviders(t *testing.T) {
	var method, path, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		method, path, body = r.Method, r.URL.Path, string(b)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(path, "/providers"):
			_ = json.NewEncoder(w).Encode([]apiclient.AIProviderResource{{ID: "openai", Label: "OpenAI", HasKey: true, Status: "ok"}})
		case strings.HasSuffix(path, "/test"):
			_, _ = w.Write([]byte(`{"ok":true,"status":"ok","latency_ms":12}`))
		case strings.HasSuffix(path, "/models"):
			_, _ = w.Write([]byte(`[{"id":"gpt-x","label":"gpt-x","context_window":1000}]`))
		case strings.HasSuffix(path, "/usage"):
			_, _ = w.Write([]byte(`{"days":[],"totals":{"requests":3}}`))
		default:
			_, _ = w.Write([]byte(`{"id":"openai","configured":true}`))
		}
	}))
	defer srv.Close()
	env := func(k string) (string, bool) { return "sk-from-env", k == "MY_KEY" }
	cases := []struct {
		args         []string
		method, path string
		wantOut      string
		wantBody     string
	}{
		{[]string{"providers", "list"}, "GET", "/api/v1/settings/ai-assistant/providers", "openai", ""},
		{[]string{"providers", "set", "openai", "--api-key-env", "MY_KEY"}, "PUT", "/api/v1/settings/ai-assistant/providers/openai", "openai", `"api_key":"sk-from-env"`},
		{[]string{"providers", "test", "openai"}, "POST", "/api/v1/settings/ai-assistant/providers/openai/test", "latency: 12 ms", ""},
		{[]string{"providers", "models", "openai"}, "GET", "/api/v1/settings/ai-assistant/providers/openai/models", "gpt-x", ""},
		{[]string{"providers", "remove", "openai"}, "DELETE", "/api/v1/settings/ai-assistant/providers/openai", "removed", ""},
		{[]string{"use", "openai", "gpt-x"}, "PUT", "/api/v1/settings/ai-assistant", "configured", `"active":{"provider":"openai","model":"gpt-x"}`},
		{[]string{"fallback", "set", "groq:llama", "gemini:flash"}, "PUT", "/api/v1/settings/ai-assistant", "configured", `"fallback":[{"provider":"groq","model":"llama"},{"provider":"gemini","model":"flash"}]`},
		{[]string{"usage", "--days", "7"}, "GET", "/api/v1/ai/usage", "total", ""},
	}
	for _, tc := range cases {
		args := append([]string{"settings", "ai-assistant"}, tc.args...)
		args = append(args, "--api-url", srv.URL)
		var stdout, stderr strings.Builder
		if code := run("levelrail-cli-test", args, &stdout, &stderr, env); code != exitOK {
			t.Fatalf("%v: exit %d stderr %s", tc.args, code, stderr.String())
		}
		if method != tc.method || path != tc.path || !strings.Contains(stdout.String(), tc.wantOut) || !strings.Contains(body, tc.wantBody) {
			t.Errorf("%v: %s %s body=%s out=%s", tc.args, method, path, body, stdout.String())
		}
	}
}
