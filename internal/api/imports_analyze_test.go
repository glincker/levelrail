package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/repolayout"
)

var nextAppFixture = map[string]string{ //nolint:gosec // fixture value proving env values are never returned
	"package.json": `{"scripts":{"start":"next start"},"dependencies":{"next":"14.2.0","pg":"8.11.0"}}`,
	".env.example": "DATABASE_URL=postgres://user:hunter2@localhost/app\nNEXTAUTH_SECRET=changeme\nSTRIPE_KEY=\n",
	"README.md":    "# shop\n",
}

func TestAnalyzeImport_NextWithEnvExample(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	useFakeLayout(rt, &fakeLayoutSource{files: nextAppFixture})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/analyze", `{"repo_url":"https://github.com/acme/shop","ref":"main","token":"ghp_supersecret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, leak := range []string{"hunter2", "ghp_supersecret", "changeme"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaks %q: %s", leak, body)
		}
	}
	var a appwizard.Analysis
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.Stack.Framework != "Next.js" || a.Port.Value != 3000 || a.Health.Value != "/" || a.Name.Value != "shop" {
		t.Fatalf("analysis = %+v", a)
	}
	if len(a.Databases) != 1 || a.Databases[0].Engine != "postgres" || a.Databases[0].Version == "" || a.Databases[0].EnvVar != "DATABASE_URL" {
		t.Fatalf("databases = %+v", a.Databases)
	}
	keys := map[string]appwizard.EnvKey{}
	for _, e := range a.Env {
		keys[e.Key] = e
	}
	if !keys["NEXTAUTH_SECRET"].Secret || !keys["NEXTAUTH_SECRET"].Required || keys["DATABASE_URL"].ProvidedBy == "" {
		t.Fatalf("env = %+v", a.Env)
	}
}

func TestAnalyzeImport_Errors(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	tests := []struct {
		name string
		body string
		src  repolayout.Source
		want int
	}{
		{"missing url", `{}`, nil, http.StatusBadRequest},
		{"not http", `{"repo_url":"ftp://github.com/a/b"}`, nil, http.StatusBadRequest},
		{"provider 404", `{"repo_url":"https://github.com/a/b","ref":"main"}`, &fakeLayoutSource{err: &repolayout.StatusError{Status: 404}}, http.StatusNotFound},
		{"provider 403", `{"repo_url":"https://github.com/a/b","ref":"main"}`, &fakeLayoutSource{err: &repolayout.StatusError{Status: 403}}, http.StatusUnprocessableEntity},
		{"no api host", `{"repo_url":"https://git.example.org/a/b"}`, nil, http.StatusUnprocessableEntity},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt.repoLayoutSource = nil
			if tc.src != nil {
				useFakeLayout(rt, tc.src)
			}
			rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/analyze", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestAnalyzeGitSource_DirectoryAndDetection(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{
		"package.json":          `{"workspaces":["apps/*"]}`,
		"apps/web/package.json": `{"dependencies":{"next":"14"}}`,
		"apps/web/.env.example": "API_TOKEN=abc123\n",
	}})

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/git-sources/analyze", `{"repo_url":"https://github.com/acme/mono","ref":"main","directory":"apps/web"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("response leaks an env value: %s", rec.Body.String())
	}
	var a appwizard.Analysis
	if err := json.Unmarshal(rec.Body.Bytes(), &a); err != nil {
		t.Fatal(err)
	}
	if a.Detection.Framework != "Next.js" || len(a.Detection.Reasons) == 0 || a.Build.BaseDirectory != "apps/web" {
		t.Fatalf("detection = %+v build = %+v", a.Detection, a.Build)
	}
	if len(a.Detection.EnvVars) != 1 || !a.Detection.EnvVars[0].SecretGuess {
		t.Fatalf("env vars = %+v", a.Detection.EnvVars)
	}
	if len(a.Packages) == 0 {
		t.Fatalf("expected workspace packages, got none")
	}
}
