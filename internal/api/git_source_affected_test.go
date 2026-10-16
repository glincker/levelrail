package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGitSourceAffected(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/mono.git","branch":"main","build_type":"railpack"}`)
	useFakeLayout(rt, &fakeLayoutSource{files: map[string]string{
		"pnpm-workspace.yaml":       "packages:\n  - 'services/*'\n",
		"pnpm-lock.yaml":            "",
		"services/api/package.json": `{"name":"api"}`,
		"services/web/package.json": `{"name":"web"}`,
	}})

	cases := []struct {
		name  string
		body  string
		code  int
		check map[string]bool
	}{
		{"one service", `{"files":["services/web/src/a.ts"]}`, 200, map[string]bool{"services/web": true, "services/api": false}},
		{"lockfile hits all", `{"files":["pnpm-lock.yaml"]}`, 200, map[string]bool{"services/web": true, "services/api": true}},
		{"empty files rejected", `{"files":[]}`, 400, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/affected", tc.body)
			if rec.Code != tc.code {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if tc.check == nil {
				return
			}
			var out affectedResponse
			if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			for _, p := range out.Packages {
				if want, ok := tc.check[p.Dir]; ok && p.Affected != want {
					t.Errorf("%s affected = %v, want %v", p.Dir, p.Affected, want)
				}
			}
			if !out.AppDeploys {
				t.Errorf("app without deploy paths should always deploy")
			}
		})
	}
}
