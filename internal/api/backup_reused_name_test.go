package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBackupRoutes_ReusedDatabaseNameSeesNoOldBackups(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	downloader := &fakeBackupDownloader{content: "dump-bytes"}
	rt, db := newTestRouterWithBackupDownloader(t, downloader)
	cookie := loginTestSession(t, rt, db)
	seedBackupTargetForAPI(t, db)
	mk := func() {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "orders", Engine: store.EnginePostgres, Version: "16"}); err != nil {
			t.Fatal(err)
		}
	}
	mk()
	seedSucceededBackupForAPI(t, db, "orders")
	get := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, ""))
		return rec
	}
	if rec := get(http.MethodGet, "/api/v1/databases/orders/backups"); !strings.Contains(rec.Body.String(), "bkh_1") {
		t.Fatalf("owner list = %s", rec.Body.String())
	}
	if err := db.DeleteDesiredDatabase(ctx, "orders"); err != nil {
		t.Fatal(err)
	}
	mk()
	if rec := get(http.MethodGet, "/api/v1/databases/orders/backups"); strings.Contains(rec.Body.String(), "bkh_1") {
		t.Fatalf("newcomer list leaked old row: %s", rec.Body.String())
	}
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/databases/orders/backups/bkh_1/download"},
		{http.MethodPut, "/api/v1/databases/orders/backups/bkh_1/pin"},
		{http.MethodGet, "/api/v1/databases/orders/backups/bkh_1/verifications"},
		{http.MethodGet, "/api/v1/databases/orders/restore-preview?backup_id=bkh_1"},
	} {
		if rec := get(c.method, c.path); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404 (%s)", c.method, c.path, rec.Code, rec.Body.String())
		}
	}
	if downloader.callCount != 0 {
		t.Errorf("downloader called %d times for a foreign backup", downloader.callCount)
	}
}
