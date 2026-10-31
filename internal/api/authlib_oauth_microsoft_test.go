package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const msTenantA = "11111111-2222-3333-4444-555555555555"

func (h *oauthHarness) setTenant(tenant, domain string) {
	h.t.Helper()
	s := store.OAuthProviderSettings{Provider: "microsoft", Enabled: true, ClientID: "client-microsoft", AllowedEmailDomain: domain, TenantID: tenant}
	if err := h.db.UpdateOAuthProviderSettings(context.Background(), s); err != nil {
		h.t.Fatal(err)
	}
}

func TestMicrosoftSharedTenantCreatesNoAccount(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("microsoft", "")
	before := countUsers(t, h)
	rec := h.signIn("microsoft", fakeIdentity{Sub: "g1", Email: "new@corp.test", Verified: true})
	if got := oauthResult(rec); got != "/login?oauth_error=sso_not_provisioned" {
		t.Fatalf("location = %q, want not provisioned", got)
	}
	if countUsers(t, h) != before {
		t.Error("an account was created from the shared tenant")
	}
}

func TestMicrosoftSharedTenantLinkedIdentityStillSignsIn(t *testing.T) {
	h := newOAuthHarness(t, modeLibrary)
	h.enable("microsoft", "")
	ctx := context.Background()
	owner := storeUserForTest(t, h.db, "linked@corp.test")
	if err := h.db.SaveOAuthIdentity(ctx, store.OAuthIdentity{ID: "oid_ms", UserID: owner.ID, Provider: "microsoft", ProviderUserID: "g2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.rt.libSessions.SyncUser(ctx, toLibUser(owner)); err != nil {
		t.Fatal(err)
	}
	if err := h.eng.LinkOAuthIdentity(ctx, owner.ID, "microsoft", "g2"); err != nil {
		t.Fatal(err)
	}
	rec := h.signIn("microsoft", fakeIdentity{Sub: "g2", Email: "linked@corp.test", Verified: true})
	if got := oauthResult(rec); got != "/oauth/complete" {
		t.Errorf("location = %q, want /oauth/complete", got)
	}
}

func TestMicrosoftTenantConfigGatesSignup(t *testing.T) {
	tests := []struct {
		name   string
		domain string
		id     fakeIdentity
		want   string
	}{
		{"matching tenant creates the account", "", fakeIdentity{Sub: "t1", Email: "a@corp.test", Verified: true, Tenant: msTenantA}, "/oauth/complete"},
		{"tenant id compared without case", "", fakeIdentity{Sub: "t2", Email: "b@corp.test", Verified: true, Tenant: "11111111-2222-3333-4444-555555555555"}, "/oauth/complete"},
		{"other tenant is refused", "", fakeIdentity{Sub: "t3", Email: "c@corp.test", Verified: true, Tenant: "99999999-2222-3333-4444-555555555555"}, "/login?oauth_error=userinfo_failed"},
		{"opaque token is refused", "", fakeIdentity{Sub: "t4", Email: "d@corp.test", Verified: true}, "/login?oauth_error=userinfo_failed"},
		{"allowed domain still applies", "corp.test", fakeIdentity{Sub: "t5", Email: "e@other.test", Verified: true, Tenant: msTenantA}, "/login?oauth_error=domain_not_allowed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newOAuthHarness(t, modeLibrary)
			h.enable("microsoft", tc.domain)
			h.setTenant(msTenantA, tc.domain)
			rec := h.signIn("microsoft", tc.id)
			if got := oauthResult(rec); got != tc.want {
				t.Errorf("location = %q, want %q", got, tc.want)
			}
		})
	}
}
