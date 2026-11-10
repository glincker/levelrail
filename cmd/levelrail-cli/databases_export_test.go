package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// exportFake serves targets, trigger, history and download. status is the
// state the started backup reports on the first poll.
func exportFake(t *testing.T, status string, triggers *int32, cloneBody *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/backup-targets":
			_, _ = w.Write([]byte(`[{"id":"bkt_1","name":"main"}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/databases/mydb/backups":
			atomic.AddInt32(triggers, 1)
			_, _ = w.Write([]byte(`{"id":"bkh_new","status":"running"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/databases/mydb/backups":
			_, _ = w.Write([]byte(`[{"id":"bkh_new","status":"` + status + `","error":"disk full","started_at":"2026-11-08T10:00:00Z"},{"id":"bkh_old","status":"succeeded","started_at":"2026-11-01T10:00:00Z"}]`))
		case strings.HasSuffix(r.URL.Path, "/download"):
			_, _ = io.WriteString(w, "DUMP:"+strings.Split(r.URL.Path, "/")[6]) //nolint:gosec // test fake echoing a path segment
		case r.URL.Path == "/api/v1/databases/mydb/restore-as-new":
			b := new(bytes.Buffer)
			_, _ = b.ReadFrom(r.Body)
			*cloneBody = b.String()
			_, _ = w.Write([]byte(`{"id":"cr_1","status":"running"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fastPoll(t *testing.T) {
	t.Helper()
	old := exportPollInterval
	exportPollInterval = func(func(string) (string, bool)) time.Duration { return 5 * time.Millisecond }
	t.Cleanup(func() { exportPollInterval = old })
}

func runDBCLI(args ...string) (int, string, string) {
	var o, e bytes.Buffer
	code := run("levelrail-cli-test", args, &o, &e, envMap())
	return code, o.String(), e.String()
}

func TestDatabasesExport_FreshWritesFile0600(t *testing.T) {
	fastPoll(t)
	var triggers int32
	srv := exportFake(t, "succeeded", &triggers, new(string))
	out := filepath.Join(t.TempDir(), "d.dump")

	code, stdout, stderr := runDBCLI("databases", "export", "mydb", "--out", out, "--api-url", srv.URL)
	if code != exitOK || stdout != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	got, _ := os.ReadFile(out) //nolint:gosec // path is a test temp dir
	fi, _ := os.Stat(out)
	if string(got) != "DUMP:bkh_new" || fi.Mode().Perm() != 0o600 || triggers != 1 {
		t.Errorf("got=%q mode=%v triggers=%d", got, fi.Mode().Perm(), triggers)
	}

	code, _, stderr = runDBCLI("databases", "export", "mydb", "--out", out, "--api-url", srv.URL)
	if code == exitOK || !strings.Contains(stderr, "--force") {
		t.Errorf("existing file: code=%d stderr=%q", code, stderr)
	}
	if code, _, _ = runDBCLI("databases", "export", "mydb", "--out", out, "--force", "--api-url", srv.URL); code != exitOK {
		t.Errorf("--force code=%d", code)
	}
}

func TestDatabasesExport_LatestStdoutNoTrigger(t *testing.T) {
	var triggers int32
	srv := exportFake(t, "running", &triggers, new(string))
	code, stdout, _ := runDBCLI("databases", "export", "mydb", "--latest", "--out", "-", "--api-url", srv.URL)
	if code != exitOK || stdout != "DUMP:bkh_old" || triggers != 0 {
		t.Errorf("code=%d stdout=%q triggers=%d", code, stdout, triggers)
	}
}

func TestDatabasesExport_FailedAndTimeout(t *testing.T) {
	fastPoll(t)
	var n int32
	srv := exportFake(t, "failed", &n, new(string))
	code, stdout, stderr := runDBCLI("databases", "export", "mydb", "--out", "-", "--api-url", srv.URL)
	if code == exitOK || stdout != "" || !strings.Contains(stderr, "disk full") {
		t.Errorf("failed: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	srv2 := exportFake(t, "running", &n, new(string))
	code, stdout, stderr = runDBCLI("databases", "export", "mydb", "--out", "-", "--wait", "50ms", "--api-url", srv2.URL)
	if code == exitOK || stdout != "" || !strings.Contains(stderr, "timed out") {
		t.Errorf("timeout: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestDatabasesExport_RefusesTerminal(t *testing.T) {
	old := exportStdoutIsTerminal
	exportStdoutIsTerminal = func(io.Writer) bool { return true }
	t.Cleanup(func() { exportStdoutIsTerminal = old })
	var n int32
	srv := exportFake(t, "succeeded", &n, new(string))
	code, stdout, stderr := runDBCLI("databases", "export", "mydb", "--latest", "--api-url", srv.URL)
	if code == exitOK || stdout != "" || !strings.Contains(stderr, "terminal") || n != 0 {
		t.Errorf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}
