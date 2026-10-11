package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/repolayout"
)

type fakeLayoutSource struct {
	files map[string]string
	err   error
}

func (f *fakeLayoutSource) Paths(context.Context) ([]string, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	out := make([]string, 0, len(f.files))
	for p := range f.files {
		out = append(out, p)
	}
	return out, false, nil
}

func (f *fakeLayoutSource) Read(_ context.Context, p string, _ int) ([]byte, error) {
	return []byte(f.files[p]), nil
}

func useFakeLayout(rt *Router, src repolayout.Source) {
	rt.repoLayoutSource = func(context.Context, string, string, string, repolayout.Hosts) (repolayout.Source, error) {
		return src, nil
	}
}

var turborepoFixture = map[string]string{
	"turbo.json":                   "{}",
	"package.json":                 `{"private":true,"workspaces":["apps/*"]}`,
	"apps/glinr/package.json":      "{}",
	"apps/glinr/deploy/Dockerfile": "FROM node:20\nRUN npx turbo prune glinr --docker\n",
	"apps/docs/package.json":       "{}",
}

func layoutReq(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, target, body))
	return rec
}

func TestGitSourceBuild_TurborepoDetectApplyAndPushDeploy(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/mono.git","branch":"main","build_type":"railpack"}`)
	useFakeLayout(rt, &fakeLayoutSource{files: turborepoFixture})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/detect", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("detect status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var det detectGitSourceResponse
	if err := json.NewDecoder(rec.Body).Decode(&det); err != nil {
		t.Fatal(err)
	}
	if !det.NeedsBuildSettings || !det.LooksLikeMonorepo || det.RootHasApp {
		t.Fatalf("detection = %+v, want a monorepo needing build settings", det)
	}
	top := det.Suggestions[0]
	if top.DockerfilePath != "apps/glinr/deploy/Dockerfile" || top.BaseDirectory != "" || top.BuildType != "dockerfile" || !top.Recommended {
		t.Fatalf("top suggestion = %+v", top)
	}

	body := `{"build_type":"` + top.BuildType + `","build_path":"` + top.DockerfilePath + `","base_directory":""}`
	rec = layoutReq(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/git-source/build", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res gitSourceResource
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.BuildType != "dockerfile" || res.BuildPath != "apps/glinr/deploy/Dockerfile" || res.ResolvedBuild.ContextDir != "." {
		t.Fatalf("resource after apply = %+v", res)
	}

	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb
	pb := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(pb)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), pb))
	prec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(prec, req)
	if prec.Code != http.StatusOK {
		t.Fatalf("push status = %d, body = %s", prec.Code, prec.Body.String())
	}
	b := fb.lastReq.Service.Build
	if b.Type != "dockerfile" || b.Path != "apps/glinr/deploy/Dockerfile" || b.BaseDirectory != "" {
		t.Fatalf("deploy build = %+v, want dockerfile at apps/glinr/deploy/Dockerfile with the repo root as context", b)
	}
}

func TestGitSourceBuild_BaseDirectoryMakesPathContextRelative(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/mono.git","branch":"main","build_type":"dockerfile","base_directory":"apps/api","build_path":"apps/api/Dockerfile.prod"}`)
	if created.BaseDirectory != "apps/api" || created.ResolvedBuild.ContextDir != "apps/api" {
		t.Fatalf("created = %+v", created)
	}
	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb
	rt.gitSourceFetch = newFakeGitSourceFetch(new([]fakeGitSourceFetchCall), t.TempDir(), new(bool), nil)
	pb := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(pb)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), pb))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("push status = %d, body = %s", rec.Code, rec.Body.String())
	}
	b := fb.lastReq.Service.Build
	if b.BaseDirectory != "apps/api" || b.Path != "Dockerfile.prod" {
		t.Fatalf("deploy build = %+v, want base apps/api and path Dockerfile.prod", b)
	}
}

func TestGitSourceBuild_RejectsBadPaths(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/mono.git","branch":"main","build_type":"railpack"}`)

	cases := []struct{ name, body string }{
		{"parent escape", `{"build_type":"dockerfile","base_directory":"../etc"}`},
		{"absolute base", `{"build_type":"dockerfile","base_directory":"/srv"}`},
		{"absolute dockerfile", `{"build_type":"dockerfile","build_path":"/Dockerfile"}`},
		{"dockerfile outside base", `{"build_type":"dockerfile","base_directory":"apps/api","build_path":"Dockerfile"}`},
		{"path with auto-detect", `{"build_type":"railpack","build_path":"Dockerfile"}`},
		{"unknown type", `{"build_type":"nope"}`},
		{"image type", `{"build_type":"image"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := layoutReq(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/git-source/build", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
	gs, err := db.GetGitSource(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if gs.BuildType != "railpack" || gs.BaseDirectory != "" {
		t.Fatalf("a rejected request changed the source: %+v", gs)
	}
}

func TestGitSourceBuild_ConnectRejectsEscapingBaseDirectory(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := layoutReq(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/git-source",
		`{"repo_url":"https://github.com/org/web.git","build_type":"dockerfile","base_directory":"../../x"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestGitSourceBuild_DetectNoNudgeForSingleApp(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"railpack"}`)
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{"package.json": `{"scripts":{"start":"node ."}}`, "index.js": ""}})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/detect", ``)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var det detectGitSourceResponse
	if err := json.NewDecoder(rec.Body).Decode(&det); err != nil {
		t.Fatal(err)
	}
	if det.NeedsBuildSettings || !det.RootHasApp {
		t.Fatalf("detection = %+v, want no nudge for a single app at the root", det)
	}
}

func TestGitSourceBuild_DetectMapsProviderErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"not found", &repolayout.StatusError{Status: 404}, http.StatusNotFound},
		{"forbidden", &repolayout.StatusError{Status: 403}, http.StatusUnprocessableEntity},
		{"upstream failure", &repolayout.StatusError{Status: 500}, http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
			useFakeLayout(rt, &fakeLayoutSource{err: tc.err})
			rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/detect", `{}`)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}

func TestGitSourceBuild_DetectRequiresConnectedSource(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/detect", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
