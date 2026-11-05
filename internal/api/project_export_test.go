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

const projectExportCanary = "CANARY-proj-7d31ab"

func seedExportProject(t *testing.T, db *store.DB, writeOnly bool) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveProject(ctx, store.Project{ID: "proj_x", Name: "shop", CreatedAt: "2026-08-16T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "acme/web:1", Port: 3000,
		Env: map[string]string{"MODE": "prod", "API_TOKEN": projectExportCanary},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_x"); err != nil {
		t.Fatal(err)
	}
	if writeOnly {
		if err := db.SaveAppEnvSettings(ctx, store.AppEnvSettings{AppName: "web", SecretsWriteOnly: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SetProjectEnvVars(ctx, "proj_x", map[string]string{"DB_PASSWORD": projectExportCanary + "-shared", "REGION": "eu"}); err != nil {
		t.Fatal(err)
	}
}

func readExportTar(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	tr := tar.NewReader(rec.Body)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}

func TestProjectExport_SharedEnvClassifiedWithoutSecrets(t *testing.T) {
	t.Setenv(envSecretReveal, "allow")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedExportProject(t, db, false)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_x/export", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	files := readExportTar(t, rec)
	for name, body := range files {
		if strings.Contains(body, projectExportCanary) {
			t.Fatalf("%s leaks the canary", name)
		}
	}
	if !strings.Contains(files[".env.example"], "CHANGE_ME") {
		t.Fatalf(".env.example has no placeholder: %s", files[".env.example"])
	}
	if _, ok := files[".env"]; ok {
		t.Fatal(".env written without include_secrets")
	}
}

func TestProjectExport_WriteOnlyAppRefusesSecrets(t *testing.T) {
	t.Setenv(envSecretReveal, "allow")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedExportProject(t, db, true)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_x/export?include_secrets=true", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), projectExportCanary) || strings.Contains(rec.Header().Get("Content-Type"), "tar") {
		t.Fatal("refused export leaked content")
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: AuditActionProjectExport})
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries = %d, err = %v", len(entries), err)
	}
	if !strings.Contains(entries[0].Detail, `"include_secrets":true`) || entries[0].StatusCode != http.StatusForbidden {
		t.Fatalf("denied attempt not recorded: %+v", entries[0])
	}
}

func TestProjectExport_SecretsAuditedBeforeBody(t *testing.T) {
	t.Setenv(envSecretReveal, "allow")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedExportProject(t, db, false)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/projects/proj_x/export?include_secrets=true", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	files := readExportTar(t, rec)
	if !strings.Contains(files[".env"], projectExportCanary) {
		t.Fatalf(".env lacks the opted-in value: %s", files[".env"])
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: AuditActionProjectExport})
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries = %d, err = %v", len(entries), err)
	}
	if strings.Contains(entries[0].Detail+entries[0].Path, projectExportCanary) || !strings.Contains(entries[0].Detail, `"include_secrets":true`) {
		t.Fatalf("audit detail wrong: %+v", entries[0])
	}
}
