package repolayout

import (
	"reflect"
	"testing"
)

func TestFindPackages(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		dirs  []string
		tool  string
	}{
		{
			name: "pnpm workspace globs",
			files: map[string]string{
				"pnpm-workspace.yaml":        "packages:\n  - 'services/*'\n  - \"!services/skip\"\n",
				"services/api/package.json":  `{"name":"api"}`,
				"services/web/package.json":  `{"name":"web"}`,
				"services/skip/package.json": `{"name":"skip"}`,
				"docs/package.json":          `{"name":"docs"}`,
			},
			dirs: []string{"services/api", "services/web"},
			tool: "pnpm-workspace",
		},
		{
			name: "npm workspaces array",
			files: map[string]string{
				"package.json":        `{"workspaces":["apps/*"]}`,
				"apps/a/package.json": `{"name":"a"}`,
			},
			dirs: []string{"apps/a"},
			tool: "npm-workspaces",
		},
		{
			name: "yarn workspaces object form",
			files: map[string]string{
				"package.json":        `{"workspaces":{"packages":["apps/*"]}}`,
				"apps/a/package.json": `{"name":"a"}`,
			},
			dirs: []string{"apps/a"},
			tool: "npm-workspaces",
		},
		{
			name: "go work block",
			files: map[string]string{
				"go.work":      "go 1.22\nuse (\n\t./svc/a\n\t./svc/b // comment\n)\n",
				"svc/a/go.mod": "module a",
				"svc/b/go.mod": "module b",
				"svc/c/go.mod": "module c",
			},
			dirs: []string{"svc/a", "svc/b"},
			tool: "go-work",
		},
		{
			name:  "go work single use",
			files: map[string]string{"go.work": "use ./only\n", "only/go.mod": "module o"},
			dirs:  []string{"only"},
			tool:  "go-work",
		},
		{name: "no workspace", files: map[string]string{"package.json": `{"name":"x"}`}, dirs: []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			present := map[string]bool{}
			for k := range tc.files {
				present[k] = true
			}
			pkgs := findPackages(keys(tc.files), present, readerFrom(tc.files))
			got := []string{}
			for _, p := range pkgs {
				got = append(got, p.Dir)
				if p.Tool != tc.tool {
					t.Errorf("tool = %q, want %q", p.Tool, tc.tool)
				}
			}
			if !reflect.DeepEqual(got, tc.dirs) {
				t.Fatalf("dirs = %v, want %v", got, tc.dirs)
			}
		})
	}
}

func TestPackageWatchPathsIncludeDepsAndShared(t *testing.T) {
	files := map[string]string{
		"pnpm-workspace.yaml":      "packages:\n  - 'apps/*'\n  - 'packages/*'\n",
		"pnpm-lock.yaml":           "",
		"apps/web/package.json":    `{"name":"web","dependencies":{"ui":"workspace:*","react":"1"}}`,
		"apps/web/Dockerfile":      "FROM node",
		"packages/ui/package.json": `{"name":"ui"}`,
	}
	present := map[string]bool{}
	for k := range files {
		present[k] = true
	}
	var web Package
	for _, p := range findPackages(keys(files), present, readerFrom(files)) {
		if p.Dir == "apps/web" {
			web = p
		}
	}
	want := []string{"apps/web/**", "packages/ui/**", "pnpm-lock.yaml", "pnpm-workspace.yaml"}
	if !reflect.DeepEqual(web.WatchPaths, want) {
		t.Fatalf("watch paths = %v, want %v", web.WatchPaths, want)
	}
	if web.Dockerfile != "apps/web/Dockerfile" || !reflect.DeepEqual(web.DependsOn, []string{"packages/ui"}) {
		t.Fatalf("dockerfile/deps = %q %v", web.Dockerfile, web.DependsOn)
	}
}

func TestAffected(t *testing.T) {
	pkgs := []Package{
		{Dir: "apps/api", WatchPaths: []string{"apps/api/**", "packages/lib/**", "pnpm-lock.yaml"}},
		{Dir: "apps/web", WatchPaths: []string{"apps/web/**", "pnpm-lock.yaml"}},
		{Dir: "apps/docs", WatchPaths: []string{"apps/docs/**/*.{md,mdx}", "pnpm-lock.yaml"}},
	}
	cases := []struct {
		name    string
		changed []string
		want    []string
	}{
		{"empty diff", nil, []string{}},
		{"one service", []string{"apps/web/src/a.ts"}, []string{"apps/web"}},
		{"shared dependency", []string{"packages/lib/index.ts"}, []string{"apps/api"}},
		{"root lockfile hits all", []string{"pnpm-lock.yaml"}, []string{"apps/api", "apps/docs", "apps/web"}},
		{"rename across services", []string{"apps/web/a.ts", "apps/api/a.ts"}, []string{"apps/api", "apps/web"}},
		{"glob extension match", []string{"apps/docs/guide/intro.mdx"}, []string{"apps/docs"}},
		{"glob extension miss", []string{"apps/docs/logo.png"}, []string{}},
		{"unrelated root file", []string{"README.md"}, []string{}},
		{"sibling prefix is not a match", []string{"apps/web-admin/x.ts"}, []string{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Affected(tc.changed, pkgs); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPackageCapFromEnv(t *testing.T) {
	files := map[string]string{
		"pnpm-workspace.yaml": "packages:\n  - 'apps/*'\n",
		"apps/a/package.json": "{}", "apps/b/package.json": "{}", "apps/c/package.json": "{}",
	}
	present := map[string]bool{}
	for k := range files {
		present[k] = true
	}
	t.Setenv(EnvMaxPackages, "2")
	pkgs, total := findPackagesTotal(keys(files), present, readerFrom(files))
	if len(pkgs) != 2 || total != 3 {
		t.Fatalf("got %d packages, total %d", len(pkgs), total)
	}
	t.Setenv(EnvMaxPackages, "bogus")
	if PackageCap() != 60 {
		t.Fatalf("bad value should fall back to the default, got %d", PackageCap())
	}
}
