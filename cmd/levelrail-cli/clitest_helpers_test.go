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

// runCLIExpectValidationError runs the CLI with args, asserts it exits
// exitValidation, and returns stderr for further assertions.
func runCLIExpectValidationError(t *testing.T, args []string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", args, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
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

// newEchoServer starts a test server that records the request method and
// path (when non-nil) and responds 200 OK with body JSON-encoded. It's the
// method-aware sibling of newListEchoServer, for mutation verbs (clear,
// disable, ...) whose test also needs to assert the HTTP method.
func newEchoServer[T any](t *testing.T, gotMethod, gotPath *string, body T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotMethod != nil {
			*gotMethod = r.Method
		}
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
}

// testEnvGet runs "<cliArgs...> env-get <id>" against a server that reports
// one env var set, and asserts the request path and that the var is shown.
func testEnvGet(t *testing.T, cliArgs []string, id, apiPath string) {
	t.Helper()
	var gotPath string
	srv := newListEchoServer(t, &gotPath, map[string]string{"LOG_LEVEL": "info"})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, append(append([]string{}, cliArgs...), "env-get", id, "--api-url", srv.URL))
	wantPath := apiPath + "/env"
	if gotPath != wantPath {
		t.Errorf("path = %q, want %s", gotPath, wantPath)
	}
	if !strings.Contains(stdout, "LOG_LEVEL") || !strings.Contains(stdout, "info") {
		t.Errorf("stdout = %q, want the env var listed", stdout)
	}
}

// testEnvGetNoneSet runs "<cliArgs...> env-get <id>" against a server that
// reports no env vars, and asserts the empty-set message.
func testEnvGetNoneSet(t *testing.T, cliArgs []string, id string) {
	t.Helper()
	srv := newListEchoServer(t, nil, map[string]string{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, append(append([]string{}, cliArgs...), "env-get", id, "--api-url", srv.URL))
	if !strings.Contains(stdout, "no env vars set") {
		t.Errorf("stdout = %q, want the empty-set message", stdout)
	}
}

// testEnvSet runs "<cliArgs...> env-set <id> --var LOG_LEVEL=info" and
// asserts the request method, path, body, and replace confirmation. noun is
// the resource name used in the confirmation message (e.g. "project").
func testEnvSet(t *testing.T, cliArgs []string, id, apiPath, noun string) {
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

	stdout, _ := runCLIExpectOK(t, append(append([]string{}, cliArgs...), "env-set", id, "--var", "LOG_LEVEL=info", "--api-url", srv.URL))
	wantPath := apiPath + "/env"
	if gotMethod != http.MethodPut || gotPath != wantPath {
		t.Errorf("request = %s %s, want PUT %s", gotMethod, gotPath, wantPath)
	}
	if gotBody["LOG_LEVEL"] != "info" {
		t.Errorf("request body = %+v, want LOG_LEVEL=info", gotBody)
	}
	wantMsg := fmt.Sprintf("%s %q env vars replaced (1 set)", noun, id)
	if !strings.Contains(stdout, wantMsg) {
		t.Errorf("stdout = %q, want a replace confirmation", stdout)
	}
}

// testEnvSetClearsAll runs "<cliArgs...> env-set <id>" with no --var flags
// and asserts the request body is empty and the replace confirmation shows
// zero vars set.
func testEnvSetClearsAll(t *testing.T, cliArgs []string, id, noun string) {
	t.Helper()
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(gotBody)
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, append(append([]string{}, cliArgs...), "env-set", id, "--api-url", srv.URL))
	if len(gotBody) != 0 {
		t.Errorf("request body = %+v, want empty", gotBody)
	}
	wantMsg := fmt.Sprintf("%s %q env vars replaced (0 set)", noun, id)
	if !strings.Contains(stdout, wantMsg) {
		t.Errorf("stdout = %q, want a replace confirmation", stdout)
	}
}

// testMetricsMissingMetric runs "<cmdArgs...> <id>" (no --metric) and
// asserts the "--metric is required" validation error every "* metrics"
// command shares (metrics_cmd.go's own runMetricsCommand), for "apps
// metrics"/"databases metrics"/"nodes metrics" alike.
func testMetricsMissingMetric(t *testing.T, cmdArgs []string, id string) {
	t.Helper()
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", append(append([]string{}, cmdArgs...), id), &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d", got, exitValidation)
	}
	if !strings.Contains(stderr.String(), "--metric is required") {
		t.Errorf("stderr = %q, want a missing --metric validation error", stderr.String())
	}
}

// testMetricsNotFound runs "<cmdArgs...> <id> --metric cpu_percent"
// against a server answering 404 with errBody, and asserts wantSubstr
// (the server's own error message) reaches stderr unchanged.
func testMetricsNotFound(t *testing.T, cmdArgs []string, id, errBody, wantSubstr string) {
	t.Helper()
	srv := newJSONErrorServer(t, http.StatusNotFound, errBody)
	stderr := runCLIExpectAPIError(t, append(append([]string{}, cmdArgs...), id, "--metric", "cpu_percent", "--api-url", srv.URL))
	if !strings.Contains(stderr, wantSubstr) {
		t.Errorf("stderr = %q, want %q", stderr, wantSubstr)
	}
}

// testMetricsHelp runs "<cmdArgs...> -h" and asserts wantSubstr appears
// in the usage text.
func testMetricsHelp(t *testing.T, cmdArgs []string, wantSubstr string) {
	t.Helper()
	_, stderr := runCLIExpectOK(t, append(append([]string{}, cmdArgs...), "-h"))
	if !strings.Contains(stderr, wantSubstr) {
		t.Errorf("stderr = %q, want usage text", stderr)
	}
}

// testMetricsQuerySuccess runs "<cmdArgs...> <id> --metric <metric>"
// against a server returning an AppMetricsResource-shaped body, and
// asserts the request path/query string and that the metric name and
// point value both appear in the human table output. Shared by "apps
// metrics"/"databases metrics" tests, whose success path is otherwise
// identical: QueryAppMetrics and QueryDatabaseMetrics return the exact
// same AppMetricsResource wire shape (see internal/apiclient's own doc
// comment on QueryDatabaseMetrics for why). "nodes metrics" returns the
// differently-shaped NodeMetricsResource (an extra resource_count field),
// so it keeps its own test rather than reusing this helper.
func testMetricsQuerySuccess(t *testing.T, cmdArgs []string, id, wantPath, metric string, value float64, wantValueText string) {
	t.Helper()
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(appMetricsResource{Metric: metric, Points: []metricPointResource{{Value: value, Count: 1}}})
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, append(append([]string{}, cmdArgs...), id, "--metric", metric, "--api-url", srv.URL))
	if gotPath != wantPath {
		t.Errorf("path = %q, want %s", gotPath, wantPath)
	}
	if !strings.Contains(gotQuery, "metric="+metric) {
		t.Errorf("query = %q, want metric=%s", gotQuery, metric)
	}
	if !strings.Contains(stdout, metric) || !strings.Contains(stdout, wantValueText) {
		t.Errorf("stdout = %q, want the metric name and point value", stdout)
	}
}

// testMetricsNoName runs cmdArgs with no positional id/name argument and
// asserts the shared "requires exactly one" usage error every
// apiFlagSet-based single-argument command produces via requireOneArg.
func testMetricsNoName(t *testing.T, cmdArgs []string) {
	t.Helper()
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", cmdArgs, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
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
