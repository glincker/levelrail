package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

func oauthResult(rec interface{ Header() http.Header }) string {
	return rec.Header().Get("Location")
}

func TestOAuthGoldenSignupPolicy(t *testing.T) {
	tests := []struct {
		name     string
		domain   string
		id       fakeIdentity
		existing string
		want     string
	}{
		{"open for any domain", "", fakeIdentity{Sub: "s1", Email: "a@anywhere.test", Verified: true}, "", "/oauth/complete"},
		{"allowed domain matches", "corp.test", fakeIdentity{Sub: "s2", Email: "a@CORP.test", Verified: true}, "", "/oauth/complete"},
		{"allowed domain refuses others", "corp.test", fakeIdentity{Sub: "s3", Email: "a@other.test", Verified: true}, "", "/login?oauth_error=domain_not_allowed"},
		{"email held by a password account", "", fakeIdentity{Sub: "s4", Email: "held@example.test", Verified: true}, "held@example.test", "/login?oauth_error=email_in_use"},
		{"unverified email is refused", "", fakeIdentity{Sub: "s5", Email: "u@example.test", Verified: false}, "", "/login?oauth_error=userinfo_failed"},
	}
	for _, tc := range tests {
		for _, mode := range []string{modeLegacy, modeLibrary} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				h := newOAuthHarness(t, mode)
				h.enable("oidc", tc.domain)
				if tc.existing != "" {
					storeUserForTest(t, h.db, tc.existing)
				}
				rec := h.signIn("oidc", tc.id)
				if got := oauthResult(rec); rec.Code != http.StatusFound || got != tc.want {
					t.Errorf("status = %d location = %q, want 302 %q", rec.Code, got, tc.want)
				}
			})
		}
	}
}

func TestOAuthGoldenExchangeFailure(t *testing.T) {
	for _, mode := range []string{modeLegacy, modeLibrary} {
		t.Run(mode, func(t *testing.T) {
			h := newOAuthHarness(t, mode)
			h.enable("oidc", "")
			_, state := h.start("oidc")
			rec := h.get(h.callbackPath("oidc")+"?state="+url.QueryEscape(state)+"&code=never-granted", true)
			if got := oauthResult(rec); got != "/login?oauth_error=exchange_failed" {
				t.Errorf("location = %q, want exchange_failed", got)
			}
		})
	}
}

func TestOAuthLibraryLinksVerifiedEmailsOnly(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		id       fakeIdentity
		want     string
		linked   bool
	}{
		{"verified email links the existing account", "oidc", fakeIdentity{Sub: "new-sub", Email: "owner@example.test", Verified: true}, "/oauth/complete", true},
		{"unverified email is refused", "microsoft", fakeIdentity{Sub: "graph-id", Email: "owner@example.test", Verified: false}, "/login?oauth_error=email_in_use", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable(tc.provider, "")
			owner := storeUserForTest(t, h.db, "owner@example.test")
			if _, err := authengine.Backfill(context.Background(), h.db.DB, authengine.BackfillOptions{EncryptionKey: h.key}); err != nil {
				t.Fatal(err)
			}
			rec := h.signIn(tc.provider, tc.id)
			if got := oauthResult(rec); got != tc.want {
				t.Fatalf("location = %q, want %q", got, tc.want)
			}
			ident, err := h.db.GetOAuthIdentity(context.Background(), tc.provider, tc.id.Sub)
			if tc.linked {
				if err != nil || ident.UserID != owner.ID {
					t.Errorf("identity %+v err %v, want linked to %s", ident, err, owner.ID)
				}
				if got, _ := h.sessionUser(rec); got != owner.ID {
					t.Errorf("session user = %q, want %q", got, owner.ID)
				}
			} else if err == nil {
				t.Errorf("unverified sign-in must not link, got %+v", ident)
			}
		})
	}
	t.Run("legacy refuses the same email", func(t *testing.T) {
		h := newOAuthHarness(t, modeLegacy)
		h.enable("oidc", "")
		storeUserForTest(t, h.db, "owner@example.test")
		rec := h.signIn("oidc", fakeIdentity{Sub: "new-sub", Email: "owner@example.test", Verified: true})
		if got := oauthResult(rec); got != "/login?oauth_error=email_in_use" {
			t.Errorf("location = %q, want email_in_use", got)
		}
	})
}

