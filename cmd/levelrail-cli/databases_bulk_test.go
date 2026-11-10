package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func newDatabasesBulkServer(t *testing.T, got *apiclient.BulkDatabasesRequest) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/databases/bulk" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(got)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`{"action":"start","dry_run":false,"results":[{"name":"a","status":"ok"},{"name":"b","status":"skipped","reason":"running","message":"already running"}],"counts":{"ok":1,"skipped":1}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_DatabasesBulk_StartNames(t *testing.T) {
	var got apiclient.BulkDatabasesRequest
	srv := newDatabasesBulkServer(t, &got)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "bulk", "start", "a", "b", "--api-url", srv.URL})
	if got.Action != "start" || strings.Join(got.Names, ",") != "a,b" {
		t.Errorf("request = %+v", got)
	}
	if !strings.Contains(stdout, "already running") {
		t.Errorf("stdout = %q, want per-database results", stdout)
	}
}

func TestRun_DatabasesBulk_SelectorDryRunJSON(t *testing.T) {
	var got apiclient.BulkDatabasesRequest
	srv := newDatabasesBulkServer(t, &got)
	stdout, _ := runCLIExpectOK(t, []string{"databases", "bulk", "backup", "--selector", "engine=postgres,project=proj_1", "--dry-run", "--output", "json", "--api-url", srv.URL})
	if !got.DryRun || got.Engine != "postgres" || got.ProjectID != "proj_1" || len(got.Names) != 0 {
		t.Errorf("request = %+v", got)
	}
	if !strings.Contains(stdout, `"results"`) {
		t.Errorf("stdout = %q, want JSON", stdout)
	}
}

func TestRun_DatabasesBulk_StopConfirmCount(t *testing.T) {
	var got apiclient.BulkDatabasesRequest
	srv := newDatabasesBulkServer(t, &got)
	var stdout, stderr strings.Builder
	if code := run("levelrail-cli-test", []string{"databases", "bulk", "stop", "a", "b", "--api-url", srv.URL}, &stdout, &stderr, envMap()); code != exitUsage {
		t.Fatalf("stop without --confirm-count exit = %d, want %d", code, exitUsage)
	}
	runCLIExpectOK(t, []string{"databases", "bulk", "stop", "a", "b", "--confirm-count", "2", "--api-url", srv.URL})
	if got.ConfirmCount != 2 {
		t.Errorf("confirm_count = %d, want 2", got.ConfirmCount)
	}
}

func TestRun_DatabasesBulk_UsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"databases", "bulk"},
		{"databases", "bulk", "restart", "a"},
		{"databases", "bulk", "start"},
		{"databases", "bulk", "start", "--selector", "colour=red"},
		{"databases", "bulk", "start", "a", "--selector", "engine=redis"},
	} {
		var stdout, stderr strings.Builder
		if code := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); code != exitUsage {
			t.Errorf("%v: exit = %d, want %d", args, code, exitUsage)
		}
	}
}
