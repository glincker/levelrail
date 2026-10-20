package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFirstRunLoginArgs(t *testing.T) {
	tests := []struct {
		name       string
		apiURL     string
		wantDevice bool
	}{
		{"https needs no device flow", "https://deploy.example.com", false},
		{"loopback http is fine", "http://localhost:8080", false},
		{"loopback ip is fine", "http://127.0.0.1:8080", false},
		{"remote http needs the device flow", "http://203.0.113.7:8080", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := strings.Join(firstRunLoginArgs(tt.apiURL, "default"), " ")
			if has := strings.Contains(got, "--device"); has != tt.wantDevice {
				t.Fatalf("args %q: device flag = %v, want %v", got, has, tt.wantDevice)
			}
			if !strings.Contains(got, "--api-url "+tt.apiURL) {
				t.Fatalf("args %q missing the server URL", got)
			}
		})
	}
}

func TestOfferFirstRunLogin_Declined(t *testing.T) {
	var stdout, stderr bytes.Buffer
	_, _, ok := offerFirstRunLogin("lr-test", "default", "http://localhost:8080", envMap(), strings.NewReader("n\n"), &stdout, &stderr)
	if ok {
		t.Fatal("declining must not sign in")
	}
	if !strings.Contains(stderr.String(), "Sign in now?") {
		t.Fatalf("stderr = %q, want the sign-in question", stderr.String())
	}
}

func TestRun_NoCredentialsNamesTheFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	run("levelrail-cli-test", []string{"apps", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	for _, want := range []string{"not signed in", "login --api-url " + srv.URL, "--token"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
		}
	}
}

func TestRun_AppsDeploysRollbackAlias(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run("levelrail-cli-test", []string{"apps", "deploys", "rollback", "-h"}, &stdout, &stderr, envMap())
	if strings.Contains(stderr.String(), "unknown apps deploys subcommand") {
		t.Fatalf("rollback must be an alias of rollback-to, got %q", stderr.String())
	}
}