func TestOAuthIdentityBackfillParity(t *testing.T) {
	for _, mode := range []string{modeLegacy, modeLibrary} {
		t.Run(mode, func(t *testing.T) {
			h := newOAuthHarness(t, mode)
			h.enable("oidc", "")
			ctx := context.Background()
			owner := storeUserForTest(t, h.db, "linked@example.test")
			if err := h.db.SaveOAuthIdentity(ctx, store.OAuthIdentity{ID: "oid_seed", UserID: owner.ID, Provider: "oidc", ProviderUserID: "sub-existing"}); err != nil {
				t.Fatal(err)
			}
			if mode == modeLibrary {
				rep, err := authengine.Backfill(ctx, h.db.DB, authengine.BackfillOptions{EncryptionKey: h.key})
				if err != nil || rep.OAuthIdentities != 1 {
					t.Fatalf("backfill %+v err %v", rep, err)
				}
			}
			before := countUsers(t, h)
			rec := h.signIn("oidc", fakeIdentity{Sub: "sub-existing", Email: "renamed@example.test", Verified: true})
			if got := oauthResult(rec); got != "/oauth/complete" {
				t.Fatalf("location = %q, want /oauth/complete", got)
			}
			if got, _ := h.sessionUser(rec); got != owner.ID {
				t.Errorf("session user = %q, want the linked user %q", got, owner.ID)
			}
			if after := countUsers(t, h); after != before {
				t.Errorf("users %d -> %d, sign-in must not create one", before, after)
			}
		})
	}
}

func countUsers(t *testing.T, h *oauthHarness) int {
	t.Helper()
	var n int
	if err := h.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOAuthProviderEditedAtRuntime(t *testing.T) {
	t.Setenv(authengine.EnvOAuthProviderTTL, "1h")
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	admin := loginTestSession(t, h.rt, h.db)

	_, state := h.start("oidc")

	h.secretsDisable(t, "oidc")
	if rec := h.get(h.startPath("oidc"), false); rec.Code != http.StatusFound {
		t.Fatalf("edit without invalidation: status = %d, want the cached provider to still answer 302", rec.Code)
	}

	body := `{"enabled":false,"client_id":"client-oidc","issuer_url":"` + h.idp.Issuer() + `"}`
	req := authedRequest(t, admin, http.MethodPut, "/api/v1/settings/oauth/oidc", body)
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT settings: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec := h.get(h.startPath("oidc"), false); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "not enabled") {
		t.Errorf("start after removal: status = %d body = %q, want 400 not enabled", rec.Code, rec.Body.String())
	}
	cb := h.get(h.callbackPath("oidc")+"?state="+url.QueryEscape(state)+"&code=x", true)
	if got := oauthResult(cb); got != "/login?oauth_error=provider_disabled" {
		t.Errorf("callback after removal: location = %q, want provider_disabled", got)
	}
}

func (h *oauthHarness) secretsDisable(t *testing.T, provider string) {
	t.Helper()
	err := h.db.UpdateOAuthProviderSettings(context.Background(), store.OAuthProviderSettings{
		Provider: provider, Enabled: false, ClientID: "client-" + provider, IssuerURL: h.idp.Issuer(),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOAuthAreaGateKeepsLegacyWhenNotListed(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("oidc", "")
	t.Setenv(authengine.EnvAreas, string(authengine.AreaTokens))
	u, _ := h.start("oidc")
	want := testBaseURL + "/api/v1/auth/oauth/oidc/callback"
	if got := u.Query().Get("redirect_uri"); got != want {
		t.Errorf("redirect_uri = %q, want the legacy %q when oauth is not an active area", got, want)
	}
}
