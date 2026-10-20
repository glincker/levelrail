package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestGitSourceStaticBuild_SetAndValidate(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCmd    string
	}{
		{"built site", `{"build_type":"static","build_command":"npm run build","publish_dir":"dist"}`, http.StatusOK, "npm run build"},
		{"nested base", `{"build_type":"static","base_directory":"apps/web","build_command":"npm run build","publish_dir":"dist"}`, http.StatusOK, "npm run build"},
		{"served as-is", `{"build_type":"static","build_path":"public"}`, http.StatusOK, ""},
		{"command without publish", `{"build_type":"static","build_command":"npm run build"}`, http.StatusBadRequest, ""},
		{"publish escapes", `{"build_type":"static","build_command":"make","publish_dir":"../x"}`, http.StatusBadRequest, ""},
		{"command on railpack", `{"build_type":"railpack","build_command":"make","publish_dir":"dist"}`, http.StatusBadRequest, ""},
		{"path and command", `{"build_type":"static","build_path":"public","build_command":"make","publish_dir":"dist"}`, http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"railpack"}`)

			rec := layoutReq(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/git-source/build", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var res gitSourceResource
			if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
				t.Fatal(err)
			}
			if res.BuildCommand != tt.wantCmd {
				t.Fatalf("build_command = %q", res.BuildCommand)
			}
			gs, err := db.GetGitSource(t.Context(), "web")
			if err != nil {
				t.Fatal(err)
			}
			b, err := gs.SpecBuild()
			if err != nil {
				t.Fatal(err)
			}
			if b.Command != tt.wantCmd || b.RunsStaticBuild() != (tt.wantCmd != "") {
				t.Fatalf("SpecBuild() = %+v", b)
			}
		})
	}
}

func TestGitSourceStaticBuild_DetectPrefillsFromPreset(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string
		wantPublish string
	}{
		{"vite", map[string]string{"package.json": `{"devDependencies":{"vite":"5"}}`}, "dist"},
		{"astro", map[string]string{"package.json": `{"dependencies":{"astro":"4"}}`}, "dist"},
		{"docusaurus", map[string]string{"package.json": `{"dependencies":{"@docusaurus/core":"3"}}`}, "build"},
		{"server framework", map[string]string{"package.json": `{"dependencies":{"express":"4"}}`}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/site.git","branch":"main","build_type":"static"}`)
			useFakeLayout(rt, &fakeLayoutSource{files: tt.files})

			rec := layoutReq(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/git-source/detect", `{}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var det detectGitSourceResponse
			if err := json.NewDecoder(rec.Body).Decode(&det); err != nil {
				t.Fatal(err)
			}
			if tt.wantPublish == "" {
				if det.StaticBuild != nil {
					t.Fatalf("unexpected suggestion %+v", det.StaticBuild)
				}
				return
			}
			if det.StaticBuild == nil || det.StaticBuild.PublishDir != tt.wantPublish || det.StaticBuild.Command == "" {
				t.Fatalf("suggestion = %+v", det.StaticBuild)
			}
		})
	}
}
