package api

import (
	"archive/tar"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const exportCanary = "CANARY-api-9f2c41"

func TestExportApp_CanaryAbsentAndAudited(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "acme/web:1", Port: 3000,
		Env:       map[string]string{"MODE": "prod", "API_TOKEN": exportCanary, "DB": "postgres://u:" + exportCanary + "@h/db"},
		SecretEnv: []store.SecretEnvRef{{Name: "SESSION_SECRET"}},
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/export?format=compose", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	tr := tar.NewReader(rec.Body)
	n := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(tr)
		if strings.Contains(string(body), exportCanary) {
			t.Fatalf("%s leaks the canary", h.Name)
		}
		n++
	}
	if n != 3 {
		t.Fatalf("files = %d, want 3", n)
	}

	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: AuditActionAppExport})
	if err != nil || len(entries) != 1 {
		t.Fatalf("app.export audit entries = %d, err = %v", len(entries), err)
	}
	if strings.Contains(entries[0].Detail+entries[0].Path, exportCanary) {
		t.Fatal("audit entry carries a value")
	}
}

func TestExportApp_AuthzAndErrors(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/export", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/missing/export", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", rec.Code)
	}
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "a:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/export?format=zip", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad format status = %d, want 400", rec.Code)
	}
}
