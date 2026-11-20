package api

import (
	"net/http"
	"strings"
	"testing"
)

func mintSiteDeployToken(t *testing.T, rt *Router, cookie *http.Cookie, site string) createTokenResponse {
	t.Helper()
	var out createTokenResponse
	req := authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites/"+site+"/deploy-tokens", `{"name":"gha"}`)
	if code := iacCall(t, rt, req, &out); code != http.StatusCreated || out.Token == "" {
		t.Fatalf("mint site token = %d", code)
	}
	return out
}

func TestSiteDeployTokenCannotTouchOtherSitesOrRoutes(t *testing.T) {
	rt, db, cookie := hookTestSite(t, false)
	seedApp(t, db, "other")
	tok := mintSiteDeployToken(t, rt, cookie, "docs").Token

	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/apps/other"},
		{http.MethodPost, "/api/v1/apps/other/uploads"},
		{http.MethodPost, "/api/v1/apps/other/builds"},
		{http.MethodGet, "/api/v1/apps"},
		{http.MethodDelete, "/api/v1/apps/docs"},
		{http.MethodGet, "/api/v1/apps/docs/status"},
		{http.MethodPost, "/api/v1/static-sites/docs/deploy-tokens"},
		{http.MethodPost, "/api/v1/auth/tokens"},
	} {
		if code := callWithToken(t, rt, tok, c.method, c.path, "{}"); code != http.StatusForbidden && code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want denied", c.method, c.path, code)
		}
	}
	for _, p := range []string{"/api/v1/apps/docs", "/api/v1/apps/docs/deploys"} {
		if code := callWithToken(t, rt, tok, http.MethodGet, p, ""); code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", p, code)
		}
	}
}

func TestSiteDeployTokenListedWithoutSecretAndRevocable(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	minted := mintSiteDeployToken(t, rt, cookie, "docs")
	if minted.ExpiresAt == nil {
		t.Fatal("expected a default expiry")
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/docs/deploy-tokens", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), minted.ID) || strings.Contains(rec.Body.String(), minted.Token) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	del := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/static-sites/docs/deploy-tokens/"+minted.ID, ""))
	if del.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d", del.Code)
	}
	if code := callWithToken(t, rt, minted.Token, http.MethodGet, "/api/v1/apps/docs", ""); code != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d, want 401", code)
	}
	if del := serve(rt, authedRequest(t, cookie, http.MethodDelete, "/api/v1/static-sites/other/deploy-tokens/"+minted.ID, "")); del.Code == http.StatusNoContent {
		t.Fatal("revoking through another site must not succeed")
	}
}
