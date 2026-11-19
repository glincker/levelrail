package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
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

// failingSetConn makes the account link fail after the source was saved.
type failingSetConn struct{ GitConnectionsStore }

func (failingSetConn) GetGitConnection(context.Context, string) (store.GitConnection, error) {
	return store.GitConnection{ID: "c1", InstanceURL: "https://github.com"}, nil
}

func (failingSetConn) SetGitSourceConnection(context.Context, string, string) error {
	return errors.New("link failed")
}

func TestHandleCreateStaticSiteRollsBackOnGitFailure(t *testing.T) {
	const body = `{"name":"gsite","git":{"repo_url":"https://github.com/org/site.git"}}`
	cases := map[string]struct {
		setup func(*Router, *fakeGitSourceSecrets)
		body  string
	}{
		"connect fails":  {func(_ *Router, f *fakeGitSourceSecrets) { f.setErr = errors.New("boom") }, body},
		"settings check": {func(_ *Router, f *fakeGitSourceSecrets) { f.existsErr = errors.New("boom") }, body},
		"connection set": {func(rt *Router, _ *fakeGitSourceSecrets) { rt.gitConns = failingSetConn{rt.gitConns} }, `{"name":"gsite","git":{"repo_url":"https://github.com/org/site.git","account_id":"c1"}}`},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			secrets := newFakeGitSourceSecrets()
			rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
			cookie := loginTestSession(t, rt, db)
			tc.setup(rt, secrets)
			do := func(method, path, b string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, b))
				return rec
			}
			if rec := do(http.MethodPost, "/api/v1/static-sites", tc.body); rec.Code != http.StatusInternalServerError {
				t.Fatalf("create = %d %s, want 500", rec.Code, rec.Body.String())
			}
			if rec := do(http.MethodGet, "/api/v1/static-sites/gsite", ""); rec.Code != http.StatusNotFound {
				t.Fatalf("app left behind after failed create: %d", rec.Code)
			}
		})
	}
}

func TestHandleCreateStaticSiteKeepsExistingAppOnConflict(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	do := func(b string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", b))
		return rec
	}
	if rec := do(`{"name":"keep"}`); rec.Code != http.StatusCreated {
		t.Fatalf("seed = %d", rec.Code)
	}
	secrets.setErr = errors.New("boom")
	if rec := do(`{"name":"keep","git":{"repo_url":"https://github.com/org/site.git"}}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d, want 409", rec.Code)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/static-sites/keep", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("existing app touched: %d", rec.Code)
	}
}

func deployRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(newFakeGitSourceSecrets()), WithBuilder(&fakeBuilder{}))
	return rt, db, loginTestSession(t, rt, db)
}

func TestHandleCreateStaticSiteDeployID(t *testing.T) {
	const body = `{"name":"dsite","deploy":true,"git":{"repo_url":"https://git.invalid/org/site.git"}}`
	t.Run("returned when the attempt appears", func(t *testing.T) {
		t.Setenv(EnvPagesDeployIDWait, "5s")
		rt, db, cookie := deployRouter(t)
		go func() {
			for i := 0; i < 100; i++ {
				time.Sleep(50 * time.Millisecond)
				a := store.DeployAttempt{ID: "dep_seed", ServiceName: "dsite", Source: store.DeployAttemptSourceManual, Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}
				now := time.Now()
				a.FinishedAt = &now
				if db.SaveDeployAttempt(context.Background(), a) == nil {
					return
				}
			}
		}()
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", body))
		var got createStaticSiteResponse
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.DeployID != "dep_seed" {
			t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("omitted without error when none appears", func(t *testing.T) {
		t.Setenv(EnvPagesDeployIDWait, "200ms")
		rt, _, cookie := deployRouter(t)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/static-sites", body))
		var got createStaticSiteResponse
		if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.DeployID != "" || got.GitSource == nil {
			t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
		}
	})
}

// hostConn is a connection on a fixed instance, to check the repo host match.
type hostConn struct{ GitConnectionsStore }

func (hostConn) GetGitConnection(context.Context, string) (store.GitConnection, error) {
	return store.GitConnection{ID: "c1", InstanceURL: "https://gitlab.example.com"}, nil
}

func TestCreateStaticSiteMatchesAccountHostAndNormalisesOutput(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	rt.gitConns = hostConn{rt.gitConns}
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	if rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"hostx","git":{"repo_url":"https://github.com/org/site.git","account_id":"c1"}}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("repo on another host = %d %s, want 400", rec.Code, rec.Body.String())
	}
	rec := do(http.MethodPost, "/api/v1/static-sites", `{"name":"normo","git":{"repo_url":"https://github.com/org/site.git","build_command":"make","output_dir":"./dist/"}}`)
	var got createStaticSiteResponse
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.GitSource == nil || got.GitSource.PublishDir != "dist" {
		t.Fatalf("normalised create: %d %s", rec.Code, rec.Body.String())
	}
}
