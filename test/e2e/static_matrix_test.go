// TestStaticMatrix_DetectsAndValidates is the offline half of the static
// framework matrix: each starter layout goes through the real detector and the
// real build validator, so a detector change that stops recognising a
// framework fails here instead of on a user's first deploy. The live half
// (building and probing on a host) is run by hand from the same table.
package e2e

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/appwizard"
	"github.com/GLINCKER/levelrail/internal/spec"
)

type staticMatrixEntry struct {
	name      string
	files     map[string]string
	framework string
	output    string
	spa       bool
}

func matrixPkg(deps, extra string) string {
	return `{"name":"starter","dependencies":{` + deps + `}` + extra + `}`
}

func staticMatrix() []staticMatrixEntry {
	vite := `,"devDependencies":{"vite":"5"},"scripts":{"build":"vite build"}`
	return []staticMatrixEntry{
		{"astro", map[string]string{"package.json": matrixPkg(`"astro":"4"`, ""), "package-lock.json": "{}"}, "Astro", "dist", false},
		{"vite react", map[string]string{"package.json": matrixPkg(`"react":"18"`, vite), "package-lock.json": "{}"}, "Vite", "dist", false},
		{"vite react router", map[string]string{"package.json": matrixPkg(`"react":"18","react-router-dom":"6"`, vite), "pnpm-lock.yaml": ""}, "Vite", "dist", true},
		{"vite vue", map[string]string{"package.json": matrixPkg(`"vue":"3","vue-router":"4"`, vite), "yarn.lock": ""}, "Vite", "dist", true},
		{"create react app", map[string]string{"package.json": matrixPkg(`"react":"18","react-scripts":"5"`, ""), "package-lock.json": "{}"}, "Create React App", "build", false},
		{"sveltekit static", map[string]string{"package.json": matrixPkg(`"@sveltejs/kit":"2","@sveltejs/adapter-static":"3"`, ""), "package-lock.json": "{}"}, "SvelteKit", "build", false},
		{"next export", map[string]string{"package.json": matrixPkg(`"next":"14"`, ""), "next.config.js": `module.exports = { output: "export" }`, "package-lock.json": "{}"}, "Next.js", "out", false},
		{"docusaurus", map[string]string{"package.json": matrixPkg(`"@docusaurus/core":"3","react":"18"`, ""), "package-lock.json": "{}"}, "Docusaurus", "build", false},
		{"eleventy", map[string]string{"package.json": matrixPkg(`"@11ty/eleventy":"2"`, ""), "package-lock.json": "{}"}, "Eleventy", "_site", false},
		{"plain html", map[string]string{"index.html": "<html></html>"}, "Static site", ".", false},
	}
}

func TestStaticMatrix_DetectsAndValidates(t *testing.T) {
	for _, e := range staticMatrix() {
		t.Run(e.name, func(t *testing.T) {
			paths := make([]string, 0, len(e.files))
			for p := range e.files {
				paths = append(paths, p)
			}
			d := appwizard.Detect(appwizard.Input{Paths: paths, Read: func(p string) (string, bool) {
				s, ok := e.files[p]
				return s, ok
			}})
			if d.Framework != e.framework || d.BuildType != "static" || d.OutputDir != e.output {
				t.Fatalf("detected %q/%q/%q, want %q/static/%q", d.Framework, d.BuildType, d.OutputDir, e.framework, e.output)
			}
			if d.StartCommand != "" {
				t.Errorf("a static site must not carry a start command, got %q", d.StartCommand)
			}
			if d.SPA != e.spa {
				t.Errorf("spa = %v, want %v", d.SPA, e.spa)
			}
			if d.BuildCommand != "" && d.OutputDir != "." {
				if err := spec.ValidateStaticBuild("site", spec.BuildStatic, d.BuildCommand, d.OutputDir, "", nil); err != nil {
					t.Errorf("detected plan fails the build validator: %v", err)
				}
			}
		})
	}
}
