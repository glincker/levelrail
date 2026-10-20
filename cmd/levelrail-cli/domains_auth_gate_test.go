package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestRun_DomainsAuthGate(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody setDomainAuthGateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(domainAuthGateResource{
			Domain: "app.example.com", Configured: true, Enabled: true, AuthURL: "http://auth:9091",
			CheckPath: "/check", CopyHeaders: []string{"X-User", "X-Email"}, DefaultTimeoutSeconds: 5,
		})
	}))
	defer srv.Close()

	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantOut    string
	}{
		{"set", []string{"set", "web", "app.example.com", "--url", "http://auth:9091", "--check-path", "/check", "--copy-header", "X-User,X-Email", "--copy-header", "X-Name"}, http.MethodPut, "copy_headers: X-User, X-Email"},
		{"get", []string{"get", "web", "app.example.com"}, http.MethodGet, "status:       on"},
		{"clear", []string{"clear", "web", "app.example.com"}, http.MethodDelete, "auth gate removed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"domains", "auth-gate"}, tt.args...)
			args = append(args, "--api-url", srv.URL)
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
				t.Fatalf("exit = %d (stdout=%q stderr=%q)", got, stdout.String(), stderr.String())
			}
			if gotMethod != tt.wantMethod || gotPath != "/api/v1/apps/web/domains/app.example.com/auth-gate" {
				t.Errorf("request = %s %s", gotMethod, gotPath)
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
	if !reflect.DeepEqual(gotBody.CopyHeaders, []string{"X-User", "X-Email", "X-Name"}) {
		t.Errorf("set body headers = %v", gotBody.CopyHeaders)
	}
}

func TestRun_DomainsAuthGateSetRequiresURL(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "auth-gate", "set", "web", "app.example.com", "--api-url", "http://127.0.0.1:1"}, &stdout, &stderr, envMap())
	if got != exitUsage || !strings.Contains(stderr.String(), "requires --url") {
		t.Errorf("exit = %d stderr = %q, want usage error naming --url", got, stderr.String())
	}
}

func TestRun_DomainsAuthGateBuiltinAndLog(t *testing.T) {
	var gotBody setDomainAuthGateRequest
	var gotPaths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPaths = append(gotPaths, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/events") {
			_ = json.NewEncoder(w).Encode(authGateEventsResponse{Domain: "app.example.com", Events: []apiAuthGateEvent{{UserEmail: "a@example.com", Decision: "denied", Reason: "not_allowed", Count: 2}}})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(domainAuthGateResource{Domain: "app.example.com", Configured: true, Enabled: true, Provider: "builtin",
			Allow: []authGateRule{{Kind: "user", Subject: "u1", Label: "a@example.com"}}})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	args := []string{"domains", "auth-gate", "set", "web", "app.example.com", "--provider", "builtin", "--allow", "a@example.com,b@example.com", "--allow-team", "ops", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("set exit = %d stderr %q", got, stderr.String())
	}
	want := []authGateRule{{Kind: "user", Subject: "a@example.com"}, {Kind: "user", Subject: "b@example.com"}, {Kind: "team", Subject: "ops"}}
	if gotBody.Provider != "builtin" || !reflect.DeepEqual(gotBody.Allow, want) || !strings.Contains(stdout.String(), "allow:        user:a@example.com") {
		t.Errorf("body = %+v stdout %q", gotBody, stdout.String())
	}

	stdout.Reset()
	args = []string{"domains", "auth-gate", "log", "web", "app.example.com", "--since", "7d", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("log exit = %d stderr %q", got, stderr.String())
	}
	if last := gotPaths[len(gotPaths)-1]; last != "GET /api/v1/apps/web/domains/app.example.com/auth-gate/events?since=7d" || !strings.Contains(stdout.String(), "not_allowed") {
		t.Errorf("log request %q stdout %q", last, stdout.String())
	}

	stderr.Reset()
	args = []string{"domains", "auth-gate", "set", "web", "app.example.com", "--url", "http://auth", "--allow", "a@example.com", "--api-url", srv.URL}
	if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitUsage {
		t.Errorf("external with --allow exit = %d", got)
	}
}
