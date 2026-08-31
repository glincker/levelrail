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

// fakeBackupVerifier is a hand-written fake for BackupVerifier, the same
// pattern fakeBackupRunner (backups_test.go) already establishes for
// BackupRunner: calls is buffered so handleVerifyBackup's detached
// goroutine can send without blocking even if a test never reads it.
type fakeBackupVerifier struct {
	err   error
	calls chan backupVerifyCall
}

type backupVerifyCall struct {
	verificationID, backupHistoryID, engine, checkedBy string
}

func newFakeBackupVerifier() *fakeBackupVerifier {
	return &fakeBackupVerifier{calls: make(chan backupVerifyCall, 4)}
}

func (f *fakeBackupVerifier) VerifyBackup(_ context.Context, verificationID, backupHistoryID, engine, checkedBy string) error {
	f.calls <- backupVerifyCall{verificationID, backupHistoryID, engine, checkedBy}
	return f.err
}

func (f *fakeBackupVerifier) awaitCall(t *testing.T) backupVerifyCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(2 * time.Second):
		t.Fatal("VerifyBackup was not called within the deadline")
		return backupVerifyCall{}
	}
}

func newTestRouterWithBackupVerifier(t *testing.T, verifier BackupVerifier) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithBackupVerifier(verifier)), db
}

// seedVerifyFixture seeds a database, a backup target, and one succeeded
// backup_history row (id "bkh_1", database "main"), the fixture every
// verify-route test needs before it can exercise the route itself.
// Builds on seedSucceededBackupForAPI (backup_download_test.go), the
// identical fixture the download-route tests already establish.
func seedVerifyFixture(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	seedBackupTargetForAPI(t, db)
	seedSucceededBackupForAPI(t, db, "main")
}

func TestBackupVerifyRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/databases/main/backups/bkh_1/verify"},
		{http.MethodGet, "/api/v1/databases/main/backups/bkh_1/verifications"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestHandleVerifyBackup_NoVerifierConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithBackupVerifier
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleVerifyBackup_DatabaseNotFound(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/missing/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleVerifyBackup_BackupNotFound(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups/bkh_missing/verify", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleVerifyBackup_BackupBelongsToDifferentDatabase(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "other", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed second database: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/other/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleVerifyBackup_NotSucceeded(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatalf("seed database: %v", err)
	}
	target := seedBackupTargetForAPI(t, db)
	if err := db.StartBackupHistory(ctx, store.BackupHistory{
		ID: "bkh_1", DatabaseName: "main", TargetID: target.ID,
		ObjectKey: "main/main-1.dump", StartedAt: "2026-08-14T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed backup history: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandleVerifyBackup_Success(t *testing.T) {
	verifier := newFakeBackupVerifier()
	rt, db := newTestRouterWithBackupVerifier(t, verifier)
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/backups/bkh_1/verify", ""))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var got backupVerificationResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(got.ID, "bkv_") {
		t.Errorf("ID = %q, want a bkv_-prefixed id", got.ID)
	}
	if got.Status != store.BackupVerificationStatusRunning {
		t.Errorf("Status = %q, want %q", got.Status, store.BackupVerificationStatusRunning)
	}
	if got.BackupHistoryID != "bkh_1" {
		t.Errorf("BackupHistoryID = %q, want %q", got.BackupHistoryID, "bkh_1")
	}

	call := verifier.awaitCall(t)
	if call.verificationID != got.ID {
		t.Errorf("VerifyBackup verificationID = %q, want it to match the response id %q", call.verificationID, got.ID)
	}
	if call.backupHistoryID != "bkh_1" {
		t.Errorf("VerifyBackup backupHistoryID = %q, want %q", call.backupHistoryID, "bkh_1")
	}
	if call.engine != store.EnginePostgres {
		t.Errorf("VerifyBackup engine = %q, want %q", call.engine, store.EnginePostgres)
	}
}

func TestHandleListBackupVerifications_BackupNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_missing/verifications", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListBackupVerifications_NoVerifierConfigured_StillWorks(t *testing.T) {
	// Listing verification attempts needs no runner: see
	// WithBackupVerifier's own doc comment. This test locks that in.
	rt, db := newTestRouter(t) // no WithBackupVerifier
	cookie := loginTestSession(t, rt, db)
	seedVerifyFixture(t, db)
	if err := db.StartBackupVerification(context.Background(), store.BackupVerification{
		ID: "bkv_1", BackupHistoryID: "bkh_1", CheckedBy: "alice", StartedAt: "2026-08-15T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed verification: %v", err)
	}
	if err := db.FinishBackupVerification(context.Background(), "bkv_1", store.BackupVerificationStatusPassed, true, true, true, 4096, "", "2026-08-15T00:01:00Z"); err != nil {
		t.Fatalf("finish verification: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/main/backups/bkh_1/verifications", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []backupVerificationResource
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(got) != 1 || got[0].ID != "bkv_1" || got[0].Status != store.BackupVerificationStatusPassed {
		t.Fatalf("verifications = %+v, want exactly the seeded, passed row", got)
	}
	if got[0].CheckedBy != "alice" {
		t.Errorf("CheckedBy = %q, want %q", got[0].CheckedBy, "alice")
	}
}
