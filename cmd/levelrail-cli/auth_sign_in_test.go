package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionLogin_WaitsForNewDeviceApproval(t *testing.T) {
	approvalPollInterval = time.Millisecond
	t.Cleanup(func() { approvalPollInterval = 3 * time.Second })
	tests := []struct {
		name     string
		final    string
		wantExit int
		wantErr  string
	}{
		{"approved", `{"status":"approved","username":"admin"}`, exitOK, ""},
		{"denied", `{"status":"denied"}`, exitAPIError, "denied"},
		{"expired", `{"status":"expired"}`, exitAPIError, "in time"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			polls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/auth/login":
					http.SetCookie(w, &http.Cookie{Name: "login_approval_binding", Value: "b", Path: "/api/v1/auth/login-approval", HttpOnly: true}) //nolint:gosec // plain HTTP test server, Secure would stop the jar resending it
					_, _ = w.Write([]byte(`{"approval_required":true,"approval_id":"la_1"}`))
				case "/api/v1/auth/login-approval/poll":
					if c, err := r.Cookie("login_approval_binding"); err != nil || c.Value != "b" {
						t.Errorf("poll without the approval cookie")
					}
					polls++
					if polls == 1 {
						_, _ = w.Write([]byte(`{"status":"pending"}`))
						return
					}
					http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "s", Path: "/", HttpOnly: true}) //nolint:gosec // plain HTTP test server
					_, _ = w.Write([]byte(tt.final))
				case "/api/v1/auth/tokens":
					_, _ = w.Write([]byte(`[]`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
				}
			}))
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"tokens", "list", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			if !strings.Contains(stderr.String(), "la_1") {
				t.Errorf("stderr = %q, want the approval id", stderr.String())
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestRun_AuthSignIn(t *testing.T) {
	const list = `{"codes":[{"id":"lc_1","requester_ip":"203.0.113.7","user_agent":"Firefox","created_at":"2026-10-10T10:00:00Z","expires_at":"2026-10-10T10:10:00Z","revealable":true}],"approvals":[{"id":"la_1","requester_ip":"198.51.100.2","user_agent":"Safari","created_at":"2026-10-10T10:00:00Z","expires_at":"2026-10-10T10:10:00Z"}]}`
	tests := []struct {
		name       string
		args       []string
		routes     map[string]string
		wantExit   int
		wantStdout []string
	}{
		{"code reveals waiting codes with context", []string{"auth", "code"}, map[string]string{
			"GET /api/v1/auth/sign-in-requests":                    list,
			"POST /api/v1/auth/sign-in-requests/codes/lc_1/reveal": `{"code":"ABCD-EF12","expires_at":"2026-10-10T10:10:00Z"}`,
		}, exitOK, []string{"ABCD-EF12", "203.0.113.7", "Firefox", "la_1", "198.51.100.2"}},
		{"code json", []string{"auth", "code", "--json"}, map[string]string{
			"GET /api/v1/auth/sign-in-requests":                    list,
			"POST /api/v1/auth/sign-in-requests/codes/lc_1/reveal": `{"code":"ABCD-EF12","expires_at":"2026-10-10T10:10:00Z"}`,
		}, exitOK, []string{`"code": "ABCD-EF12"`, `"approvals"`}},
		{"approve", []string{"auth", "approve", "la_1"}, map[string]string{
			"POST /api/v1/auth/login-approvals/la_1/approve": "",
		}, exitOK, []string{"approved sign-in la_1"}},
		{"deny", []string{"auth", "deny", "la_1"}, map[string]string{
			"POST /api/v1/auth/login-approvals/la_1/deny": "",
		}, exitOK, []string{"denied sign-in la_1"}},
		{"devices list", []string{"auth", "devices"}, map[string]string{
			"GET /api/v1/auth/trusted-devices": `{"devices":[{"id":"td_1","label":"Chrome","ip":"192.0.2.1","created_at":"2026-10-10T10:00:00Z","last_used_at":"2026-10-10T10:00:00Z","expires_at":"2027-01-01T00:00:00Z"}]}`,
		}, exitOK, []string{"td_1", "Chrome"}},
		{"devices revoke", []string{"auth", "devices", "revoke", "td_1"}, map[string]string{
			"DELETE /api/v1/auth/trusted-devices/td_1": "",
		}, exitOK, []string{"revoked trusted device td_1"}},
		{"code-login update", []string{"auth", "code-login", "--admins", "true"}, map[string]string{
			"PUT /api/v1/settings/auth/code-login": `{"admins":true,"others":true,"saved":true,"new_device_approval":true}`,
		}, exitOK, []string{"admins:               true"}},
		{"code-login bad value", []string{"auth", "code-login", "--admins", "maybe"}, nil, exitValidation, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, ok := tt.routes[r.Method+" "+r.URL.Path]
				if !ok {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if body == "" {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			args := append(tt.args, "--token", "tok", "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout = %q, want %q", stdout.String(), w)
				}
			}
		})
	}
}
