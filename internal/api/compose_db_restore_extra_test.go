package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// scriptedRestoreRunner blocks until release is closed, then returns err.
type scriptedRestoreRunner struct {
	started chan struct{}
	release chan struct{}
	err     error
}

func (s *scriptedRestoreRunner) RunComposeServiceRestore(context.Context, string, string, string, string, store.ComposeServiceBackupRun) error {
	s.started <- struct{}{}
	<-s.release
	return s.err
}

func postComposeRestore(rt *Router, cookie *http.Cookie, t *testing.T, service, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/shop/compose-db-services/"+service+"/backups/csr_ok/restore", body))
	return rec
}

// waitRestoreIdle keeps the package-level single-flight key from leaking into other tests.
func waitRestoreIdle(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, busy := composeRestoreInFlight.Load("shop/db"); !busy {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("restore still marked in flight")
}

func TestComposeDBRestoreExtra_SecondRestoreConflicts(t *testing.T) {
	rt, db, _ := newComposeRestoreRouter(t)
	runner := &scriptedRestoreRunner{started: make(chan struct{}, 1), release: make(chan struct{})}
	rt.composeDBRestoreRunner = runner
	cookie := loginTestSession(t, rt, db)
	t.Cleanup(func() { waitRestoreIdle(t) })

	if rec := postComposeRestore(rt, cookie, t, "db", `{"confirm":"restore shop/db"}`); rec.Code != http.StatusAccepted {
		close(runner.release)
		t.Fatalf("first restore = %d: %s", rec.Code, rec.Body.String())
	}
	<-runner.started
	rec := postComposeRestore(rt, cookie, t, "db", `{"confirm":"restore shop/db"}`)
	close(runner.release)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second restore = %d, want 409", rec.Code)
	}
}

func TestComposeDBRestoreExtra_NonDatabaseServiceRejected(t *testing.T) {
	rt, db, runner := newComposeRestoreRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, body := range []string{`{"confirm":"restore shop/web"}`, `{"preview":true}`} {
		if rec := postComposeRestore(rt, cookie, t, "web", body); rec.Code != http.StatusNotFound {
			t.Errorf("web service %s = %d, want 404", body, rec.Code)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatal("runner called for a non-database service")
	}
}

func TestComposeDBRestoreExtra_RunnerFailureAuditedWithoutDetail(t *testing.T) {
	rt, db, _ := newComposeRestoreRouter(t)
	const leak = "pg_dump: SECRET-DUMP-CONTENT"
	runner := &scriptedRestoreRunner{started: make(chan struct{}, 1), release: make(chan struct{}), err: errors.New(leak)}
	close(runner.release)
	rt.composeDBRestoreRunner = runner
	cookie := loginTestSession(t, rt, db)
	t.Cleanup(func() { waitRestoreIdle(t) })

	rec := postComposeRestore(rt, cookie, t, "db", `{"confirm":"restore shop/db"}`)
	if rec.Code != http.StatusAccepted || strings.Contains(rec.Body.String(), leak) {
		t.Fatalf("restore = %d: %s", rec.Code, rec.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		rows, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
		if err != nil {
			t.Fatalf("list audit: %v", err)
		}
		for _, e := range rows {
			if e.Action != AuditActionComposeDBRestore {
				continue
			}
			if !strings.Contains(e.Detail, `"outcome":"failed"`) || strings.Contains(e.Detail, "SECRET") || e.StatusCode != http.StatusInternalServerError {
				t.Fatalf("audit = %d %s", e.StatusCode, e.Detail)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no failed-restore audit row recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
