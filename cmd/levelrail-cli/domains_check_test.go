package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cutoververify"
)

func TestRun_DomainsCheck(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, domainCheckResource{
		Domain: "app.example.com", Status: "connected", Resolved: true,
		ExpectedHost: "203.0.113.10", ResolvedHosts: []string{"203.0.113.10"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "check", "web", "app.example.com", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/domains/app.example.com/check" {
		t.Errorf("path = %q, want /api/v1/apps/web/domains/app.example.com/check", gotPath)
	}
	if !strings.Contains(stdout, "connected") {
		t.Errorf("stdout = %q, want the connected status shown", stdout)
	}
}

func TestRun_DomainsCheck_NotResolving(t *testing.T) {
	srv := newListEchoServer(t, nil, domainCheckResource{
		Domain: "app.example.com", Status: "not_resolving", Resolved: false, ExpectedHost: "203.0.113.10",
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"domains", "check", "web", "app.example.com", "--api-url", srv.URL})
	if !strings.Contains(stdout, "not_resolving") {
		t.Errorf("stdout = %q, want the not_resolving status shown", stdout)
	}
}

func TestRun_DomainsCheck_MissingArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "check", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
	if !strings.Contains(stderr.String(), "requires an app name and a domain") {
		t.Errorf("stderr = %q, want a missing-args usage error", stderr.String())
	}
}

func TestRun_DomainsCheck_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, 404, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"domains", "check", "ghost", "app.example.com", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want the server's not-found message", stderr)
	}
}

func TestRun_DomainsCheck_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "check", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "domains check") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}

func cutoverVerifyServer(t *testing.T, gotQuery *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*gotQuery = r.URL.RawQuery
		rep := cutoververify.Report{Domain: "ok.example.com", Verdict: cutoververify.Pass, Summary: "ok.example.com is ready: every check passed",
			Lines: []cutoververify.Line{{ID: "dns", Verdict: cutoververify.Pass, Title: "DNS points at this server", Detail: "resolves to 203.0.113.10", Next: "Nothing to do."}}}
		if strings.Contains(r.URL.Path, "old.example.com") {
			rep = cutoververify.Report{Domain: "old.example.com", Verdict: cutoververify.Fail, Summary: "old.example.com is not ready: 1 item to fix first",
				Lines: []cutoververify.Line{{ID: "dns", Verdict: cutoververify.Fail, Title: "DNS points at this server", Detail: "resolves to 76.76.21.21", Next: "Change the record to 203.0.113.10."}}}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rep)
	}))
}

func TestRun_DomainsCheckCutover_OneMismatchFails(t *testing.T) {
	var q string
	srv := cutoverVerifyServer(t, &q)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"domains", "check", "--cutover", "ok.example.com", "old.example.com", "--expect-ip", "203.0.113.10", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitCheckFailed {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitCheckFailed, stderr.String())
	}
	for _, want := range []string{"PASS  ok.example.com is ready", "FAIL  old.example.com is not ready", "next: Change the record to 203.0.113.10."} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if q != "expect_ip=203.0.113.10" {
		t.Errorf("query = %q, want the expect_ip forwarded", q)
	}
}

func TestRun_DomainsCheckCutover_AllPassJSON(t *testing.T) {
	var q string
	srv := cutoverVerifyServer(t, &q)
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "check", "--cutover", "ok.example.com", "--json", "--api-url", srv.URL})
	var reps []cutoververify.Report
	if err := json.Unmarshal([]byte(stdout), &reps); err != nil || len(reps) != 1 || reps[0].Verdict != cutoververify.Pass {
		t.Fatalf("stdout = %q err = %v", stdout, err)
	}
}

func TestRun_DomainsCheckCutover_NeedsDomain(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "check", "--cutover"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
}
