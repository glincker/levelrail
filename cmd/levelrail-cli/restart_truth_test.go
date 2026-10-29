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

// restartServer answers the restart POST and then reports the newest deploy with outcome.
func restartServer(t *testing.T, outcome string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/restart":
			_ = json.NewEncoder(w).Encode(appResource{Name: "web", Image: "levelrail/web:1", Port: 3000})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/deploys/latest":
			_, _ = io.WriteString(w, `{"id":"d-restart","outcome":"`+outcome+`","outcome_reason":"ReadinessFailed"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestRestartWaitReportsRealOutcome(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		outcome string
		want    int
	}{
		{"failed restart exits non-zero", []string{"apps", "restart", "web", "--wait"}, "failed", exitDeployNotHealthy},
		{"healthy restart exits zero", []string{"apps", "restart", "web", "--wait"}, "healthy", exitOK},
		{"top-level verb waits too", []string{"restart", "web", "--wait", "--json"}, "failed", exitDeployNotHealthy},
		{"without wait only the request counts", []string{"restart", "web"}, "failed", exitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := restartServer(t, tc.outcome)
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			got := run("lr", append(tc.args, "--api-url", srv.URL), &stdout, &stderr, envMap())
			if got != tc.want {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tc.want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRestartWaitTimeoutFromEnv(t *testing.T) {
	srv := restartServer(t, "in_progress")
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("lr", []string{"restart", "web", "--wait", "--api-url", srv.URL}, &stdout, &stderr, func(k string) (string, bool) { return "50ms", k == envRestartWaitTimeout })
	if got != exitDeployTimeout {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitDeployTimeout, stderr.String())
	}
	if !strings.Contains(stdout.String(), "timed out after 50ms") {
		t.Errorf("stdout = %q, want the env timeout in the message", stdout.String())
	}
}

func TestEnvUnsetClearsKeys(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		keys     []string
		wantEnv  string
		wantCode int
	}{
		{"removes one of two", map[string]string{"A": "1", "B": "2"}, []string{"A"}, `{"B":"2"}`, exitOK},
		{"last key sends an empty map", map[string]string{"A": "1"}, []string{"A"}, `{}`, exitOK},
		{"missing key changes nothing", map[string]string{"A": "1"}, []string{"Z"}, "", exitOK},
		{"secret is refused", map[string]string{"A": "1"}, []string{"TOKEN"}, "", exitValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var putEnv string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web":
					_ = json.NewEncoder(w).Encode(map[string]any{"name": "web", "image": "x:1", "env": tc.env, "secret_env": []string{"TOKEN"}})
				case r.URL.Path == "/api/v1/apps/web/env/bound":
					_, _ = io.WriteString(w, `[]`)
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web":
					var body struct {
						Env json.RawMessage `json:"env"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					putEnv = string(body.Env)
					_, _ = io.WriteString(w, `{"name":"web"}`)
				case r.URL.Path == "/api/v1/apps/web/pending-changes":
					_, _ = io.WriteString(w, `{"pending":false}`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer srv.Close()
			var stdout, stderr bytes.Buffer
			args := append([]string{"env", "unset", "web"}, tc.keys...)
			got := run("lr", append(args, "--api-url", srv.URL, "--json"), &stdout, &stderr, envMap())
			if got != tc.wantCode {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tc.wantCode, stdout.String(), stderr.String())
			}
			if putEnv != tc.wantEnv {
				t.Errorf("PUT env = %q, want %q", putEnv, tc.wantEnv)
			}
		})
	}
}
