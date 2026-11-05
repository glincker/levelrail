package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func changesServer(t *testing.T, rolledBack *atomic.Bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/domains/far.example.com/changes":
			_, _ = w.Write([]byte(`[{"run_id":"gl_1","kind":"dns_apply","result":"applied","created_at":"2026-11-03T10:00:00Z","summary":[{"action":"created","description":"A record @ -> 203.0.113.7"}],"rollback_until":"2026-11-03T11:00:00Z","can_rollback":true,"already_undone":false}]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/domains/far.example.com/changes/gl_1/rollback":
			rolledBack.Store(true)
			_, _ = w.Write([]byte(`{"run_id":"gl_1","domain":"far.example.com","undone_at":"2026-11-03T10:30:00Z","outcomes":[{"status":"reverted","label":"removed the A record for far.example.com"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestDomainsChangesLists(t *testing.T) {
	var rb atomic.Bool
	srv := changesServer(t, &rb)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "changes", "far.example.com", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d: %s", got, stderr.String())
	}
	if !strings.Contains(stdout.String(), "gl_1") || !strings.Contains(stdout.String(), "rollback until") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDomainsRollbackNeedsYes(t *testing.T) {
	var rb atomic.Bool
	srv := changesServer(t, &rb)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	if got := run("levelrail-cli-test", []string{"domains", "rollback", "far.example.com", "gl_1", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d: %s", got, stderr.String())
	}
	if rb.Load() || !strings.Contains(stdout.String(), "pass --yes") {
		t.Fatalf("rolled back = %v stdout = %q", rb.Load(), stdout.String())
	}
	stdout.Reset()
	if got := run("levelrail-cli-test", []string{"domains", "rollback", "far.example.com", "gl_1", "--yes", "--api-url", srv.URL}, &stdout, &stderr, envMap()); got != exitOK {
		t.Fatalf("exit = %d: %s", got, stderr.String())
	}
	if !rb.Load() || !strings.Contains(stdout.String(), "reverted:") {
		t.Fatalf("rolled back = %v stdout = %q", rb.Load(), stdout.String())
	}
}
