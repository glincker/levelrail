package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AuthWhoami(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		args       []string
		wantExit   int
		wantStdout []string
		wantStderr string
	}{
		{name: "token table", status: 200, body: `{"kind":"token","name":"ci","abilities":["read","deploy"],"expires_at":""}`, wantExit: exitOK, wantStdout: []string{"token", "ci", "read, deploy"}},
		{name: "json", status: 200, body: `{"kind":"session","name":"a@b.c","abilities":["root"],"expires_at":"2026-10-05T00:00:00Z"}`, args: []string{"--json"}, wantExit: exitOK, wantStdout: []string{`"kind"`, `"a@b.c"`}},
		{name: "query", status: 200, body: `{"kind":"token","name":"ci","abilities":[],"expires_at":""}`, args: []string{"--query", "name"}, wantExit: exitOK, wantStdout: []string{"ci"}},
		{name: "unauthorized", status: 401, body: `{"error":"invalid token"}`, wantExit: exitAPIError, wantStderr: "invalid token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAuth string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAuth = r.Header.Get("Authorization")
				if r.Method != http.MethodGet || r.URL.Path != "/api/v1/auth/whoami" {
					t.Errorf("request = %s %s, want GET /api/v1/auth/whoami", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			var stdout, stderr bytes.Buffer
			args := append([]string{"auth", "whoami", "--token", "sometoken", "--api-url", srv.URL}, tt.args...)
			got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
			if got != tt.wantExit {
				t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, tt.wantExit, stdout.String(), stderr.String())
			}
			if gotAuth != "Bearer sometoken" {
				t.Errorf("Authorization = %q, want Bearer sometoken", gotAuth)
			}
			for _, w := range tt.wantStdout {
				if !strings.Contains(stdout.String(), w) {
					t.Errorf("stdout = %q, want it to contain %q", stdout.String(), w)
				}
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRun_AuthWhoami_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "whoami", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "auth whoami") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func TestRun_Auth_UnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"auth", "bogus"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown auth subcommand") {
		t.Errorf("stderr = %q, want an unknown-subcommand error", stderr.String())
	}
}
