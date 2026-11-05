package appwizard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const updateGoldenEnv = "APP_WIZARD_UPDATE_GOLDEN"

var planNext = PlanNext{Method: "POST", Path: "/api/v1/imports/repo/apply"}

var planFixtures = map[string]map[string]string{
	"node": {
		"package.json":      `{"name":"api","scripts":{"start":"node server.js"},"dependencies":{"express":"4.19.0"}}`,
		"package-lock.json": "{}",
		"server.js":         "require('express')().listen(3000)",
		".env.example":      "SESSION_SECRET=\nLOG_LEVEL=info\n",
	},
	"flask": {
		"requirements.txt": "flask==3.0.0\ngunicorn==21.2.0\n",
		"app.py":           "from flask import Flask\napp = Flask(__name__)\n",
		".env.sample":      "SECRET_KEY=changeme\nDATABASE_URL=\n",
	},
	"go": {
		"go.mod":  "module example.com/svc\n\ngo 1.22\n",
		"main.go": "package main\nfunc main() {}\n",
	},
	"static": {
		"index.html": "<html></html>",
	},
}

func loadDir(t *testing.T, dir string) DirSource {
	t.Helper()
	return DirSource{Root: dir}
}

func planFor(t *testing.T, src interface {
	Paths(context.Context) ([]string, bool, error)
	Read(context.Context, string, int) ([]byte, error)
}, repo string) PlanView {
	t.Helper()
	a, err := Analyze(context.Background(), src, testOpts(repo))
	if err != nil {
		t.Fatal(err)
	}
	return BuildPlanView(a, planNext)
}

func checkGolden(t *testing.T, name string, pv PlanView) {
	t.Helper()
	got, err := json.MarshalIndent(pv, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "plan", name+".golden.json")
	if os.Getenv(updateGoldenEnv) != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("missing golden %s (set %s=1 to create): %v", path, updateGoldenEnv, err)
	}
	if string(want) != string(got) {
		t.Errorf("plan for %s differs from %s:\n%s", name, path, got)
	}
}

func TestPlanViewGolden(t *testing.T) {
	for name, files := range planFixtures {
		t.Run(name, func(t *testing.T) {
			pv := planFor(t, &fakeSource{files: files}, name)
			checkGolden(t, name, pv)
		})
	}
	t.Run("nextjs", func(t *testing.T) {
		checkGolden(t, "nextjs", planFor(t, loadDir(t, filepath.Join("testdata", "nextjs-standalone")), "nextjs"))
	})
}

func TestPlanViewNeverReturnsValues(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("package.json", `{"name":"x","dependencies":{"express":"4"}}`)
	write(".env.example", "API_TOKEN=\"abc=def==\"\nexport DB_URL='postgres://u:hunter2@db/x?a=b'\nPLAIN=visible-value\nEMPTY=\n")
	write(".env", "REAL_SECRET=topsecretvalue\n")
	write(".env.production", "PROD_SECRET=prodsecretvalue\n")
	pv := planFor(t, loadDir(t, root), "x")
	raw, err := json.Marshal(pv)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"abc=def", "hunter2", "visible-value", "topsecretvalue", "prodsecretvalue", "REAL_SECRET", "PROD_SECRET"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("plan leaks %q: %s", leak, raw)
		}
	}
	names := map[string]PlanEnv{}
	for _, e := range pv.Env {
		names[e.Name] = e
	}
	if !names["API_TOKEN"].LooksSecret || !names["PLAIN"].HasDefault || names["EMPTY"].HasDefault {
		t.Errorf("env flags wrong: %+v", names)
	}
}

func TestPlanViewWarnings(t *testing.T) {
	pv := planFor(t, &fakeSource{files: map[string]string{
		"package.json":   `{"name":"w","dependencies":{"next":"15.0.0"}}`,
		"next.config.js": "module.exports = { output: 'standalone' }",
		"app/page.tsx":   "export const revalidate = 60",
	}}, "w")
	codes := map[string]bool{}
	for _, w := range pv.Warnings {
		codes[w.Code] = true
	}
	for _, want := range []string{HintNextMultiReplica, WarnNoDockerfile} {
		if !codes[want] {
			t.Errorf("missing warning %s in %v", want, pv.Warnings)
		}
	}
	mono := planFor(t, &fakeSource{files: map[string]string{
		"pnpm-workspace.yaml":   "packages:\n  - 'apps/*'\n",
		"apps/web/package.json": `{"name":"web","dependencies":{"next":"14"}}`,
		"apps/api/package.json": `{"name":"api","dependencies":{"express":"4"}}`,
	}}, "mono")
	found := false
	for _, w := range mono.Warnings {
		found = found || w.Code == WarnMonorepo
	}
	if !found {
		t.Errorf("monorepo warning missing: %v", mono.Warnings)
	}
}

func TestDirSourceStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "x.txt"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks unavailable")
	}
	src := DirSource{Root: root}
	paths, _, err := src.Paths(context.Background())
	if err != nil || len(paths) != 0 {
		t.Fatalf("paths = %v err = %v", paths, err)
	}
	for _, p := range []string{"../x.txt", "link/x.txt", ".env"} {
		if b, err := src.Read(context.Background(), p, 100); err == nil {
			t.Errorf("read %q returned %q", p, b)
		}
	}
}
