package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/migrationgate"
)

type resumeAPI struct {
	srv       *httptest.Server
	calls     []string
	job       apiclient.MigrationJob
	connected apiclient.AppImportRequest
	staged    apiclient.AppImportItemsRequest
	retire    migrationgate.Report
	retireReq map[string]any
}

func newResumeAPI(t *testing.T) *resumeAPI {
	t.Helper()
	a := &resumeAPI{job: apiclient.MigrationJob{ID: "appimp-1", Platform: "coolify", SourceURL: "https://c.example.com", Phase: "attention",
		Total: 2, Done: 1, Failed: 1, States: map[string]int{"verified": 1, "stage-failed": 1}, Next: []string{"reconnect", "retry"},
		Items: []apiclient.MigrationJobItem{
			{SourceID: "a1", Name: "web", State: "verified"},
			{SourceID: "a2", Name: "api", State: "stage-failed", Reason: "image pull denied", Retryable: true},
		}}}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.calls = append(a.calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/progress"):
			_ = json.NewEncoder(w).Encode(a.job)
		case strings.HasSuffix(r.URL.Path, "/connect"):
			_ = json.NewDecoder(r.Body).Decode(&a.connected)
			a.job.Connected = true
			_, _ = w.Write([]byte(`{"id":"appimp-1","items":[],"states":{}}`))
		case strings.HasSuffix(r.URL.Path, "/stage"):
			_ = json.NewDecoder(r.Body).Decode(&a.staged)
			a.job.Items[1].State, a.job.Items[1].Retryable, a.job.Failed = "staged", false, 0
			_, _ = w.Write([]byte(`{"id":"appimp-1","items":[],"states":{}}`))
		case strings.HasSuffix(r.URL.Path, "/retire"):
			a.retireReq = map[string]any{}
			if r.Method == http.MethodPost {
				_ = json.NewDecoder(r.Body).Decode(&a.retireReq)
			}
			_ = json.NewEncoder(w).Encode(a.retire)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func resumeEnv(extra map[string]string) func(string) (string, bool) {
	m := map[string]string{"APP_API_TOKEN": "cp"}
	for k, v := range extra {
		m[k] = v
	}
	return importEnv(m)
}

func TestMigrateResumeShowsWhereTheJobStands(t *testing.T) {
	a := newResumeAPI(t)
	var out, errb bytes.Buffer
	code := run("cli", []string{"migrate", "coolify", "--resume", "appimp-1", "--api-url", a.srv.URL}, &out, &errb, resumeEnv(nil))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	for _, want := range []string{"appimp-1", "stage-failed", "image pull denied", "retry the failed items", "reconnect the source"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q: %s", want, out.String())
		}
	}
	if len(a.calls) != 1 || a.calls[0] != "GET /api/v1/migration/apps/sessions/appimp-1/progress" {
		t.Errorf("a plain resume must only read: %v", a.calls)
	}
}

func TestMigrateResumeRetryReconnectsAndRestagesOnlyTheFailedItem(t *testing.T) {
	a := newResumeAPI(t)
	var out, errb bytes.Buffer
	code := run("cli", []string{"migrate", "--resume", "appimp-1", "--retry", "--api-url", a.srv.URL}, &out, &errb, resumeEnv(map[string]string{envImportSourceToken: "fake-source-token"}))
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if a.connected.Token != "fake-source-token" || a.connected.URL != "https://c.example.com" || a.connected.Platform != "coolify" {
		t.Errorf("reconnect request: %+v", a.connected)
	}
	if len(a.staged.Items) != 1 || a.staged.Items[0] != "a2" {
		t.Errorf("only the failed item should be staged again: %+v", a.staged)
	}
	if strings.Contains(out.String()+errb.String(), "fake-source-token") {
		t.Error("the source token was printed")
	}
}

func TestMigrateResumeRetryWithoutACredentialExplainsWhy(t *testing.T) {
	a := newResumeAPI(t)
	var out, errb bytes.Buffer
	code := run("cli", []string{"migrate", "--resume", "appimp-1", "--retry", "--api-url", a.srv.URL}, &out, &errb, resumeEnv(nil))
	if code == exitOK {
		t.Fatalf("a retry that needs the source credential must not pretend to work")
	}
	if !strings.Contains(out.String()+errb.String(), envImportSourceToken) {
		t.Errorf("the error should say how to supply the credential: %s %s", out.String(), errb.String())
	}
}

func TestMigrateResumeRetireExitCodeFollowsTheVerdict(t *testing.T) {
	a := newResumeAPI(t)
	a.retire = migrationgate.Evaluate(migrationgate.Input{Items: []migrationgate.Item{{Name: "web", State: "verified", Domains: []string{"a.example.com"}}}})
	var out, errb bytes.Buffer
	code := run("cli", []string{"migrate", "--resume", "appimp-1", "--retire", "--api-url", a.srv.URL}, &out, &errb, resumeEnv(nil))
	if code != exitCheckFailed {
		t.Fatalf("an unproven job must exit %d, got %d", exitCheckFailed, code)
	}
	for _, want := range []string{"Not safe to delete yet", "FAIL", "routed", "Rollback guidance", "--probe"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q: %s", want, out.String())
		}
	}

	a.retire = migrationgate.Evaluate(migrationgate.Input{
		Items:        []migrationgate.Item{{Name: "web", State: "routed", Domains: []string{"a.example.com"}}},
		Probes:       []migrationgate.Probe{{Domain: "a.example.com", DNS: true, Responds: true}},
		Acknowledged: []string{migrationgate.GateBackup, migrationgate.GateTraffic}})
	out.Reset()
	code = run("cli", []string{"migrate", "--resume", "appimp-1", "--retire", "--probe", "--ack", "source-backup,traffic-cycle", "--api-url", a.srv.URL}, &out, &errb, resumeEnv(nil))
	if code != exitOK || !strings.Contains(out.String(), "Every gate passed") {
		t.Fatalf("a proven job should exit 0: %d %s", code, out.String())
	}
	if a.retireReq["probe"] != true {
		t.Errorf("--probe was not sent: %v", a.retireReq)
	}
	if acks, _ := a.retireReq["acknowledged"].([]any); len(acks) != 2 {
		t.Errorf("acknowledgments were not sent: %v", a.retireReq)
	}
}

func TestMigrateResumeNeedsAJobID(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run("cli", []string{"migrate", "--resume", "", "--api-url", "http://127.0.0.1:1"}, &out, &errb, resumeEnv(nil)); code != exitUsage {
		t.Fatalf("exit %d", code)
	}
	if code := run("cli", []string{"migrate", "--resume", "x", "--probe", "--api-url", "http://127.0.0.1:1"}, &out, &errb, resumeEnv(nil)); code != exitUsage {
		t.Fatalf("--probe without --retire should be a usage error, got %d", code)
	}
}
