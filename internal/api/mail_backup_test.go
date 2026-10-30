package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailbackup"
)

func mailBackupCall(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestMailBackupsNotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "")
	if rec := mailBackupCall(t, rt, cookie, http.MethodGet, "/api/v1/mail/backups", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d %s", rec.Code, rec.Body)
	}
}

func TestMailBackupsListAndStorage(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	saveMailApp(t, db, "")
	dir := t.TempDir()
	WithMailBackup(dir, nil, nil, nil)(rt)

	folder := filepath.Join(dir, "mail-backups", "mail")
	m := mailbackup.Manifest{ID: "20261030T120000Z", MoxVersion: "v0.0.17", CreatedAt: time.Now().UTC(), Files: 3, Bytes: 10, ArchiveName: "mail-20261030T120000Z.tar.zst.age", Verified: true}
	raw, _ := json.Marshal(m)
	if err := os.MkdirAll(folder, 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(folder, mailbackup.ManifestName(m.ID)), raw, 0o600)
	_ = os.WriteFile(filepath.Join(folder, m.ArchiveName), []byte("sealed"), 0o600)

	rec := mailBackupCall(t, rt, cookie, http.MethodGet, "/api/v1/mail/backups", "")
	var list mailBackupListResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if rec.Code != http.StatusOK || len(list.Backups) != 1 || !list.Backups[0].Verified || list.Backups[0].StoredBytes != 6 || list.UsedBytes == 0 || list.Encrypted {
		t.Fatalf("unexpected %d %s", rec.Code, rec.Body)
	}

	if rec := mailBackupCall(t, rt, cookie, http.MethodPost, "/api/v1/mail/backups/..%2Fx/restore", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: want 400, got %d", rec.Code)
	}
	if rec := mailBackupCall(t, rt, cookie, http.MethodPut, "/api/v1/mail/storage", `{"kind":"ftp"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad kind: want 400, got %d", rec.Code)
	}
	if rec := mailBackupCall(t, rt, cookie, http.MethodPut, "/api/v1/mail/storage", `{"kind":"s3","bucket":"b","access_key_id":"k","secret_access_key":"not-a-real-value"}`); rec.Code != http.StatusNotImplemented {
		t.Fatalf("no secret store: want 501, got %d", rec.Code)
	}
	rec = mailBackupCall(t, rt, cookie, http.MethodGet, "/api/v1/mail/storage", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"kind":"local"`) || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("storage: %d %s", rec.Code, rec.Body)
	}
}
