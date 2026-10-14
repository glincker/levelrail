package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestSAMLSCIMRoutesFollowTheFeatureFlag(t *testing.T) {
	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/saml/providers"},
		{http.MethodGet, "/api/v1/auth/saml/x/metadata"},
		{http.MethodGet, "/api/v1/auth/saml/x/login"},
		{http.MethodPost, "/api/v1/auth/saml/x/acs"},
		{http.MethodGet, "/api/v1/sso/saml/connections"},
		{http.MethodGet, "/api/v1/sso/settings"},
		{http.MethodGet, "/api/v1/sso/group-mappings"},
		{http.MethodGet, "/api/v1/sso/scim/tokens"},
		{http.MethodGet, "/api/v1/scim/v2/Users"},
		{http.MethodGet, "/api/v1/scim/v2/ServiceProviderConfig"},
	}
	h := newSAMLHarness(t, nil)
	serve := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	for _, r := range routes {
		rec := serve(r.method, r.path)
		if rec.Code != http.StatusNotFound || policyCode(rec) != ExperimentalDisabledCode {
			t.Errorf("flag off %s %s: %d %s", r.method, r.path, rec.Code, rec.Body)
		}
	}
	if feats := enabledFeatures(); len(feats) != 0 {
		t.Errorf("enabledFeatures with the flag off = %v", feats)
	}

	experimental.Set(experimental.SAMLSCIM)
	for _, r := range routes {
		if rec := serve(r.method, r.path); policyCode(rec) == ExperimentalDisabledCode {
			t.Errorf("flag on %s %s still gated: %d", r.method, r.path, rec.Code)
		}
	}
}

func TestSSOEnforcementIgnoredWhileSAMLIsOff(t *testing.T) {
	h := newSAMLHarness(t, nil)
	if rec := h.admin2(http.MethodPut, "/api/v1/sso/settings", map[string]any{"enforce_sso": true}); rec.Code != http.StatusOK {
		t.Fatalf("enforce: %d %s", rec.Code, rec.Body)
	}
	dev := storeUserWithAbilitiesForTest(t, h.db, "gate-dev@example.com", []string{"read"})
	ctx := context.Background()

	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	if h.rt.ssoBlocksLocalSignIn(ctx, dev.ID) {
		t.Fatal("enforcement locked out password sign-in while SAML is gated off")
	}
	experimental.Set(experimental.SAMLSCIM)
	if !h.rt.ssoBlocksLocalSignIn(ctx, dev.ID) {
		t.Fatal("enforcement stopped applying once SAML is on")
	}
}
