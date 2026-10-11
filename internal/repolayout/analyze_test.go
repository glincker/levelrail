package repolayout

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func readerFrom(files map[string]string) Reader {
	return func(p string, _ int) ([]byte, error) {
		s, ok := files[p]
		if !ok {
			return nil, errors.New("missing")
		}
		return []byte(s), nil
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestAnalyze(t *testing.T) {
	cases := []struct {
		name         string
		files        map[string]string
		wantMonorepo bool
		wantRootApp  bool
		wantTop      Suggestion
		wantNone     bool
		wantTools    []string
	}{
		{
			name: "turborepo with turbo prune dockerfile",
			files: map[string]string{
				"turbo.json":                      "{}",
				"package.json":                    `{"private": true, "workspaces": ["apps/*"]}`,
				"apps/glinr/package.json":         "{}",
				"apps/glinr/deploy/Dockerfile":    "FROM node\nRUN turbo prune glinr --docker\n",
				"apps/docs/package.json":          "{}",
				"node_modules/foo/Dockerfile":     "FROM scratch",
				"packages/ui/vendor/x/Dockerfile": "FROM scratch",
			},
			wantMonorepo: true,
			wantTop:      Suggestion{BuildType: "dockerfile", DockerfilePath: "apps/glinr/deploy/Dockerfile", ReasonCode: ReasonTurboPrune},
			wantTools:    []string{"turbo", "npm-workspaces"},
		},
		{
			name: "pnpm workspace copying whole repo",
			files: map[string]string{
				"pnpm-workspace.yaml":   "packages: ['apps/*']",
				"package.json":          "{}",
				"apps/web/Dockerfile":   "FROM node\nCOPY . .\nRUN pnpm i\n",
				"apps/web/package.json": "{}",
				"apps/api/package.json": "{}",
			},
			wantMonorepo: true,
			wantTop:      Suggestion{BuildType: "dockerfile", DockerfilePath: "apps/web/Dockerfile", ReasonCode: ReasonRootCopy},
			wantTools:    []string{"pnpm-workspace"},
		},
		{
			name: "plain single app with root dockerfile",
			files: map[string]string{
				"Dockerfile":   "FROM node",
				"package.json": `{"scripts":{"start":"node ."}}`,
				"src/index.js": "",
			},
			wantRootApp: true,
			wantTop:     Suggestion{BuildType: "dockerfile", DockerfilePath: "Dockerfile", ReasonCode: ReasonRootDockerfile},
			wantTools:   []string{},
		},
		{
			name: "plain app without dockerfile has no suggestions",
			files: map[string]string{
				"package.json": `{"scripts":{"start":"node ."}}`,
				"index.js":     "",
			},
			wantRootApp: true,
			wantNone:    true,
			wantTools:   []string{},
		},
		{
			name: "nested dockerfiles without workspace tooling",
			files: map[string]string{
				"api/Dockerfile": "FROM golang\nCOPY go.mod .\n",
				"web/Dockerfile": "FROM node\n",
				"README.md":      "",
			},
			wantMonorepo: true,
			wantTop:      Suggestion{BuildType: "dockerfile", DockerfilePath: "api/Dockerfile", BaseDirectory: "api", ReasonCode: ReasonNestedDocker},
			wantTools:    []string{},
		},
		{
			name: "deploy directory of an app dir builds from root",
			files: map[string]string{
				"apps/site/package.json":      "{}",
				"apps/site/deploy/Dockerfile": "FROM node\nCOPY apps/site .\n",
				"apps/other/package.json":     "{}",
			},
			wantMonorepo: true,
			wantTop:      Suggestion{BuildType: "dockerfile", DockerfilePath: "apps/site/deploy/Dockerfile", ReasonCode: ReasonAppDeployDir},
			wantTools:    []string{},
		},
		{
			name: "ignored directories only",
			files: map[string]string{
				"node_modules/a/Dockerfile": "FROM scratch",
				"vendor/b/Dockerfile":       "FROM scratch",
			},
			wantNone:  true,
			wantTools: []string{},
		},
		{
			name: "go workspace without dockerfiles suggests railpack per app",
			files: map[string]string{
				"go.work":           "go 1.22",
				"services/a/go.mod": "module a",
				"services/b/go.mod": "module b",
			},
			wantMonorepo: true,
			wantTop:      Suggestion{BuildType: "railpack", BaseDirectory: "services/a", ReasonCode: ReasonAppRailpack},
			wantTools:    []string{"go-work"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := Analyze(keys(tc.files), readerFrom(tc.files))
			if res.LooksLikeMonorepo != tc.wantMonorepo {
				t.Errorf("LooksLikeMonorepo = %v, want %v", res.LooksLikeMonorepo, tc.wantMonorepo)
			}
			if res.RootHasApp != tc.wantRootApp {
				t.Errorf("RootHasApp = %v, want %v", res.RootHasApp, tc.wantRootApp)
			}
			if tc.wantTools != nil && strings.Join(sortedCopy(res.Tools), ",") != strings.Join(sortedCopy(tc.wantTools), ",") {
				t.Errorf("Tools = %v, want %v", res.Tools, tc.wantTools)
			}
			if tc.wantNone {
				if len(res.Suggestions) != 0 {
					t.Fatalf("want no suggestions, got %+v", res.Suggestions)
				}
				return
			}
			if len(res.Suggestions) == 0 {
				t.Fatal("no suggestions")
			}
			top := res.Suggestions[0]
			if top.BuildType != tc.wantTop.BuildType || top.DockerfilePath != tc.wantTop.DockerfilePath ||
				top.BaseDirectory != tc.wantTop.BaseDirectory || top.ReasonCode != tc.wantTop.ReasonCode {
				t.Errorf("top = %+v, want %+v", top, tc.wantTop)
			}
			if !top.Recommended || top.Reason == "" {
				t.Errorf("top should be recommended with a reason: %+v", top)
			}
			for _, d := range res.Dockerfiles {
				if strings.Contains(d, "node_modules") || strings.Contains(d, "vendor/") {
					t.Errorf("ignored dir leaked: %s", d)
				}
			}
		})
	}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestAnalyzeComposeFilesListed(t *testing.T) {
	files := map[string]string{"docker-compose.yml": "", "deploy/compose.prod.yaml": "", "Dockerfile": "FROM x"}
	res := Analyze(keys(files), readerFrom(files))
	if len(res.ComposeFiles) != 2 {
		t.Fatalf("compose files = %v", res.ComposeFiles)
	}
}

func TestGitHubSourceDetect(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/mono/git/ref/heads/main", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"object":{"sha":"abc123"}}`))
	})
	mux.HandleFunc("/repos/acme/mono/git/trees/abc123", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"truncated":false,"tree":[
			{"path":"turbo.json","type":"blob"},
			{"path":"apps","type":"tree"},
			{"path":"apps/glinr/deploy/Dockerfile","type":"blob"},
			{"path":"package.json","type":"blob"}]}`))
	})
	mux.HandleFunc("/repos/acme/mono/contents/apps/glinr/deploy/Dockerfile", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("FROM node\nRUN npx turbo prune glinr\n"))
	})
	mux.HandleFunc("/repos/acme/mono/contents/package.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"workspaces":["apps/*"]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	src := &githubSource{c: srv.Client(), api: srv.URL, repo: "acme/mono", ref: "main", token: "tok"}
	res, err := Detect(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Suggestions) != 1 || res.Suggestions[0].ReasonCode != ReasonTurboPrune {
		t.Fatalf("suggestions = %+v", res.Suggestions)
	}
	if res.Suggestions[0].BaseDirectory != "" {
		t.Fatalf("turbo prune must keep the root context, got %q", res.Suggestions[0].BaseDirectory)
	}
}

func TestNewAPISourceHosts(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://github.com/acme/mono.git", false},
		{"https://gitlab.com/acme/mono", false},
		{"https://codeberg.org/acme/mono", false},
		{"https://git.example.com/acme/mono", true},
		{"https://github.com/solo", true},
	}
	for _, tc := range cases {
		_, err := NewAPISource(http.DefaultClient, tc.url, "main", "", Hosts{})
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.url, err, tc.wantErr)
		}
	}
	if _, err := NewAPISource(http.DefaultClient, "https://git.example.com/acme/mono", "main", "", Hosts{GitLab: "git.example.com"}); err != nil {
		t.Errorf("self-hosted gitlab: %v", err)
	}
}
