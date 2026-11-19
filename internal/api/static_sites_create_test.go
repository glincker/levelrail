package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleCreateStaticSite(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	do := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", body))
		return rec
	}

	rec := do(`{"name":"my-pages"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var got staticSiteDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "my-pages" || got.BuildType != "static" {
		t.Errorf("unexpected body: %+v", got)
	}
	if rec := do(`{"name":"my-pages"}`); rec.Code != http.StatusConflict {
		t.Errorf("duplicate = %d, want 409", rec.Code)
	}
	if rec := do(`{"name":""}`); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", rec.Code)
	}
}

func TestHandleCreateStaticSiteWithGit(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"gsite","git":{"repo_url":"https://github.com/org/site.git","branch":"main","build_command":"npm run build","output_dir":"dist","deploy_on_push":false}}`)
	var got createStaticSiteResponse
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.GitSource == nil || got.GitSource.TriggerMode != "manual" || got.GitSource.PublishDir != "dist" || got.GitSource.BuildCommand != "npm run build" {
		t.Fatalf("create with git: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"nogit","deploy":true}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("deploy without git = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"badout","git":{"repo_url":"https://github.com/org/site.git","build_command":"make","output_dir":"../x"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("escaping output dir = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"badacct","git":{"repo_url":"https://github.com/org/site.git","account_id":"nope"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown account = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodGet, "/api/v1/static-sites/badout", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("a rejected create must not leave an app, got %d", rec.Code)
	}
}
