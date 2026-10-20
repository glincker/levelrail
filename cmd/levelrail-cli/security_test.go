package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityCommands(t *testing.T) {
	var gotMethod, gotURI string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotURI, gotBody = r.Method, r.URL.RequestURI(), nil
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/security/posture":
			_, _ = w.Write([]byte(`{"score":72,"grade":"C","full":true,"items":[{"id":"admin_without_mfa","severity":"critical","status":"fail","count":1,"fix":{"kind":"link","link":"/settings/security"}}],"account":[]}`))
		case r.URL.Path == "/api/v1/security/sessions":
			_, _ = w.Write([]byte(`{"sessions":[{"id":"s1","browser":"Firefox on Linux","network":"203.0.113.0/24","current":true}],"trusted_devices":[],"tokens":[]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/security/sessions/revoke-others"):
			_, _ = w.Write([]byte(`{"revoked":3}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/security/sessions/"):
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/security/policy":
			_, _ = w.Write([]byte(`{"approval_scope":"all_methods","sources":{"approval_scope":"saved"}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()
	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantMethod string
		wantURI    string
		wantOut    string
	}{
		{"posture", []string{"security", "posture"}, exitOK, "GET", "/api/v1/security/posture", "admin_without_mfa"},
		{"posture json", []string{"security", "posture", "--json"}, exitOK, "GET", "/api/v1/security/posture", `"score": 72`},
		{"sessions list", []string{"security", "sessions", "list"}, exitOK, "GET", "/api/v1/security/sessions", "Firefox on Linux"},
		{"sessions list for a user", []string{"security", "sessions", "list", "--user", "user_b"}, exitOK, "GET", "/api/v1/security/sessions?user_id=user_b", "s1"},
		{"revoke", []string{"security", "sessions", "revoke", "s1"}, exitOK, "DELETE", "/api/v1/security/sessions/s1", "revoked session s1"},
		{"revoke others", []string{"security", "sessions", "revoke-others"}, exitOK, "POST", "/api/v1/security/sessions/revoke-others", "revoked 3"},
		{"policy get", []string{"security", "policy", "get"}, exitOK, "GET", "/api/v1/security/policy", "saved"},
		{"policy set", []string{"security", "policy", "set", "--approval-scope", "all_methods"}, exitOK, "PUT", "/api/v1/security/policy", "all_methods"},
		{"policy set needs a flag", []string{"security", "policy", "set"}, exitUsage, "", "", ""},
		{"revoke needs an id", []string{"security", "sessions", "revoke"}, exitUsage, "", "", ""},
		{"unknown subcommand", []string{"security", "nope"}, exitUsage, "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMethod, gotURI = "", ""
			var stdout, stderr bytes.Buffer
			args := append(append([]string{}, tt.args...), "--api-url", srv.URL, "--token", "t")
			if tt.wantExit == exitUsage && tt.name == "unknown subcommand" {
				args = tt.args
			}
			got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			if gotMethod != tt.wantMethod || gotURI != tt.wantURI {
				t.Fatalf("request = %s %s, want %s %s", gotMethod, gotURI, tt.wantMethod, tt.wantURI)
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("stdout %q missing %q", stdout.String(), tt.wantOut)
			}
		})
	}
	var stdout, stderr bytes.Buffer
	run("levelrail-cli-test", []string{"security", "policy", "set", "--max-token-lifetime-days", "0", "--api-url", srv.URL, "--token", "t"}, &stdout, &stderr, envMap())
	if v, ok := gotBody["max_token_lifetime_days"]; !ok || v != float64(0) || len(gotBody) != 1 {
		t.Fatalf("an explicit 0 must be sent and nothing else: %v", gotBody)
	}
}
