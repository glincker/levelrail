package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_TokensCreate_RootCombinedFailsBeforeSignIn(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tokens", "create", "--name", "ci", "--abilities", "root,read,write,deploy", "--username", "admin", "--password", "x", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
	if !strings.Contains(stderr.String(), "root cannot be combined") {
		t.Errorf("stderr = %q, want the root exclusivity error", stderr.String())
	}
	if calls != 0 {
		t.Errorf("control plane received %d requests, want 0 (no sign-in or approval before validation)", calls)
	}
}
