package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitSourceReconnect_KeepsStaticSPAWhenOmitted(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"static","static_spa":true}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/site.git","branch":"dev","build_type":"static"}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"static_spa":true`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGitSourceReconnect_ExplicitFalseClearsStaticSPA(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"static","static_spa":true}`)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source", `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"static","static_spa":false}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"static_spa":false`) {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGitPushStaticSPA_CarriesStoredFlag(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"static","static_spa":true}`)

	ctx := context.Background()
	if !rt.gitPushStaticSPA(ctx, "web", "static") {
		t.Fatal("stored static_spa was not applied to the push build")
	}
	if rt.gitPushStaticSPA(ctx, "web", "railpack") {
		t.Fatal("static_spa must not apply to a non static build")
	}
}
