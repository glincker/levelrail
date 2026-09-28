package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestEnvTools(t *testing.T) {
	const secretValue = "s3cr3t-value-xyz"
	tests := []struct {
		name       string
		tool       string
		args       map[string]any
		wantCalls  []string
		wantRedeps bool
		wantErr    string
	}{
		{"set plain", "set_app_env", map[string]any{"name": "web", "key": "A", "value": "1"}, []string{"GET /api/v1/apps/web", "PUT /api/v1/apps/web"}, true, ""},
		{"set secret", "set_app_env", map[string]any{"name": "web", "key": "TOKEN", "value": secretValue, "secret": true}, []string{"PUT /api/v1/apps/web/secrets/TOKEN"}, true, ""},
		{"unset plain", "unset_app_env", map[string]any{"name": "web", "key": "EXISTING"}, []string{"GET /api/v1/apps/web", "PUT /api/v1/apps/web"}, true, ""},
		{"unset missing plain", "unset_app_env", map[string]any{"name": "web", "key": "NOPE"}, []string{"GET /api/v1/apps/web"}, false, "no plain env var"},
		{"unset secret", "unset_app_env", map[string]any{"name": "web", "key": "TOKEN", "secret": true}, []string{"DELETE /api/v1/apps/web/secrets/TOKEN"}, true, ""},
		{"unset secret force", "unset_app_env", map[string]any{"name": "web", "key": "TOKEN", "secret": true, "force": true}, []string{"DELETE /api/v1/apps/web/secrets/TOKEN"}, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			var calls []string
			session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls = append(calls, r.Method+" "+r.URL.Path)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet:
					_ = json.NewEncoder(w).Encode(apiclient.AppResource{Name: "web", Env: map[string]string{"EXISTING": "x"}})
				case r.Method == http.MethodPut && !strings.Contains(r.URL.Path, "/secrets/"):
					var app apiclient.AppResource
					_ = json.NewDecoder(r.Body).Decode(&app)
					app.EnvDirty = true
					_ = json.NewEncoder(w).Encode(app)
				default:
					w.WriteHeader(http.StatusNoContent)
				}
			})
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil {
				t.Fatal(err)
			}
			text := toolResultText(result)
			if strings.Contains(text, secretValue) {
				t.Fatalf("secret value leaked in result: %s", text)
			}
			if tc.wantErr != "" {
				if !result.IsError || !strings.Contains(text, tc.wantErr) {
					t.Fatalf("result = %s, want error %q", text, tc.wantErr)
				}
			} else {
				if result.IsError {
					t.Fatalf("unexpected error: %s", text)
				}
				var got EnvChangeResult
				decodeStructured(t, result, &got)
				if got.Key != tc.args["key"] || got.RedeployNeeded != tc.wantRedeps || !strings.Contains(got.Hint, "deploy_app") {
					t.Fatalf("got %+v", got)
				}
			}
			if strings.Join(calls, ",") != strings.Join(tc.wantCalls, ",") {
				t.Fatalf("calls = %v, want %v", calls, tc.wantCalls)
			}
		})
	}
}

func TestSetAppEnvErrorDoesNotLeakSecret(t *testing.T) {
	const secretValue = "s3cr3t-value-xyz"
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"key is locked"}`))
	})
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_app_env", Arguments: map[string]any{"name": "web", "key": "TOKEN", "value": secretValue, "secret": true}})
	if err != nil {
		t.Fatal(err)
	}
	text := toolResultText(result)
	if !result.IsError || !strings.Contains(text, "locked") || strings.Contains(text, secretValue) {
		t.Fatalf("result = %s", text)
	}
}

func TestEnvToolsClassified(t *testing.T) {
	for _, n := range []string{"set_app_env", "unset_app_env"} {
		m, ok := toolTable[n]
		if !ok || m.Class != ClassMutate || !m.Sensitive() {
			t.Fatalf("%s meta = %+v", n, m)
		}
	}
}
