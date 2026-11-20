package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestSiteDeployTokenRotationKeepsSiteScope(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	old := mintSiteDeployToken(t, rt, cookie)
	rec := rotateAs(t, rt, cookie, old.ID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rotate = %d %s", rec.Code, rec.Body.String())
	}
	var out rotateTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Scope == nil || out.Scope.Site != "docs" {
		t.Fatalf("replacement scope = %+v, want site docs", out.Scope)
	}
	if code := callWithToken(t, rt, out.Token, http.MethodGet, "/api/v1/apps", ""); code != http.StatusForbidden {
		t.Fatalf("rotated site token on /apps = %d, want 403", code)
	}
}

func TestSiteDeployTokenCannotStartNonStaticBuild(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	rt.builder = &fakeBuilder{tag: "img:sha"}
	tok := mintSiteDeployToken(t, rt, cookie).Token
	for _, body := range []string{
		`{"build":{"type":"image","image":"x:1"}}`,
		`{"build":{"type":"dockerfile"},"upload_hash":"abc"}`,
		`{"build":{"type":"static"}}`,
		`{"build":{"type":"static"},"upload_hash":"abc","repo_url":"https://x.invalid/a.git"}`,
		`{"build":{"type":"static"},"upload_hash":"abc","source_url":"https://x.invalid/a.zip"}`,
		`{"build":{"type":"static","args":{"A":"b"}},"upload_hash":"abc"}`,
		`{"build":{"type":"static"},"upload_hash":"abc","override_freeze":true,"override_reason":"x"}`,
	} {
		if code := callWithToken(t, rt, tok, http.MethodPost, "/api/v1/apps/docs/builds", body); code != http.StatusForbidden {
			t.Errorf("build %s = %d, want 403", body, code)
		}
	}
}

func TestSiteDeployTokenMintAndRevokeNeedASession(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	minted := mintSiteDeployToken(t, rt, cookie)
	// A plain bearer token with deploy rights must not mint or revoke site tokens.
	plain := mintTokenAs(t, rt, cookie, "plain")
	if code := callWithToken(t, rt, plain.Token, http.MethodPost, "/api/v1/static-sites/docs/deploy-tokens", `{}`); code != http.StatusForbidden {
		t.Fatalf("bearer mint = %d, want 403", code)
	}
	if code := callWithToken(t, rt, plain.Token, http.MethodDelete, "/api/v1/static-sites/docs/deploy-tokens/"+minted.ID, ""); code != http.StatusForbidden {
		t.Fatalf("bearer revoke = %d, want 403", code)
	}
}

func TestDeletingTheSiteRevokesItsTokens(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	minted := mintSiteDeployToken(t, rt, cookie)
	rt.revokeSiteTokens(context.Background(), "docs")
	if tokenUsable(rt, minted.Token) {
		t.Fatal("site token must be revoked with its site")
	}
	if ids, err := rt.tokenSiteScopes().ListAPITokenIDsForSite(context.Background(), "docs"); err != nil || len(ids) != 0 {
		t.Fatalf("scope rows = %v, %v", ids, err)
	}
}

func TestSiteTokenOfDeletedAppDoesNotCarryToANewApp(t *testing.T) {
	rt, db, cookie := hookTestSite(t, false)
	tok := mintSiteDeployToken(t, rt, cookie).Token
	if err := db.DeleteDesiredService(context.Background(), "docs"); err != nil {
		t.Fatal(err)
	}
	seedApp(t, db, "docs")
	if code := callWithToken(t, rt, tok, http.MethodGet, "/api/v1/apps/docs", ""); code == http.StatusOK {
		t.Fatalf("old site token reached a recreated app: %d", code)
	}
}

func TestLeavingATeamRevokesOwnedSiteTokens(t *testing.T) {
	rt, _, cookie := hookTestSite(t, false)
	minted := mintSiteDeployToken(t, rt, cookie)
	if !tokenUsable(rt, minted.Token) {
		t.Fatal("token should start valid")
	}
	rt.revokeOwnedSiteTokens(context.Background(), minted.OwnerUserID)
	if tokenUsable(rt, minted.Token) {
		t.Fatal("owned site token must be revoked")
	}
}
