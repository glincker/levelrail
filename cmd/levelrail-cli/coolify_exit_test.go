package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/coolifyexit"
	"github.com/GLINCKER/levelrail/internal/coolifyexit/exportfixture"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func loopbackEnv() func(string) (string, bool) {
	return func(k string) (string, bool) {
		if k == platformimport.AllowLoopbackEnv {
			return "true", true
		}
		return "", false
	}
}

func TestRun_MigrateCoolify_DryRunAgainstExportFixture(t *testing.T) {
	srv := exportfixture.New(t)
	var stdout, stderr bytes.Buffer
	code := run("lr", []string{"migrate", "coolify", "--url", srv.URL, "--token", exportfixture.Token, "--dry-run"}, &stdout, &stderr, loopbackEnv())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"dry run, nothing was created", "SKIPPED:", "MANUAL:", "[database-data] main-db", "lr migrate db-copy", "DNS switch order:", "Cutover checklist:", "Keep Coolify as the fallback"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q", want)
		}
	}
	for _, leak := range []string{"fake-db-password", "fake-session-secret", "fake-webhook-secret"} {
		if strings.Contains(out, leak) {
			t.Errorf("output leaks %q", leak)
		}
	}
	if v := srv.Violations(); len(v) != 0 {
		t.Errorf("source was written to: %v", v)
	}
}

func TestRun_MigrateCoolify_DryRunJSON(t *testing.T) {
	srv := exportfixture.New(t)
	var stdout, stderr bytes.Buffer
	code := run("lr", []string{"migrate", "coolify", "--url", srv.URL, "--token", exportfixture.Token, "--dry-run", "--json", "--only", "umami"}, &stdout, &stderr, loopbackEnv())
	if code != exitOK {
		t.Fatalf("exit = %d, stderr = %q", code, stderr.String())
	}
	var rep coolifyexit.Report
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("not json: %v\n%s", err, stdout.String())
	}
	if rep.Mode != coolifyexit.ModeDryRun || len(rep.Rows) == 0 {
		t.Errorf("report = %+v", rep)
	}
	for _, r := range rep.Rows {
		if r.Kind == "app" && r.Name != "umami" {
			t.Errorf("--only leaked app %q", r.Name)
		}
	}
}

func TestRun_MigrateCoolify_DryRunRejectsApply(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run("lr", []string{"migrate", "coolify", "--url", "http://127.0.0.1:1", "--token", "x", "--dry-run", "--apply"}, &stdout, &stderr, envMap())
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d", code, exitValidation)
	}
}
