package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func approvalServer(t *testing.T, final string, pendingForever bool) *httptest.Server {
	t.Helper()
	polls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "login_approval_binding", Value: "b", Path: "/api/v1/auth/login-approval", HttpOnly: true}) //nolint:gosec // plain HTTP test server, Secure would stop the jar resending it
			_, _ = w.Write([]byte(`{"approval_required":true,"approval_id":"la_1","approval_match":37}`))
		case "/api/v1/auth/login-approval/poll":
			if c, err := r.Cookie("login_approval_binding"); err != nil || c.Value != "b" {
				t.Errorf("poll without the approval cookie")
			}
			polls++
			if polls == 1 || pendingForever {
				_, _ = w.Write([]byte(`{"status":"pending"}`))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "session_token", Value: "s", Path: "/", HttpOnly: true}) //nolint:gosec // plain HTTP test server
			_, _ = w.Write([]byte(final))
		case "/api/v1/auth/tokens":
			_, _ = w.Write([]byte(`[]`))
		case "/api/v1/instance":
			w.WriteHeader(http.StatusNotFound)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSessionLogin_WaitsForNewDeviceApproval(t *testing.T) {
	approvalPollInterval = time.Millisecond
	t.Cleanup(func() { approvalPollInterval = 2 * time.Second })
	tests := []struct {
		name     string
		final    string
		wantExit int
		wantErr  string
	}{
		{"approved", `{"status":"approved","username":"admin"}`, exitOK, ""},
		{"denied", `{"status":"denied"}`, exitAPIError, "denied"},
		{"expired", `{"status":"expired"}`, exitAPIError, "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := approvalServer(t, tt.final, false)
			var stdout, stderr bytes.Buffer
			args := []string{"tokens", "list", "--username", "admin", "--password", "x", "--api-url", srv.URL}
			got := run("levelrail-cli-test", args, &stdout, &stderr, func(k string) (string, bool) { return "1", k == envLoginApprovalWait })
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			for _, want := range []string{"la_1", "37", srv.URL + "/approve-sign-in?request=la_1", "Ctrl+C", "--code XXXX-XXXX"} {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want %q", stderr.String(), want)
				}
			}
			if tt.wantErr != "" && !strings.Contains(stderr.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantErr)
			}
		})
	}
}

func TestSessionLogin_ApprovalTimesOutWithRetryHint(t *testing.T) {
	approvalPollInterval = time.Millisecond
	t.Cleanup(func() { approvalPollInterval = 2 * time.Second })
	srv := approvalServer(t, "", true)
	var stdout, stderr bytes.Buffer
	args := []string{"auth", "login", "--username", "admin", "--password", "x", "--api-url", srv.URL, "--wait", "--approval-timeout", "20ms"}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
	}
	if !strings.Contains(stderr.String(), "nobody approved the sign-in within") || !strings.Contains(stderr.String(), "--code") {
		t.Errorf("stderr = %q, want the timeout and the retry hint", stderr.String())
	}
}

func TestSessionLogin_NonTTYFailsAtOnceWithTheFix(t *testing.T) {
	srv := approvalServer(t, "", true)
	var stdout, stderr bytes.Buffer
	args := []string{"auth", "login", "--username", "admin", "--password", "x", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
	}
	out := stderr.String()
	if strings.Count(out, "\n") != 1 || !strings.Contains(out, "approval required") || !strings.Contains(out, "--code XXXX-XXXX") || strings.Contains(out, "Waiting") {
		t.Errorf("stderr = %q, want one error line naming the fix", out)
	}
}

func TestSessionLogin_OpenFlag(t *testing.T) {
	approvalPollInterval = time.Millisecond
	t.Cleanup(func() { approvalPollInterval = 2 * time.Second })
	tests := []struct {
		name     string
		extra    []string
		wantOpen bool
	}{
		{"explicit open", []string{"--open"}, true},
		{"explicit off", []string{"--open=false"}, false},
		{"auto is off in scripts", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opened string
			openBrowserFunc = func(_ context.Context, u string) error { opened = u; return nil }
			t.Cleanup(func() { openBrowserFunc = openBrowser })
			srv := approvalServer(t, `{"status":"denied"}`, false)
			var stdout, stderr bytes.Buffer
			args := append([]string{"auth", "login", "--username", "admin", "--password", "x", "--api-url", srv.URL, "--wait"}, tt.extra...)
			run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if (opened != "") != tt.wantOpen {
				t.Fatalf("opened = %q, want open=%v", opened, tt.wantOpen)
			}
			if tt.wantOpen && opened != srv.URL+"/approve-sign-in?request=la_1" {
				t.Errorf("opened %q", opened)
			}
		})
	}
}

func TestAuthLogin_PairingCode(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantExit int
		wantOut  string
	}{
		{"redeemed", 201, `{"token":"tok-secret","token_id":"tk_1","username":"admin","abilities":["read"],"expires_at":"2027-01-01T00:00:00Z"}`, exitOK, "logged in as"},
		{"refused", 400, `{"error":"this pairing code is invalid or has expired","code":"pairing_invalid"}`, exitAPIError, ""},
		{"rate limited", 429, `{"error":"slow down"}`, exitAPIError, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/auth/cli-pairing/redeem" {
					var got struct{ Code string }
					_ = json.NewDecoder(r.Body).Decode(&got)
					if got.Code != "ABCD-EF23" {
						t.Errorf("code = %q", got.Code)
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(tt.body))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer srv.Close()
			home := t.TempDir()
			t.Setenv("HOME", home)
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test-pair", []string{"auth", "login", "--code", "ABCD-EF23", "--api-url", srv.URL}, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tt.wantExit, stderr.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "tok-secret") {
				t.Errorf("output leaks the token: %q %q", stdout.String(), stderr.String())
			}
			if tt.wantOut != "" && !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q", stdout.String())
			}
			_, err := os.Stat(filepath.Join(home, ".config", "levelrail-cli-test-pair", credentialsFileName))
			if (err == nil) != (tt.wantExit == exitOK) {
				t.Errorf("credentials saved = %v, want %v", err == nil, tt.wantExit == exitOK)
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
		{"approve with the matching number", []string{"auth", "approve", "la_1", "--match", "42"}, map[string]string{
			"POST /api/v1/auth/login-approvals/la_1/approve": "",
		}, exitOK, []string{"approved sign-in la_1"}},
		{"approve without a number is refused", []string{"auth", "approve", "la_1"}, nil, exitValidation, nil},
		{"approve with a number that is not two digits is refused", []string{"auth", "approve", "la_1", "--match", "7"}, nil, exitValidation, nil},
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
				if strings.HasSuffix(r.URL.Path, "/approve") {
					var got struct {
						Match int `json:"match"`
					}
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil || got.Match != 42 {
						t.Errorf("approve body match = %d (%v), want 42", got.Match, err)
					}
				}
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
