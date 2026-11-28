package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

var reservedWriterNames = []string{
	"vault", "email-settings", "mail-system-email", "ai-assistant", "cloudflare-tunnel", "cloudflare-dns",
	"google-cloud-dns", "route53-dns", "builtin-registry", "dns-hetzner", "dns-digitalocean", "ai-assistant-provider-x",
	"heartbeat", "webpush", "ddns",
}

func assertNoApp(t *testing.T, db *store.DB, name string) {
	t.Helper()
	if _, err := db.GetDesiredService(context.Background(), name); !errors.Is(err, store.ErrServiceNotFound) {
		t.Errorf("app %q exists after a refused write (err %v)", name, err)
	}
}

// The reported exploit: clone an app that declares secret_env to the Vault namespace name.
func TestCloneToVaultNamespaceRefused(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 80, SecretEnv: []store.SecretEnvRef{{Name: "credential"}}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range reservedWriterNames {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"`+name+`"}`))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("clone to %q: status %d body %s, want 400", name, rec.Code, rec.Body.String())
		}
		assertNoApp(t, db, name)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"vault"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("escaped body form: status %d, want 400", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/clone", `{"new_name":"vault%2F"}`))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("slash form: status %d, want 400", rec.Code)
	}
	assertNoApp(t, db, "vault")
}

// An environment clone derives <app>-<slug>, so email into "settings" would become email-settings.
func TestEnvironmentCloneDerivedReservedNameRefused(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedCloneFixture(t, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "email", Image: "img:1", Port: 80, SecretEnv: []store.SecretEnvRef{{Name: "smtp_password"}}}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateServiceProject(ctx, "email", "proj_1"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "email", "env_src"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"settings"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("derived email-settings: status %d body %s, want 400", rec.Code, rec.Body.String())
	}
	assertNoApp(t, db, "email-settings")

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/environments/env_src/clone", `{"new_environment_name":"x","apps":[{"source_app":"web","new_name":"vault"}]}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("override vault: status %d body %s, want 400", rec.Code, rec.Body.String())
	}
	assertNoApp(t, db, "vault")
}

// Every writer, every reserved name.
func TestReservedNamesRefusedByEveryWriter(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, name := range reservedWriterNames {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps", `{"name":"`+name+`","image":"img:1","port":80}`))
		if rec.Code == http.StatusCreated || rec.Code == http.StatusAccepted || rec.Code == http.StatusOK {
			t.Errorf("create %q: status %d", name, rec.Code)
		}
		if err := rt.apps.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 80}); !errors.Is(err, store.ErrInvalidServiceName) {
			t.Errorf("store save %q: %v, want ErrInvalidServiceName", name, err)
		}
		if err := db.SaveSuspendedDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 80}); !errors.Is(err, store.ErrInvalidServiceName) {
			t.Errorf("suspended save %q: %v, want ErrInvalidServiceName", name, err)
		}
		if err := rt.appNameFree(ctx, name); err == nil {
			t.Errorf("appNameFree(%q) = nil", name)
		}
		assertNoApp(t, db, name)
	}
}

// An app that predates the guard still saves, and the doctor check lists it.
func TestExistingReservedNameStillSavesAndIsListed(t *testing.T) {
	t.Parallel()
	_, db := newTestRouter(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO desired_services (name, image, port) VALUES ('vault', 'img:1', 80)`); err != nil {
		t.Skipf("raw seed not possible: %v", err)
	}
	bad, err := db.ServiceNamesBreakingNamespaceRule(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 1 || bad[0] != "vault" {
		t.Fatalf("doctor offenders = %v, want [vault]", bad)
	}
}
