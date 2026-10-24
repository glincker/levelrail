package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/repoplan"
)

var planMonoFixture = map[string]string{
	"pnpm-workspace.yaml":      "packages:\n  - 'apps/*'\n  - 'packages/*'\n",
	"apps/web/package.json":    `{"name":"web","dependencies":{"next":"14","ui":"workspace:*"}}`,
	"apps/api/package.json":    `{"name":"api","dependencies":{"express":"4"}}`,
	"apps/api/Dockerfile":      "FROM node:20\nEXPOSE 3000\n",
	"packages/ui/package.json": `{"name":"ui"}`,
}

func TestImportRepoPlan_ListsPackagesAndCreatesNothing(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "mono-web")
	useFakeLayout(rt, &fakeLayoutSource{files: planMonoFixture})
	before, _ := rt.apps.ListDesiredServices(t.Context())

	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/repo/plan", `{"repo_url":"https://github.com/acme/mono","branch":"main","token":"ghp_supersecret"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "ghp_supersecret") {
		t.Fatalf("plan leaks the token: %s", rec.Body.String())
	}
	var plan repoplan.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	rows := map[string]repoplan.Row{}
	for _, r := range plan.Packages {
		rows[r.Directory] = r
	}
	if len(rows) != 3 || plan.Tool != "pnpm-workspace" || plan.Totals.Packages != 3 {
		t.Fatalf("plan = %+v", plan)
	}
	if web := rows["apps/web"]; web.Name != "mono-web-2" || web.BuildKind != repoplan.KindBuildpack || web.Port == nil || *web.Port != 3000 {
		t.Fatalf("web row = %+v", web)
	}
	if api := rows["apps/api"]; api.Name != "mono-api" || api.BuildKind != repoplan.KindDockerfile || api.Port == nil || *api.Port != 3000 {
		t.Fatalf("api row = %+v", api)
	}
	after, _ := rt.apps.ListDesiredServices(t.Context())
	if len(after) != len(before) {
		t.Fatalf("plan created rows: %d -> %d", len(before), len(after))
	}
}

func TestImportRepoPlan_BaseDirectoryAndErrors(t *testing.T) {
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
		{"bad json", `{`, nil, http.StatusBadRequest},
		{"provider 404", `{"repo_url":"https://github.com/a/b","branch":"main"}`, &fakeLayoutSource{err: &repolayout.StatusError{Status: 404}}, http.StatusNotFound},
		{"provider 403", `{"repo_url":"https://github.com/a/b","branch":"main"}`, &fakeLayoutSource{err: &repolayout.StatusError{Status: 403}}, http.StatusUnprocessableEntity},
		{"base dir", `{"repo_url":"https://github.com/a/b","branch":"main","base_directory":"packages"}`, &fakeLayoutSource{files: planMonoFixture}, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt.repoLayoutSource = nil
			if tc.src != nil {
				useFakeLayout(rt, tc.src)
			}
			rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/repo/plan", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.name == "base dir" {
				var plan repoplan.Plan
				if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil || len(plan.Packages) != 1 || plan.Packages[0].Directory != "packages/ui" {
					t.Fatalf("plan = %+v err = %v", plan, err)
				}
			}
		})
	}
}

func TestImportRepoPlan_PackageCapFromEnv(t *testing.T) {
	t.Setenv(repolayout.EnvMaxPackages, "1")
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	useFakeLayout(rt, &fakeLayoutSource{files: planMonoFixture})
	rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/imports/repo/plan", `{"repo_url":"https://github.com/acme/mono","branch":"main"}`)
	var plan repoplan.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.PackageCap != 1 || plan.PackagesFound != 3 || len(plan.Packages) != 1 || len(plan.Warnings) == 0 || plan.Warnings[0].Code != repoplan.WarnPackageCap {
		t.Fatalf("plan = %+v", plan)
	}
}
