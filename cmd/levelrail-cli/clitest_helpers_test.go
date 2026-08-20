package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// runCLIExpectOK runs the CLI with args, asserts it exits exitOK, and
// returns stdout/stderr for further assertions.
func runCLIExpectOK(t *testing.T, args []string) (stdout, stderr string) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	got := run("levelrail-cli-test", args, &outBuf, &errBuf, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, outBuf.String(), errBuf.String())
	}
	return outBuf.String(), errBuf.String()
}

// runCLIExpectAPIError runs the CLI with args, asserts it exits
// exitAPIError, and returns stderr for further assertions.
func runCLIExpectAPIError(t *testing.T, args []string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	if got != exitAPIError {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
	}
	return stderr.String()
}

// newJSONErrorServer starts a test server that always responds with status
// and body, closed automatically when the test ends.
func newJSONErrorServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// assertUsageErrorMissingName runs "apps <subcommand> <verb>" for each verb
// and asserts a missing-name usage error.
func assertUsageErrorMissingName(t *testing.T, subcommand string, verbs []string) {
	t.Helper()
	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", []string{"apps", subcommand, verb}, &stdout, &stderr, envMap())
			if got != exitUsage {
				t.Fatalf("exit = %d, want %d", got, exitUsage)
			}
			if !strings.Contains(stderr.String(), "requires exactly one") {
				t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
			}
		})
	}
}
