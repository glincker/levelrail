package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type runtimeCall struct {
	method string
	path   string
	body   string
}

type runtimeReply struct {
	status int
	body   string
}

// newRuntimeSettingsTestServer records every request and answers from responses,
// keyed by "METHOD /path". Unmatched requests get 404.
func newRuntimeSettingsTestServer(t *testing.T, calls *[]runtimeCall, responses map[string]runtimeReply) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*calls = append(*calls, runtimeCall{method: r.Method, path: r.URL.Path, body: strings.TrimSpace(string(raw))})
		reply, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"not found"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(reply.status)
		_, _ = io.WriteString(w, reply.body)
	}))
}

func runtimeSettingJSON(t *testing.T, s runtimeSettingResource) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal setting: %v", err)
	}
	return string(b)
}

func runRuntimeCLI(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	return code, stdout.String(), stderr.String()
}

func TestRun_SettingsRuntime_Commands(t *testing.T) {
	replay := runtimeSettingResource{
		Key: "security.webhook_replay_protection", Kind: "bool", Risk: "normal", Apply: "live",
		Default: true, Value: true, Source: "default",
	}
	audit := runtimeSettingResource{
		Key: "security.audit_sensitive_reads", Kind: "bool", Risk: "weakens_security", Apply: "live",
		Default: true, Value: true, Source: "default",
	}
	window := runtimeSettingResource{
		Key: "security.webhook_replay_window", Kind: "duration", Risk: "normal", Apply: "live",
		Default: "10m", Value: "10m", Source: "default",
	}
	roots := runtimeSettingResource{
		Key: "security.bind_allowed_roots", Kind: "string_list", Risk: "normal", Apply: "live",
		Default: []any{}, Value: []any{}, Source: "default",
	}
	trustHeaders := runtimeSettingResource{
		Key: "security.trust_loopback_forwarded_headers", Kind: "bool", Risk: "normal", Apply: "restart",
		Default: true, Value: false, Source: "saved", PendingRestart: true,
	}
	listBody, err := json.Marshal(runtimeSettingsListResource{
		Settings: []runtimeSettingResource{replay, trustHeaders},
		Restart:  runtimeSettingsRestart{Supported: true, PendingRestart: true},
	})
	if err != nil {
		t.Fatalf("marshal list: %v", err)
	}

	const (
		replayPath = "/api/v1/settings/runtime/security.webhook_replay_protection"
		auditPath  = "/api/v1/settings/runtime/security.audit_sensitive_reads"
		windowPath = "/api/v1/settings/runtime/security.webhook_replay_window"
		rootsPath  = "/api/v1/settings/runtime/security.bind_allowed_roots"
	)

	tests := []struct {
		name       string
		args       []string
		responses  map[string]runtimeReply
		wantExit   int
		wantCalls  []runtimeCall
		wantStdout []string
		wantStderr []string
	}{
		{
			name: "list prints table with pending restart",
			args: []string{"settings", "runtime", "list"},
			responses: map[string]runtimeReply{
				"GET /api/v1/settings/runtime": {http.StatusOK, string(listBody)},
			},
			wantExit:   exitOK,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: "/api/v1/settings/runtime"}},
			wantStdout: []string{"KEY", "SOURCE", "PENDING RESTART", "security.webhook_replay_protection", "true", "default", "yes", "A restart is pending"},
		},
		{
			name: "get prints one setting",
			args: []string{"settings", "runtime", "get", "security.webhook_replay_protection"},
			responses: map[string]runtimeReply{
				"GET " + replayPath: {http.StatusOK, runtimeSettingJSON(t, replay)},
			},
			wantExit:   exitOK,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: replayPath}},
			wantStdout: []string{"key:", "security.webhook_replay_protection", "value:            true", "source:           default"},
		},
		{
			name: "set bool sends typed value",
			args: []string{"settings", "runtime", "set", "security.webhook_replay_protection", "false"},
			responses: map[string]runtimeReply{
				"GET " + replayPath: {http.StatusOK, runtimeSettingJSON(t, replay)},
				"PUT " + replayPath: {http.StatusOK, runtimeSettingJSON(t, replay)},
			},
			wantExit: exitOK,
			wantCalls: []runtimeCall{
				{method: http.MethodGet, path: replayPath},
				{method: http.MethodPut, path: replayPath, body: `{"value":false}`},
			},
			wantStdout: []string{"key:"},
		},
		{
			name: "set duration sends string",
			args: []string{"settings", "runtime", "set", "security.webhook_replay_window", "30m"},
			responses: map[string]runtimeReply{
				"GET " + windowPath: {http.StatusOK, runtimeSettingJSON(t, window)},
				"PUT " + windowPath: {http.StatusOK, runtimeSettingJSON(t, window)},
			},
			wantExit: exitOK,
			wantCalls: []runtimeCall{
				{method: http.MethodGet, path: windowPath},
				{method: http.MethodPut, path: windowPath, body: `{"value":"30m"}`},
			},
		},
		{
			name: "set string list splits and trims",
			args: []string{"settings", "runtime", "set", "security.bind_allowed_roots", "/srv/data, /opt,,"},
			responses: map[string]runtimeReply{
				"GET " + rootsPath: {http.StatusOK, runtimeSettingJSON(t, roots)},
				"PUT " + rootsPath: {http.StatusOK, runtimeSettingJSON(t, roots)},
			},
			wantExit: exitOK,
			wantCalls: []runtimeCall{
				{method: http.MethodGet, path: rootsPath},
				{method: http.MethodPut, path: rootsPath, body: `{"value":["/srv/data","/opt"]}`},
			},
		},
		{
			name: "set empty string list clears to an empty array",
			args: []string{"settings", "runtime", "set", "security.bind_allowed_roots", ""},
			responses: map[string]runtimeReply{
				"GET " + rootsPath: {http.StatusOK, runtimeSettingJSON(t, roots)},
				"PUT " + rootsPath: {http.StatusOK, runtimeSettingJSON(t, roots)},
			},
			wantExit: exitOK,
			wantCalls: []runtimeCall{
				{method: http.MethodGet, path: rootsPath},
				{method: http.MethodPut, path: rootsPath, body: `{"value":[]}`},
			},
		},
		{
			name: "set weakening setting without confirm is refused before writing",
			args: []string{"settings", "runtime", "set", "security.audit_sensitive_reads", "false"},
			responses: map[string]runtimeReply{
				"GET " + auditPath: {http.StatusOK, runtimeSettingJSON(t, audit)},
				"PUT " + auditPath: {http.StatusOK, runtimeSettingJSON(t, audit)},
			},
			wantExit:   exitValidation,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: auditPath}},
			wantStderr: []string{"--confirm-weakening"},
		},
		{
			name: "set weakening setting with confirm sends confirm_weakening",
			args: []string{"settings", "runtime", "set", "security.audit_sensitive_reads", "false", "--confirm-weakening"},
			responses: map[string]runtimeReply{
				"GET " + auditPath: {http.StatusOK, runtimeSettingJSON(t, audit)},
				"PUT " + auditPath: {http.StatusOK, runtimeSettingJSON(t, audit)},
			},
			wantExit: exitOK,
			wantCalls: []runtimeCall{
				{method: http.MethodGet, path: auditPath},
				{method: http.MethodPut, path: auditPath, body: `{"value":false,"confirm_weakening":true}`},
			},
		},
		{
			name: "set invalid bool is rejected before writing",
			args: []string{"settings", "runtime", "set", "security.webhook_replay_protection", "yes"},
			responses: map[string]runtimeReply{
				"GET " + replayPath: {http.StatusOK, runtimeSettingJSON(t, replay)},
			},
			wantExit:   exitValidation,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: replayPath}},
			wantStderr: []string{"true or false"},
		},
		{
			name: "set invalid duration is rejected before writing",
			args: []string{"settings", "runtime", "set", "security.webhook_replay_window", "soon"},
			responses: map[string]runtimeReply{
				"GET " + windowPath: {http.StatusOK, runtimeSettingJSON(t, window)},
			},
			wantExit:   exitValidation,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: windowPath}},
			wantStderr: []string{"duration"},
		},
		{
			name:       "set missing value is a usage error",
			args:       []string{"settings", "runtime", "set", "security.webhook_replay_protection"},
			wantExit:   exitUsage,
			wantStderr: []string{"KEY and VALUE"},
		},
		{
			name: "server rejection surfaces as api error",
			args: []string{"settings", "runtime", "get", "security.unknown"},
			responses: map[string]runtimeReply{
				"GET /api/v1/settings/runtime/security.unknown": {http.StatusNotFound, `{"error":"unknown setting"}`},
			},
			wantExit:   exitAPIError,
			wantCalls:  []runtimeCall{{method: http.MethodGet, path: "/api/v1/settings/runtime/security.unknown"}},
			wantStderr: []string{"unknown setting"},
		},
		{
			name: "reset deletes the saved value",
			args: []string{"settings", "runtime", "reset", "security.webhook_replay_protection"},
			responses: map[string]runtimeReply{
				"DELETE " + replayPath: {http.StatusOK, runtimeSettingJSON(t, replay)},
			},
			wantExit:   exitOK,
			wantCalls:  []runtimeCall{{method: http.MethodDelete, path: replayPath}},
			wantStdout: []string{"key:"},
		},
		{
			name: "apply posts to the apply route",
			args: []string{"settings", "runtime", "apply"},
			responses: map[string]runtimeReply{
				"POST /api/v1/settings/runtime/apply": {http.StatusAccepted, `{"message":"restart scheduled"}`},
			},
			wantExit:   exitOK,
			wantCalls:  []runtimeCall{{method: http.MethodPost, path: "/api/v1/settings/runtime/apply"}},
			wantStdout: []string{"restart scheduled"},
		},
		{
			name: "apply reports conflict when restart is unsupported",
			args: []string{"settings", "runtime", "apply"},
			responses: map[string]runtimeReply{
				"POST /api/v1/settings/runtime/apply": {http.StatusConflict, `{"error":"restart not supported on this deployment"}`},
			},
			wantExit:   exitAPIError,
			wantCalls:  []runtimeCall{{method: http.MethodPost, path: "/api/v1/settings/runtime/apply"}},
			wantStderr: []string{"restart not supported"},
		},
		{
			name:       "unknown subcommand is a usage error",
			args:       []string{"settings", "runtime", "toggle"},
			wantExit:   exitUsage,
			wantStderr: []string{"unknown settings runtime subcommand"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []runtimeCall
			srv := newRuntimeSettingsTestServer(t, &calls, tt.responses)
			defer srv.Close()

			args := append(append([]string{}, tt.args...), "--api-url", srv.URL, "--token", "test-token")
			code, stdout, stderr := runRuntimeCLI(args)
			if code != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", code, tt.wantExit, stdout, stderr)
			}
			if len(calls) != len(tt.wantCalls) {
				t.Fatalf("calls = %+v, want %+v", calls, tt.wantCalls)
			}
			for i, want := range tt.wantCalls {
				got := calls[i]
				if got.method != want.method || got.path != want.path {
					t.Errorf("call %d = %s %s, want %s %s", i, got.method, got.path, want.method, want.path)
				}
				if want.body != "" && got.body != want.body {
					t.Errorf("call %d body = %s, want %s", i, got.body, want.body)
				}
			}
			for _, s := range tt.wantStdout {
				if !strings.Contains(stdout, s) {
					t.Errorf("stdout missing %q: %q", s, stdout)
				}
			}
			for _, s := range tt.wantStderr {
				if !strings.Contains(stderr, s) {
					t.Errorf("stderr missing %q: %q", s, stderr)
				}
			}
		})
	}
}
