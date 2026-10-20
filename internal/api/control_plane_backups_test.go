package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newTestRouterWithCPBackups(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	m := cpbackup.NewManager(db, t.TempDir())
	return NewRouter(discardLogger(), testBrand(), db, WithControlPlaneBackups(m)), db
}

func TestControlPlaneBackups_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/backups", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
}

func TestControlPlaneBackups_Lifecycle(t *testing.T) {
	rt, db := newTestRouterWithCPBackups(t)
	cookie := loginTestSession(t, rt, db)
	do := func(method, target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, ""))
		return rec
	}

	rec := do(http.MethodPost, "/api/v1/system/backups")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created cpbackup.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !cpbackup.ValidName(created.Name) || created.SizeBytes == 0 || len(created.SHA256) != 64 {
		t.Fatalf("unexpected created info: %+v", created)
	}

	rec = do(http.MethodGet, "/api/v1/system/backups")
	var list []cpbackup.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].SHA256 != created.SHA256 {
		t.Fatalf("list = %s, err = %v", rec.Body.String(), err)
	}

	rec = do(http.MethodGet, "/api/v1/system/backups/"+created.Name+"/download")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "application/octet-stream" ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") ||
		!strings.HasPrefix(rec.Body.String(), "SQLite format 3") {
		t.Fatalf("download status = %d headers = %v", rec.Code, rec.Header())
	}

	rec = do(http.MethodPost, "/api/v1/system/backups/"+created.Name+"/verify")
	var ver cpbackup.VerifyResult
	if err := json.Unmarshal(rec.Body.Bytes(), &ver); rec.Code != http.StatusOK || err != nil || !ver.OK || ver.Name != created.Name || len(ver.Checks) != 3 {
		t.Fatalf("verify status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec = do(http.MethodPost, "/api/v1/system/backups/levelrail-20200101T000000Z.db/verify"); rec.Code != http.StatusNotFound {
		t.Fatalf("verify missing status = %d, want 404", rec.Code)
	}

	if rec = do(http.MethodDelete, "/api/v1/system/backups/"+created.Name); rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if rec = do(http.MethodDelete, "/api/v1/system/backups/"+created.Name); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", rec.Code)
	}
}

func TestControlPlaneBackups_BadNames(t *testing.T) {
	rt, db := newTestRouterWithCPBackups(t)
	cookie := loginTestSession(t, rt, db)
	for _, name := range []string{"..%2Fetc%2Fpasswd", "levelrail.db", "levelrail-20200101T000000Z.db.tmp", "levelrail-20200101T000000Z.db"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/backups/"+name+"/download", ""))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", name, rec.Code)
		}
	}
}

func TestControlPlaneBackups_RequireRoot(t *testing.T) {
	rt, db := newTestRouterWithCPBackups(t)
	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	for _, c := range []routeCase{
		{http.MethodPost, "/api/v1/system/backups"},
		{http.MethodGet, "/api/v1/system/backups"},
		{http.MethodGet, "/api/v1/system/backups/levelrail-20200101T000000Z.db/download"},
		{http.MethodPost, "/api/v1/system/backups/levelrail-20200101T000000Z.db/verify"},
		{http.MethodDelete, "/api/v1/system/backups/levelrail-20200101T000000Z.db"},
	} {
		req := httptest.NewRequest(c.method, c.target, nil)
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: status = %d, want 403", c.method, c.target, rec.Code)
		}
	}
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodGet, "/api/v1/system/backups"}})
}
