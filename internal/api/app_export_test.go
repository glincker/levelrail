package api

import (
	"archive/tar"
	"context"
	"errors"
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

type failingEnvSettings struct{ appEnvSettingsStore }

func (failingEnvSettings) GetAppEnvSettings(context.Context, string) (store.AppEnvSettings, error) {
	return store.AppEnvSettings{}, errors.New("boom")
}

func exportEnvOf(rt *Router, svc store.DesiredService) (map[string]string, []string) {
	out := rt.exportInput(context.Background(), "web", []store.DesiredService{svc})
	return out[0].Env, out[0].SecretEnv
}

func TestExportInput_ClassifiesEnvLikeOtherSurfaces(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "a:1"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAppEnvSettings(ctx, store.AppEnvSettings{AppName: "web", SensitiveOverrides: map[string]bool{"CUSTOM_BLOB": true}}); err != nil {
		t.Fatal(err)
	}
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.abcdefghij" + exportCanary
	svc := store.DesiredService{Name: "web", Image: "a:1", Env: map[string]string{
		"MODE": "prod", "DB_PASS": exportCanary, "REDIS_URL": "redis://:" + exportCanary + "@r:6379",
		"STRIPE": "sk_live_" + exportCanary, "SESSION": jwt, "CUSTOM_BLOB": exportCanary,
	}}
	env, secrets := exportEnvOf(rt, svc)
	if len(env) != 1 || env["MODE"] != "prod" {
		t.Fatalf("plain env = %v, want only MODE", env)
	}
	if len(secrets) != 5 {
		t.Fatalf("secrets = %v, want 5", secrets)
	}
	for k, v := range env {
		if strings.Contains(k+v, exportCanary) {
			t.Fatal("canary in plain env")
		}
	}
}

func TestExportInput_SettingsFailureMasksEverything(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	rt.envProtect.settings = failingEnvSettings{}
	env, secrets := exportEnvOf(rt, store.DesiredService{Name: "web", Image: "a:1", Env: map[string]string{"MODE": "prod", "OTHER": exportCanary}})
	if len(env) != 0 || len(secrets) != 2 {
		t.Fatalf("env = %v secrets = %v, want everything masked", env, secrets)
	}
}

func TestExportApp_GroupDeniedMember(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: "app_shop", Name: "shop"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"shop-web", "shop-api"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, AppID: "app_shop", Image: "a:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/shop-web/export", nil)
	bearer(tok)(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("before deny status = %d: %s", rec.Code, rec.Body.String())
	}
	attachTestPolicy(t, db, "deny-api", "Deny", "*", "app:shop-api", store.PrincipalTypeToken, "scoped-tok")
	req = httptest.NewRequest(http.MethodGet, "/api/v1/apps/shop-web/export", nil)
	bearer(tok)(req)
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("denied member status = %d, want 404", rec.Code)
	}
}
