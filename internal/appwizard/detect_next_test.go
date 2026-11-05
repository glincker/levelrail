package appwizard

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNextProfile(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  NextProfile
		hints []string
		noHit []string
	}{
		{"standalone with sharp", map[string]string{
			"package.json":    pkg(`"next":"^15.1.0","sharp":"0.33.0"`, ""),
			"next.config.mjs": "export default { output: 'standalone' }",
		}, NextProfile{Version: "15.1.0", Major: 15, Output: "standalone", Sharp: true}, nil, []string{HintNextSharp, HintNextNoOutput}},
		{"no output mode", map[string]string{
			"package.json": pkg(`"next":"14"`, ""), "next.config.js": "module.exports = {}",
		}, NextProfile{Version: "14", Major: 14, Output: "none"}, []string{HintNextNoOutput}, nil},
		{"old next standalone lacks sharp", map[string]string{
			"package.json": pkg(`"next":"~13.5.2"`, ""), "next.config.js": "module.exports = { output: \"standalone\" }",
		}, NextProfile{Version: "13.5.2", Major: 13, Output: "standalone"}, []string{HintNextSharp}, nil},
		{"commented output is ignored", map[string]string{
			"package.json": pkg(`"next":"15"`, ""), "next.config.js": "// output: 'standalone'\nmodule.exports = {}",
		}, NextProfile{Version: "15", Major: 15, Output: "none"}, []string{HintNextNoOutput}, nil},
		{"custom cache handler and memory", map[string]string{
			"package.json":    pkg(`"next":"15"`, ""),
			"next.config.mjs": "export default { output: 'standalone', cacheHandler: './cache.js', cacheMaxMemorySize: 0 }",
			"app/page.tsx":    "export const revalidate = 60",
		}, NextProfile{Version: "15", Major: 15, Output: "standalone", CacheHandler: true, UsesRevalidate: true}, nil, []string{HintNextMultiReplica}},
		{"isr without handler", map[string]string{
			"package.json": pkg(`"next":"15"`, ""), "next.config.js": "module.exports = { output: 'standalone' }",
			"app/page.tsx": "export const revalidate = 60", "app/a.ts": "revalidatePath('/x')",
		}, NextProfile{Version: "15", Major: 15, Output: "standalone", UsesRevalidate: true, MultiReplicaRisk: true}, []string{HintNextMultiReplica}, nil},
		{"cache components", map[string]string{
			"package.json": pkg(`"next":"16.0.1"`, ""), "next.config.ts": "const c = { experimental: { ppr: true }, cacheComponents: true, output: 'standalone' }; export default c",
		}, NextProfile{Version: "16.0.1", Major: 16, Output: "standalone", CacheComponents: true, Experimental: []string{"cacheComponents", "ppr"}, MultiReplicaRisk: true}, []string{HintNextMultiReplica}, nil},
		{"export with proxy", map[string]string{
			"package.json": pkg(`"next":"15"`, ""), "next.config.js": "module.exports = { output: 'export' }", "proxy.ts": "x",
		}, NextProfile{Version: "15", Major: 15, Output: "export", Middleware: "proxy.ts"}, []string{HintNextExportServer}, []string{HintNextNoOutput}},
		{"workspaces without tracing root", map[string]string{
			"package.json": pkg(`"next":"15"`, `,"workspaces":["packages/*"]`), "next.config.js": "module.exports = { output: 'standalone' }",
		}, NextProfile{Version: "15", Major: 15, Output: "standalone", Monorepo: true}, []string{HintNextTracingRoot}, nil},
		{"tracing root set", map[string]string{
			"package.json":   pkg(`"next":"15"`, `,"workspaces":["packages/*"]`),
			"next.config.js": "const path = require('path'); module.exports = { output: 'standalone', outputFileTracingRoot: path.join(__dirname, '../..') }",
		}, NextProfile{Version: "15", Major: 15, Output: "standalone", Monorepo: true, TracingRoot: true}, nil, []string{HintNextTracingRoot}},
		{"public vars counted once", map[string]string{
			"package.json": pkg(`"next":"15"`, ""), "next.config.js": "module.exports = { output: 'standalone' }",
			"app/a.tsx":    "process.env.NEXT_PUBLIC_API + process.env.NEXT_PUBLIC_API + process.env.NEXT_PUBLIC_KEY",
			".env.example": "NEXT_PUBLIC_SITE=1",
		}, NextProfile{Version: "15", Major: 15, Output: "standalone", PublicVars: 3}, []string{HintNextPublicVars}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Detect(tree(c.files))
			if d.Next == nil {
				t.Fatal("no next profile")
			}
			got, want := *d.Next, c.want
			if want.Experimental == nil {
				want.Experimental = []string{}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("profile\n got %+v\nwant %+v", got, want)
			}
			for _, h := range c.hints {
				if !hasWarn(d, h) {
					t.Errorf("missing hint %s", h)
				}
			}
			for _, h := range c.noHit {
				if hasWarn(d, h) {
					t.Errorf("unexpected hint %s", h)
				}
			}
		})
	}
}

func TestNextProfileAbsentForOtherStacks(t *testing.T) {
	if d := Detect(tree(map[string]string{"package.json": pkg(`"express":"4"`, "")})); d.Next != nil {
		t.Fatalf("express got a next profile: %+v", d.Next)
	}
}

func TestNextProfileExample(t *testing.T) {
	root := filepath.Join("testdata", "nextjs-standalone")
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p) //nolint:gosec // fixed testdata directory
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(b)
		return rerr
	})
	if err != nil {
		t.Fatal(err)
	}
	files["package-lock.json"] = ""
	d := Detect(tree(files))
	if d.Next == nil || d.Next.Output != "standalone" || d.Next.Major != 16 || d.Next.MultiReplicaRisk {
		t.Fatalf("example profile: %+v", d.Next)
	}
	if hasWarn(d, HintNextNoOutput) || hasWarn(d, HintNextMultiReplica) {
		t.Fatalf("example should be clean: %+v", d.Warnings)
	}
}

func TestNextStandaloneStartProposal(t *testing.T) {
	cases := []struct {
		name, cfg, want string
	}{
		{"standalone", "module.exports = { output: 'standalone' }", "node .next/standalone/server.js"},
		{"default output", "module.exports = {}", "next start"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Detect(tree(map[string]string{"package.json": pkg(`"next":"15"`, ""), "next.config.js": c.cfg, "package-lock.json": ""}))
			if d.StartCommand != c.want {
				t.Fatalf("start = %q, want %q", d.StartCommand, c.want)
			}
		})
	}
}
