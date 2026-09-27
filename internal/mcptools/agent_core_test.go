package mcptools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func newModeSession(t *testing.T, opts Options, h http.HandlerFunc) *mcp.ClientSession {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	server, _ := NewServerWithOptions(apiclient.NewClient(srv.URL, "t"), opts)
	st, ct := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Run(ctx, st) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close(); cancel(); <-done })
	return session
}

func TestAgentCoreToolSet(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeAgentCore})
	if len(tools) > 15 {
		t.Fatalf("agent-core exposes %d tools, want at most 15", len(tools))
	}
	got := map[string]*mcp.Tool{}
	for _, tool := range tools {
		got[tool.Name] = tool
	}
	for name := range agentCoreTools {
		tool, ok := got[name]
		if !ok {
			t.Errorf("agent-core is missing %q", name)
			continue
		}
		m, _ := Lookup(name)
		a := tool.Annotations
		if a == nil || a.ReadOnlyHint != (m.Class == ClassRead) {
			t.Errorf("%s: read-only hint does not match class %s", name, m.Class)
		}
		if m.Class != ClassRead && (a.DestructiveHint == nil || *a.DestructiveHint != (m.Class == ClassDestructive)) {
			t.Errorf("%s: destructive hint does not match class %s", name, m.Class)
		}
		if len(tool.Description) > 200 {
			t.Errorf("%s: description is %d chars, keep it to one short sentence", name, len(tool.Description))
		}
		if tool.OutputSchema != nil {
			t.Errorf("%s: compact tools should not carry an output schema", name)
		}
	}
	if len(got) != len(agentCoreTools) {
		t.Errorf("agent-core exposes %d tools, allowlist has %d", len(got), len(agentCoreTools))
	}
	if m, _ := Lookup("rollback_app"); m.Class != ClassDestructive {
		t.Errorf("rollback_app must stay destructive")
	}
}

func TestAgentCoreSmallerThanFull(t *testing.T) {
	core := estimateTokens(t, listAll(t, Options{Mode: ModeAgentCore}))
	full := estimateTokens(t, listAll(t, Options{Mode: ModeFull}))
	t.Logf("agent-core ~%d tokens, full ~%d tokens", core, full)
	if core*5 > full {
		t.Errorf("agent-core (~%d) should be under a fifth of full (~%d)", core, full)
	}
}

func TestParseModeAgentCore(t *testing.T) {
	for in, want := range map[string]Mode{"agent-core": ModeAgentCore, " Agent-Core ": ModeAgentCore, "": ModeStandard} {
		got, err := ParseMode(in)
		if err != nil || got != want {
			t.Errorf("ParseMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestAgentCoreCompactResults(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		args    map[string]any
		handler http.HandlerFunc
		want    []string
		notWant []string
	}{
		{
			name: "deploy pending approval",
			tool: "deploy_app",
			args: map[string]any{"name": "web", "image": "img:2"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "img:1", "port": 80, "pending_approval": map[string]any{"id": "apr_1", "status": "pending"}})
			},
			want:    []string{`"accepted":false`, `"pending_approval_id":"apr_1"`},
			notWant: []string{`"port"`},
		},
		{
			name: "deploy accepted",
			tool: "rollback_app",
			args: map[string]any{"name": "web", "image": "img:1"},
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "img:1"})
			},
			want: []string{`"accepted":true`, "get_app_status"},
		},
		{
			name: "list apps drops env",
			tool: "list_apps",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode([]map[string]any{{"name": "web", "image": "img:1", "port": 80, "env": map[string]string{"A": "secretish"}}})
			},
			want:    []string{`"count":1`, `"name":"web"`},
			notWant: []string{"secretish"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			session := newModeSession(t, Options{Mode: ModeAgentCore}, tc.handler)
			res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil || res.IsError {
				t.Fatalf("call: %v %+v", err, res)
			}
			text := toolResultText(res)
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("result %s missing %q", text, w)
				}
			}
			for _, w := range tc.notWant {
				if strings.Contains(text, w) {
					t.Errorf("result %s should not contain %q", text, w)
				}
			}
		})
	}
}

func TestSetAppEnv(t *testing.T) {
	var saved map[string]string
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web":
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "i", "env": map[string]string{"KEEP": "1", "OLD": "x"}, "secret_env": []string{"TOKEN"}})
		case r.URL.Path == "/api/v1/apps/web/secrets":
			_ = json.NewEncoder(w).Encode([]map[string]any{{"key": "DB_PASS"}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web":
			var app apiclient.AppResource
			_ = json.NewDecoder(r.Body).Decode(&app)
			saved = app.Env
			_ = json.NewEncoder(w).Encode(app)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}
	session := newModeSession(t, Options{Mode: ModeAgentCore}, handler)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_app_env", Arguments: map[string]any{
		"name": "web", "set": map[string]string{"NEW": "2", "TOKEN": "nope", "DB_PASS": "nope"}, "unset": []string{"OLD"},
	}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	if saved["NEW"] != "2" || saved["KEEP"] != "1" || saved["TOKEN"] != "" || saved["DB_PASS"] != "" {
		t.Errorf("saved env = %v", saved)
	}
	if _, ok := saved["OLD"]; ok {
		t.Errorf("OLD should be removed: %v", saved)
	}
	text := toolResultText(res)
	if !strings.Contains(text, "TOKEN") || !strings.Contains(text, "skipped_secret_keys") {
		t.Errorf("result should report skipped secret keys: %s", text)
	}
}

func TestSetAppEnvNoChangeSkipsSave(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			t.Errorf("no PUT expected for an all-secret change")
		}
		if strings.HasSuffix(r.URL.Path, "/secrets") {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "secret_env": []string{"TOKEN"}})
	}
	session := newModeSession(t, Options{Mode: ModeAgentCore}, handler)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_app_env", Arguments: map[string]any{"name": "web", "set": map[string]string{"TOKEN": "x"}}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v", err)
	}
}

func TestSetAppSecretNeverEchoesValue(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/apps/web/secrets/API_KEY" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}
	session := newModeSession(t, Options{Mode: ModeAgentCore}, handler)
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "set_app_secret", Arguments: map[string]any{"name": "web", "key": "API_KEY", "value": "hunter2"}})
	if err != nil || res.IsError {
		t.Fatalf("call: %v %s", err, toolResultText(res))
	}
	if strings.Contains(toolResultText(res), "hunter2") {
		t.Errorf("secret value echoed: %s", toolResultText(res))
	}
}
