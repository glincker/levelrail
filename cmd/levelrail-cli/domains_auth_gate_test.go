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
