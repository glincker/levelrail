package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

func putLegacyVault(t *testing.T, f *teamFixture, cookie *http.Cookie, body string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/"+appA+"/vault-env/X", body))
	return rec.Code
}

func TestVaultLegacyPrefix_DeveloperIsVetted(t *testing.T) {
	f := newTeamFixture(t)
	f.rt.runtimeSettings = runtimesettings.New(f.db, func(k string) (string, bool) {
		return "secret/data/team-a", k == "APP_VAULT_LEGACY_ALLOWED_PREFIXES"
	})
	dev := f.cookies[store.TeamRoleDeveloper]

	if code := putLegacyVault(t, f, dev, `{"path":"secret/data/team-b/db","key":"password"}`); code != http.StatusForbidden {
		t.Fatalf("path under another team's prefix = %d, want 403", code)
	}
	if code := putLegacyVault(t, f, dev, `{"path":"secret/data/team-a/../team-b","key":"password"}`); code != http.StatusForbidden {
		t.Fatalf("traversal = %d, want 403", code)
	}
	if code := putLegacyVault(t, f, dev, `{"path":"secret/data/team-a/db","key":"password"}`); code != http.StatusOK {
		t.Fatalf("path inside the allowlist = %d, want 200", code)
	}
}

func TestVaultLegacyPrefix_StoredPathKeepsWorking(t *testing.T) {
	f := newTeamFixture(t)
	if err := f.db.SetServiceVaultEnvVar(context.Background(), appA, "X", &store.VaultEnvRef{Path: "old/path", Key: "k"}); err != nil {
		t.Fatal(err)
	}
	dev := f.cookies[store.TeamRoleDeveloper]
	if code := putLegacyVault(t, f, dev, `{"path":"old/path","key":"k"}`); code != http.StatusOK {
		t.Fatalf("unchanged stored path = %d, want 200", code)
	}
	if code := putLegacyVault(t, f, dev, `{"path":"old/other","key":"k"}`); code != http.StatusForbidden {
		t.Fatalf("changed path = %d, want 403", code)
	}
}

func TestVaultLegacyPrefix_SpecRefIsUnvetted(t *testing.T) {
	f := newTeamFixture(t)
	if err := f.db.SetServiceVaultEnvVar(context.Background(), appA, "X", &store.VaultEnvRef{Path: "p", Key: "k", Unvetted: true}); err != nil {
		t.Fatal(err)
	}
	dev := f.cookies[store.TeamRoleDeveloper]
	if code := putLegacyVault(t, f, dev, `{"path":"p","key":"k"}`); code != http.StatusForbidden {
		t.Fatalf("an unvetted stored path must not be laundered by a developer, got %d", code)
	}
}
