package appwizard

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

type fakeSource struct {
	files map[string]string
	reads int
}

func (f *fakeSource) Paths(context.Context) ([]string, bool, error) {
	out := make([]string, 0, len(f.files))
	for p := range f.files {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, false, nil
}

func (f *fakeSource) Read(_ context.Context, p string, limit int) ([]byte, error) {
	f.reads++
	s, ok := f.files[p]
	if !ok {
		return nil, errors.New("not found")
	}
	if len(s) > limit {
		s = s[:limit]
	}
	return []byte(s), nil
}

func testOpts(repo string) Options {
	return Options{
		RepoURL: "https://github.com/acme/" + repo, Branch: "main",
		AppsBaseDomain: "apps.example.com",
		DomainFor:      func(name, base string) string { return name + "." + base },
		DefaultVersion: func(e string) string {
			return map[string]string{"postgres": "16", "redis": "7", "mysql": "8", "mongodb": "7"}[e]
		},
	}
}

func TestAnalyzeFixtures(t *testing.T) {
	tests := []struct {
		name       string
		repo       string
		files      map[string]string
		wantRT     string
		wantFW     string
		wantBuild  string
		wantDir    string
		wantDF     string
		wantPort   int
		wantHealth string
		wantDBs    []string
		wantEnv    []string
	}{
		{
			name: "next with env example and pg", repo: "shop",
			files: map[string]string{
				"package.json": `{"scripts":{"start":"next start"},"dependencies":{"next":"14","pg":"8"}}`,
				".env.example": "DATABASE_URL=\nNEXTAUTH_SECRET=changeme\nNEXT_PUBLIC_URL=http://localhost:3000\n",
			},
			wantRT: "node", wantFW: "Next.js", wantBuild: "railpack", wantPort: 3000, wantHealth: "/",
			wantDBs: []string{"postgres"}, wantEnv: []string{"DATABASE_URL", "NEXTAUTH_SECRET", "NEXT_PUBLIC_URL"},
		},
		{
			name: "vite static", repo: "site",
			files: map[string]string{
				"package.json": `{"devDependencies":{"vite":"5"}}`,
			},
			wantRT: "node", wantFW: "Static frontend", wantBuild: "railpack", wantPort: 3000, wantHealth: "/",
		},
		{
			name: "express with dockerfile expose and healthcheck", repo: "api",
			files: map[string]string{
				"package.json": `{"dependencies":{"express":"4","ioredis":"5"}}`,
				"Dockerfile":   "FROM node:20\nEXPOSE 4000\nHEALTHCHECK CMD curl -f http://localhost:4000/healthz || exit 1\n",
			},
			wantRT: "node", wantFW: "Node.js server", wantBuild: "dockerfile", wantDF: "Dockerfile", wantPort: 4000, wantHealth: "/healthz",
			wantDBs: []string{"redis"},
		},
		{
			name: "django", repo: "blog",
			files: map[string]string{
				"requirements.txt": "Django==5.0\npsycopg2-binary\n",
				".env.sample":      "SECRET_KEY=\nDEBUG=false\n",
			},
			wantRT: "python", wantFW: "Django", wantBuild: "railpack", wantPort: 8000, wantHealth: "/",
			wantDBs: []string{"postgres"}, wantEnv: []string{"SECRET_KEY", "DEBUG"},
		},
		{
			name: "rails", repo: "forum",
			files: map[string]string{
				"Gemfile": "gem 'rails'\ngem 'pg'\n",
			},
			wantRT: "ruby", wantFW: "Rails", wantBuild: "railpack", wantPort: 3000, wantHealth: "/up",
			wantDBs: []string{"postgres"},
		},
		{
			name: "go", repo: "svc",
			files: map[string]string{
				"go.mod": "module x\n\ngo 1.22\n",
			},
			wantRT: "go", wantFW: "Go", wantBuild: "railpack", wantPort: 8080, wantHealth: "/",
		},
		{
			name: "php laravel", repo: "cms",
			files: map[string]string{
				"composer.json": `{"require":{"laravel/framework":"^11"}}`,
			},
			wantRT: "php", wantFW: "Laravel", wantBuild: "railpack", wantPort: 80, wantHealth: "/",
		},
		{
			name: "plain static", repo: "docs",
			files: map[string]string{
				"index.html": "<html></html>",
			},
			wantRT: "static", wantFW: "Static site", wantBuild: "static", wantPort: 80, wantHealth: "/",
		},
		{
			name: "turborepo with app dockerfile", repo: "mono",
			files: map[string]string{
				"turbo.json":               `{}`,
				"package.json":             `{"workspaces":["apps/*"]}`,
				"apps/web/package.json":    `{"scripts":{"start":"next start -p 4100"},"dependencies":{"next":"14"}}`,
				"apps/web/Dockerfile":      "FROM node:20\nCOPY . .\nRUN turbo prune web\nEXPOSE 3000\n",
				"apps/web/.env.example":    "API_URL=\n",
				"apps/docs/package.json":   `{"dependencies":{"next":"14"}}`,
				"packages/ui/package.json": `{}`,
			},
			wantRT: "node", wantFW: "Next.js", wantBuild: "dockerfile", wantDF: "apps/web/Dockerfile", wantDir: "apps/web", wantPort: 3000, wantHealth: "/",
			wantEnv: []string{"API_URL"},
		},
		{
			name: "compose with postgres and redis", repo: "stack",
			files: map[string]string{
				"package.json":       `{"dependencies":{"express":"4"}}`,
				"docker-compose.yml": "services:\n  app:\n    build: .\n    environment:\n      - JWT_SECRET=\n      - DATABASE_URL=${DATABASE_URL}\n  db:\n    image: postgres:15-alpine\n    environment:\n      POSTGRES_PASSWORD: secret\n  cache:\n    image: redis:7\n",
			},
			wantRT: "node", wantFW: "Node.js server", wantBuild: "railpack", wantPort: 3000, wantHealth: "/",
			wantDBs: []string{"postgres", "redis"}, wantEnv: []string{"JWT_SECRET", "DATABASE_URL"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, err := Analyze(context.Background(), &fakeSource{files: tc.files}, testOpts(tc.repo))
			if err != nil {
				t.Fatal(err)
			}
			if a.Stack.Runtime != tc.wantRT || a.Stack.Framework != tc.wantFW {
				t.Errorf("stack = %s/%s, want %s/%s", a.Stack.Runtime, a.Stack.Framework, tc.wantRT, tc.wantFW)
			}
			if a.Build.Type != tc.wantBuild || a.Build.DockerfilePath != tc.wantDF || a.AppDirectory != tc.wantDir {
				t.Errorf("build = %+v dir=%q, want %s %q dir=%q", a.Build, a.AppDirectory, tc.wantBuild, tc.wantDF, tc.wantDir)
			}
			if a.Port.Value != tc.wantPort || a.Port.Evidence == "" {
				t.Errorf("port = %+v, want %d with evidence", a.Port, tc.wantPort)
			}
			if a.Health.Value != tc.wantHealth {
				t.Errorf("health = %q, want %q", a.Health.Value, tc.wantHealth)
			}
			var dbs []string
			for _, d := range a.Databases {
				dbs = append(dbs, d.Engine)
				if d.Evidence == "" || d.Version == "" || d.SuggestedName == "" {
					t.Errorf("database need missing detail: %+v", d)
				}
			}
			if strings.Join(dbs, ",") != strings.Join(tc.wantDBs, ",") {
				t.Errorf("databases = %v, want %v", dbs, tc.wantDBs)
			}
			have := map[string]bool{}
			for _, e := range a.Env {
				have[e.Key] = true
			}
			for _, k := range tc.wantEnv {
				if !have[k] {
					t.Errorf("env key %s missing from %+v", k, a.Env)
				}
			}
			if a.Memory.Bytes == 0 || a.Name.Value == "" {
				t.Errorf("missing memory or name: %+v %+v", a.Memory, a.Name)
			}
			if !strings.HasSuffix(a.Domain.Value, ".apps.example.com") {
				t.Errorf("domain = %q", a.Domain.Value)
			}
		})
	}
}

func TestAnalyzeNeverReturnsEnvValuesAndProvidesDatabaseKeys(t *testing.T) {
	//nolint:gosec // fixture value used to prove env values are never returned
	src := &fakeSource{files: map[string]string{
		"package.json": `{"dependencies":{"next":"14"}}`,
		".env.example": "DATABASE_URL=postgres://u:hunter2@db/app\nAPI_TOKEN=sk_live_abc\nMODE=production\n",
	}}
	a, err := Analyze(context.Background(), src, testOpts("x"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range a.Env {
		if strings.Contains(e.Key, "hunter2") || strings.Contains(e.Source, "sk_live") {
			t.Fatalf("value leaked: %+v", e)
		}
		if e.Key == "DATABASE_URL" && e.ProvidedBy == "" {
			t.Errorf("DATABASE_URL should be provided by the suggested database: %+v", e)
		}
		if e.Key == "API_TOKEN" && !e.Secret {
			t.Errorf("API_TOKEN should be marked secret")
		}
	}
}

func TestAnalyzeReadBudget(t *testing.T) {
	t.Setenv(EnvMaxFiles, "1")
	src := &fakeSource{files: map[string]string{
		"package.json": `{"dependencies":{"next":"14"}}`,
		".env.example": "A=\n",
		"README.md":    "# x\n",
	}}
	a, err := Analyze(context.Background(), src, testOpts("x"))
	if err != nil {
		t.Fatal(err)
	}
	if src.reads > 1 {
		t.Fatalf("reads = %d, want at most 1", src.reads)
	}
	found := false
	for _, w := range a.Warnings {
		found = found || w.Code == WarnReadBudget
	}
	if !found {
		t.Errorf("expected a read budget warning, got %+v", a.Warnings)
	}
}

func TestAnalyzeOverridesAndMemoryEnv(t *testing.T) {
	t.Setenv("APP_WIZARD_MEMORY_MB_GO", "64")
	src := &fakeSource{files: map[string]string{"svc/go.mod": "module x\n", "svc/Dockerfile": "FROM golang\nEXPOSE 9000\n"}}
	o := testOpts("repo")
	o.BuildType, o.BaseDirectory, o.DockerfilePath, o.Name = "dockerfile", "svc", "svc/Dockerfile", "My App!"
	a, err := Analyze(context.Background(), src, o)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name.Value != "my-app" || a.Port.Value != 9000 || a.Memory.Bytes != 64*mib {
		t.Errorf("got name=%q port=%d mem=%d", a.Name.Value, a.Port.Value, a.Memory.Bytes)
	}
}

func TestSlugAndRepoName(t *testing.T) {
	for in, want := range map[string]string{"Hello World": "hello-world", "--x--": "x", "": "app", "A_B.c": "a-b-c"} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := repoName("https://github.com/acme/thing.git"); got != "thing" {
		t.Errorf("repoName = %q", got)
	}
}

func TestAnalyzeEmitsNoNullSlices(t *testing.T) {
	src := &fakeSource{files: map[string]string{"README.md": "hello"}}
	a, err := Analyze(context.Background(), src, testOpts("bare"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"env", "databases", "warnings", "packages", "alternatives", "env_vars", "reasons"} {
		if strings.Contains(string(raw), `"`+field+`":null`) {
			t.Errorf("field %q marshalled as null: %s", field, raw)
		}
	}
}
