package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const hostedCLIURL = "postgres://app:s3cr3tPw@db.example.com:5432/d1" //nolint:gosec // fake credential

func hostedServer(t *testing.T, blocked bool, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls = append(*calls, r.URL.Path)
		var req apiclient.HostedImportRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.URL != hostedCLIURL && r.Method == http.MethodPost {
			t.Errorf("api received url %q", req.URL)
		}
		switch {
		case strings.HasSuffix(r.URL.Path, "/hosted/preflight"):
			checks := []apiclient.HostedCheck{
				{ID: "tls", Severity: "ok", Message: "encrypted"},
				{ID: "provider_extensions", Severity: "warn", Message: "skipping neon", NextAction: "none needed"},
			}
			if blocked {
				checks = append(checks, apiclient.HostedCheck{ID: "policy_roles", Severity: "block", Message: "roles missing", NextAction: "create the roles"})
			}
			_ = json.NewEncoder(w).Encode(apiclient.HostedPreflight{Database: "mydb", Source: "db.example.com:5432/d1 (password masked)", Blocked: blocked, TargetVersion: "16", Checks: checks})
		case strings.HasSuffix(r.URL.Path, "/hosted/run"):
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.DataCopyStatus{Database: "mydb", Engine: "postgres", Status: "verified", Checked: 3})
		}
	}))
}

func TestImportHostedDryRunGroupsFindings(t *testing.T) {
	var calls []string
	srv := hostedServer(t, true, &calls)
	defer srv.Close()
	var out, errb bytes.Buffer
	env := importEnv(map[string]string{"APP_API_TOKEN": "cp", "SRC": hostedCLIURL})
	code := runDatabasesImportHosted("cli", []string{"mydb", "--from-env", "SRC", "--dry-run", "--writes-frozen", "--api-url", srv.URL}, strings.NewReader(""), &out, &errb, env)
	if code != exitCheckFailed {
		t.Fatalf("exit %d, want the blocked exit: %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{"BLOCKERS", "[policy_roles]", "fix: create the roles", "WARNINGS", "INFO", "Blocked"} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout missing %q: %s", want, got)
		}
	}
	if strings.Index(got, "BLOCKERS") > strings.Index(got, "WARNINGS") || strings.Contains(got+errb.String(), "s3cr3tPw") {
		t.Errorf("order or leak: %s", got)
	}
	if len(calls) != 1 {
		t.Errorf("dry run made calls %v, want preflight only", calls)
	}
}

func TestImportHostedRefusesWithoutWritesFrozen(t *testing.T) {
	var calls []string
	srv := hostedServer(t, false, &calls)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runDatabasesImportHosted("cli", []string{"mydb", "--from-stdin", "--api-url", srv.URL}, strings.NewReader(hostedCLIURL+"\n"), &out, &errb, importEnv(map[string]string{"APP_API_TOKEN": "cp"}))
	if code != exitValidation {
		t.Fatalf("exit %d, want validation", code)
	}
	if !strings.Contains(errb.String(), "not frozen") || !strings.Contains(errb.String(), "heroku maintenance:on") || len(calls) != 0 {
		t.Errorf("stderr %q calls %v", errb.String(), calls)
	}
}

func TestImportHostedRunJSONAndBlockedStop(t *testing.T) {
	var calls []string
	srv := hostedServer(t, false, &calls)
	defer srv.Close()
	var out, errb bytes.Buffer
	env := importEnv(map[string]string{"APP_API_TOKEN": "cp", "SRC": hostedCLIURL})
	code := runDatabasesImportHosted("cli", []string{"mydb", "--from-env", "SRC", "--writes-frozen", "--json", "--api-url", srv.URL}, strings.NewReader(""), &out, &errb, env)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var st apiclient.DataCopyStatus
	if err := json.Unmarshal(out.Bytes(), &st); err != nil || st.Status != "verified" {
		t.Fatalf("json %q: %v", out.String(), err)
	}
	if len(calls) != 2 || strings.Contains(out.String()+errb.String(), "s3cr3tPw") {
		t.Errorf("calls %v", calls)
	}

	calls = nil
	blockedSrv := hostedServer(t, true, &calls)
	defer blockedSrv.Close()
	out.Reset()
	errb.Reset()
	code = runDatabasesImportHosted("cli", []string{"mydb", "--from-env", "SRC", "--writes-frozen", "--api-url", blockedSrv.URL}, strings.NewReader(""), &out, &errb, env)
	if code != exitCheckFailed || len(calls) != 1 {
		t.Errorf("blocked run exit %d calls %v", code, calls)
	}
}

func TestImportHostedNeverTakesTheURLAsAnArgument(t *testing.T) {
	var out, errb bytes.Buffer
	code := runDatabasesImportHosted("cli", []string{"mydb", hostedCLIURL, "--writes-frozen"}, strings.NewReader(""), &out, &errb, importEnv(nil))
	if code != exitUsage || strings.Contains(errb.String(), "s3cr3tPw") {
		t.Fatalf("exit %d stderr %q", code, errb.String())
	}
	code = runDatabasesImportHosted("cli", []string{"mydb", "--from-env", "SRC", "--from-stdin"}, strings.NewReader(""), &out, &errb, importEnv(nil))
	if code != exitUsage {
		t.Fatalf("both sources: exit %d", code)
	}
	if code = runDatabasesImportHosted("cli", []string{"mydb", "--from-env", "MISSING", "--writes-frozen"}, strings.NewReader(""), &out, &errb, importEnv(nil)); code != exitValidation {
		t.Fatalf("empty url: exit %d", code)
	}
}
