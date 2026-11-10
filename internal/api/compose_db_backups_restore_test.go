package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeComposeDBRestoreRunner struct{ calls chan [4]string }

func (f *fakeComposeDBRestoreRunner) RunComposeServiceRestore(_ context.Context, safetyID, app, service, engine string, run store.ComposeServiceBackupRun) error {
	f.calls <- [4]string{app, service, engine, run.ID + "/" + safetyID}
	return nil
}

func newComposeRestoreRouter(t *testing.T) (*Router, *store.DB, *fakeComposeDBRestoreRunner) {
	t.Helper()
	runner := &fakeComposeDBRestoreRunner{calls: make(chan [4]string, 2)}
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithComposeDBRestoreRunner(runner))
	seedComposeApp(t, db)
	seedBackupTargetForAPI(t, db)
	ctx := context.Background()
	for _, run := range []struct{ id, status string }{{"csr_ok", store.BackupStatusSucceeded}, {"csr_bad", store.BackupStatusFailed}} {
		if err := db.StartComposeServiceBackupRun(ctx, store.ComposeServiceBackupRun{ID: run.id, App: "shop", Service: "db", TargetID: "bkt_test1", ObjectKey: "compose/shop/db/" + run.id, StartedAt: "2026-11-08T00:00:00Z"}); err != nil {
			t.Fatalf("start run: %v", err)
		}
		if err := db.FinishComposeServiceBackupRun(ctx, run.id, run.status, 12, "sum", "", "2026-11-08T00:01:00Z"); err != nil {
			t.Fatalf("finish run: %v", err)
		}
	}
	return rt, db, runner
}

func TestComposeDBRestore_NeedsRoot(t *testing.T) {
	t.Parallel()
	rt, db, runner := newComposeRestoreRouter(t)
	const path = "/api/v1/apps/shop/compose-db-services/db/backups/csr_ok/restore"
	for name, abilities := range map[string][]string{
		"viewer":    {AbilityRead},
		"developer": {AbilityRead, AbilityWrite, AbilityDeploy},
	} {
		tok := seedMatrixToken(t, db, name, abilities)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"confirm":"restore shop/db"}`))
		bearer(tok)(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s status = %d, want 403", name, rec.Code)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatal("runner called for an unauthorized caller")
	}
}

func TestComposeDBRestore_PreviewConfirmAndAudit(t *testing.T) {
	t.Parallel()
	rt, db, runner := newComposeRestoreRouter(t)
	cookie := loginTestSession(t, rt, db)
	do := func(path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/shop/compose-db-services/db/backups/"+path+"/restore", body))
		return rec
	}

	rec := do("csr_ok", `{"preview":true}`)
	var pv composeDBRestoreResource
	if err := json.NewDecoder(rec.Body).Decode(&pv); err != nil || rec.Code != http.StatusOK || pv.ConfirmPhrase != "restore shop/db" || pv.Engine != store.EnginePostgres || pv.ObjectKey == "" || !pv.Preview {
		t.Fatalf("preview = %d %+v %v", rec.Code, pv, err)
	}
	if rec = do("csr_ok", `{"confirm":"restore shop/web"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirm = %d, want 400", rec.Code)
	}
	if rec = do("csr_ok", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing confirm = %d, want 400", rec.Code)
	}
	if rec = do("csr_bad", `{"confirm":"restore shop/db"}`); rec.Code != http.StatusConflict {
		t.Fatalf("failed run = %d, want 409", rec.Code)
	}
	if rec = do("csr_nope", `{"confirm":"restore shop/db"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown run = %d, want 404", rec.Code)
	}
	if rec = do("csr_ok", `{"confirm":"restore shop/db","as_new":"copy"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("as_new = %d, want 400", rec.Code)
	}
	if len(runner.calls) != 0 {
		t.Fatal("runner called before a valid confirmed request")
	}

	if rec = do("csr_ok", `{"confirm":"restore shop/db"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("restore = %d: %s", rec.Code, rec.Body.String())
	}
	select {
	case c := <-runner.calls:
		if c[0] != "shop" || c[1] != "db" || c[2] != store.EnginePostgres || !strings.HasPrefix(c[3], "csr_ok/csr_") {
			t.Errorf("runner call = %v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner not called")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		rows, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
		if err != nil {
			t.Fatalf("list audit: %v", err)
		}
		for _, e := range rows {
			if e.Action == AuditActionComposeDBRestore && strings.Contains(e.Detail, `"outcome":"succeeded"`) && !strings.Contains(e.Detail, "sum") {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("no restore audit row recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAuditComposeDBRestore_NilAuditLogReturnsEarly(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.auditLog = nil
	rt.auditComposeDBRestore("/api/v1/apps/x/db/restore", "user", "u1", "tester", map[string]string{"k": "v"}, http.StatusOK)
}
