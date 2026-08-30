package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// newListEchoServer starts a test server that records the request path in
// gotPath (when non-nil) and responds with items JSON-encoded.
func newListEchoServer[T any](t *testing.T, gotPath *string, items T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(items)
	}))
}

// newNoContentEchoServer starts a test server that records the request
// method and path and responds 204 No Content, for action-verb endpoints
// with no response body.
func newNoContentEchoServer(t *testing.T) (srv *httptest.Server, gotPath, gotMethod *string) {
	t.Helper()
	gotPath, gotMethod = new(string), new(string)
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotPath, *gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	return srv, gotPath, gotMethod
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

// assertEnvGetNoneSet runs "<verbArgs...> env-get <id>" against a server
// returning an empty env map and asserts the empty-set message, the
// shared shape every shared-env-var tier's own env-get test already
// establishes (apps organizations/environments/projects).
func assertEnvGetNoneSet(t *testing.T, verbArgs []string, id string) {
	t.Helper()
	srv := newListEchoServer(t, nil, map[string]string{})
	defer srv.Close()

	args := append(append([]string{}, verbArgs...), "env-get", id, "--api-url", srv.URL)
	stdout, _ := runCLIExpectOK(t, args)
	if !strings.Contains(stdout, "no env vars set") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

// assertEnvSet runs "<verbArgs...> env-set <id> --var LOG_LEVEL=info"
// and asserts the request/response shape every shared-env-var tier's own
// env-set establishes: a PUT to wantPath carrying the var, and a
// "<label> env vars replaced (1 set)" confirmation.
func assertEnvSet(t *testing.T, verbArgs []string, id, wantPath, label string) {
	t.Helper()
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	args := append(append([]string{}, verbArgs...), "env-set", id, "--var", "LOG_LEVEL=info", "--api-url", srv.URL)
	stdout, _ := runCLIExpectOK(t, args)
	if gotMethod != http.MethodPut || gotPath != wantPath {
		t.Errorf("request = %s %s, want PUT %s", gotMethod, gotPath, wantPath)
	}
	if gotBody["LOG_LEVEL"] != "info" {
		t.Errorf("request body = %+v, want LOG_LEVEL=info", gotBody)
	}
	if want := fmt.Sprintf("%s env vars replaced (1 set)", label); !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// assertEnvSetNoVarsClearsAll runs "<verbArgs...> env-set <id>" with no
// --var flags and asserts every env var is cleared, the shared shape
// every shared-env-var tier's own env-set establishes.
func assertEnvSetNoVarsClearsAll(t *testing.T, verbArgs []string, id, label string) {
	t.Helper()
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	args := append(append([]string{}, verbArgs...), "env-set", id, "--api-url", srv.URL)
	stdout, _ := runCLIExpectOK(t, args)
	if len(gotBody) != 0 {
		t.Errorf("request body = %+v, want empty", gotBody)
	}
	if want := fmt.Sprintf("%s env vars replaced (0 set)", label); !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// assertClearAssignment runs "<verbArgs...>" (e.g. "apps clear-project
// web") against a server that echoes back an empty response and asserts
// the request was a PUT to wantPath with every body value empty, the
// shared shape every "clear-<tier>" verb pair establishes (apps
// clear-environment, apps clear-project, databases clear-project).
func assertClearAssignment(t *testing.T, verbArgs []string, wantPath, wantMsg string) {
	t.Helper()
	var gotMethod, gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{})
	}))
	defer srv.Close()

	args := append(append([]string{}, verbArgs...), "--api-url", srv.URL)
	stdout, _ := runCLIExpectOK(t, args)
	if gotMethod != http.MethodPut || gotPath != wantPath {
		t.Errorf("request = %s %s, want PUT %s", gotMethod, gotPath, wantPath)
	}
	for k, v := range gotBody {
		if v != "" {
			t.Errorf("request body %s = %q, want empty", k, v)
		}
	}
	if !strings.Contains(stdout, wantMsg) {
		t.Errorf("stdout = %q, want %q", stdout, wantMsg)
	}
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
