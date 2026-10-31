package appwizard

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/repolayout"
)

func tree(files map[string]string) Input {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	return Input{Paths: paths, Read: func(p string) (string, bool) {
		s, ok := files[p]
		return s, ok
	}}
}

func hasWarn(d Detection, code string) bool {
	for _, w := range d.Warnings {
		if w.Code == code {
			return true
		}
	}
	return false
}

func hasReason(d Detection, sub string) bool {
	for _, r := range d.Reasons {
		if strings.Contains(r.Detail, sub) {
			return true
		}
	}
	return false
}

func pkg(deps string, extra string) string {
	return `{"name":"x","dependencies":{` + deps + `}` + extra + `}`
}

func TestDetectFrameworks(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		framework string
		buildType string
		port      int
		portSrc   string
		output    string
		version   string
		reason    string
	}{
		{"next app router standalone", map[string]string{
			"package.json": pkg(`"next":"14"`, `,"engines":{"node":">=20"}`), "package-lock.json": "{}",
			"next.config.mjs": "export default { output: 'standalone' }", "app/page.tsx": "x",
		}, "Next.js", "railpack", 3000, "default", "", ">=20", "App Router"},
		{"next static export", map[string]string{
			"package.json": pkg(`"next":"14"`, ""), "yarn.lock": "", "next.config.js": `module.exports = { output: "export" }`,
		}, "Next.js", "static", 80, "static", "out", "", "output: 'export'"},
		{"vite spa", map[string]string{"package.json": pkg(`"react":"18"`, `,"devDependencies":{"vite":"5"}`), "pnpm-lock.yaml": "", ".nvmrc": "v20.11.0\n"},
			"Vite", "static", 80, "static", "dist", "20.11.0", "single page app"},
		{"astro static", map[string]string{"package.json": pkg(`"astro":"4"`, ""), "package-lock.json": ""},
			"Astro", "static", 80, "static", "dist", "", "static site"},
		{"astro ssr", map[string]string{"package.json": pkg(`"astro":"4","@astrojs/node":"8"`, ""), "package-lock.json": ""},
			"Astro", "railpack", 4321, "default", "", "", "runs as a server"},
		{"sveltekit static", map[string]string{"package.json": pkg(`"@sveltejs/kit":"2","@sveltejs/adapter-static":"3"`, ""), "package-lock.json": ""},
			"SvelteKit", "static", 80, "static", "build", "", "prerendered"},
		{"nuxt", map[string]string{"package.json": pkg(`"nuxt":"3"`, ""), "package-lock.json": ""}, "Nuxt", "railpack", 3000, "default", "", "", "nuxt"},
		{"remix", map[string]string{"package.json": pkg(`"@remix-run/node":"2"`, ""), "package-lock.json": ""}, "Remix", "railpack", 3000, "default", "", "", "remix"},
		{"express", map[string]string{"package.json": pkg(`"express":"4"`, `,"scripts":{"start":"node index.js --port 4100"}`), "package-lock.json": ""},
			"Express", "railpack", 4100, "package.json start script", "", "", "express"},
		{"fastify", map[string]string{"package.json": pkg(`"fastify":"4"`, ""), "package-lock.json": ""}, "Fastify", "railpack", 3000, "convention", "", "", "fastify"},
		{"nestjs", map[string]string{"package.json": pkg(`"@nestjs/core":"10","express":"4"`, ""), "package-lock.json": ""}, "NestJS", "railpack", 3000, "default", "", "", "@nestjs/core"},
		{"django", map[string]string{"requirements.txt": "Django==5.0\n", "manage.py": "", "mysite/wsgi.py": "", ".python-version": "3.12\n"},
			"Django", "railpack", 8000, "convention", "", "3.12", "django"},
		{"fastapi", map[string]string{"requirements.txt": "fastapi\nuvicorn\n", "app/main.py": "", "pyproject.toml": "[project]\nrequires-python = \">=3.11\"\n"},
			"FastAPI", "railpack", 8000, "convention", "", ">=3.11", "fastapi"},
		{"flask", map[string]string{"requirements.txt": "flask==3\n", "app.py": ""}, "Flask", "railpack", 5000, "convention", "", "", "flask"},
		{"rails", map[string]string{"Gemfile": "ruby '3.3.0'\ngem 'rails'\n", "Gemfile.lock": "", "config/application.rb": "", "package.json": pkg(`"esbuild":"1"`, "")},
			"Rails", "railpack", 3000, "default", "", "3.3.0", "rails gem"},
		{"laravel", map[string]string{"composer.json": `{"require":{"php":"^8.2","laravel/framework":"^11"}}`, "composer.lock": "", "artisan": ""},
			"Laravel", "railpack", 80, "PHP", "", "^8.2", "laravel/framework"},
		{"go net/http", map[string]string{"go.mod": "module x\n\ngo 1.22.1\n\nrequire a/b v1.0.0\n", "go.sum": "", "main.go": ""}, "Go (net/http)", "railpack", 8080, "convention", "", "1.22.1", "go.mod"},
		{"rust axum", map[string]string{"Cargo.toml": "[dependencies]\naxum = \"0.7\"\n", "Cargo.lock": "", "rust-toolchain.toml": "[toolchain]\nchannel = \"1.80\"\n"},
			"Rust (axum)", "railpack", 3000, "axum", "", "1.80", "axum"},
		{"spring boot", map[string]string{"pom.xml": "<properties><java.version>21</java.version></properties><artifactId>spring-boot-starter-actuator</artifactId><parent>spring-boot-starter-parent</parent>",
			"src/main/resources/application.properties": "server.port=9090\n"},
			"Spring Boot", "railpack", 9090, "application.properties", "", "21", "actuator"},
		{"static html", map[string]string{"index.html": "<html></html>"}, "Static site", "static", 80, "static", ".", "", "index.html"},
		{"dockerfile first", map[string]string{"Dockerfile": "FROM node\nEXPOSE 8080\nHEALTHCHECK CMD curl http://localhost:8080/ready\n", "package.json": pkg(`"express":"4"`, ""), "package-lock.json": ""},
			"Express", "dockerfile", 8080, "EXPOSE", "", "", "Dockerfile is used"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Detect(tree(tc.files))
			if d.Framework != tc.framework || d.BuildType != tc.buildType {
				t.Fatalf("got %q/%q want %q/%q", d.Framework, d.BuildType, tc.framework, tc.buildType)
			}
			if d.Port.Value != tc.port || !strings.Contains(d.Port.Source, tc.portSrc) {
				t.Errorf("port %+v want %d from %q", d.Port, tc.port, tc.portSrc)
			}
			if d.OutputDir != tc.output {
				t.Errorf("output %q want %q", d.OutputDir, tc.output)
			}
			if d.RuntimeVersion.Value != tc.version {
				t.Errorf("version %+v want %q", d.RuntimeVersion, tc.version)
			}
			if !hasReason(d, tc.reason) {
				t.Errorf("no reason containing %q in %+v", tc.reason, d.Reasons)
			}
			if d.Confidence == "" {
				t.Error("empty confidence")
			}
		})
	}
}

func TestDetectHealthPaths(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"rails", map[string]string{"Gemfile": "gem 'rails'"}, "/up"},
		{"spring actuator", map[string]string{"pom.xml": "spring-boot actuator"}, "/actuator/health"},
		{"dockerfile healthcheck", map[string]string{"Dockerfile": "FROM x\nHEALTHCHECK CMD wget http://localhost/healthz\n"}, "/healthz"},
		{"default", map[string]string{"requirements.txt": "flask"}, "/"},
	}
	for _, tc := range cases {
		if got := Detect(tree(tc.files)).HealthPath; got != tc.want {
			t.Errorf("%s: health %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestDetectEdgeCases(t *testing.T) {
	huge := `{"name":"x","dependencies":{"express":"4"},"pad":"` + strings.Repeat("a", 2<<20) + `"}`
	cases := []struct {
		name       string
		in         Input
		framework  string
		buildType  string
		confidence string
		warn       string
		noWarn     string
	}{
		{"nothing recognisable", tree(map[string]string{"README.md": "hi", "notes.txt": "x"}), "", "railpack", ConfidenceLow, WarnNothingDetected, ""},
		{"empty tree", tree(nil), "", "railpack", ConfidenceLow, WarnNothingDetected, ""},
		{"malformed json falls back to raw text", tree(map[string]string{"package.json": `{"dependencies":{"next":"14"`, "package-lock.json": ""}), "Next.js", "railpack", ConfidenceMedium, WarnMalformedManifest, ""},
		{"huge package.json is truncated not fatal", tree(map[string]string{"package.json": huge, "package-lock.json": ""}), "Express", "railpack", ConfidenceMedium, WarnMalformedManifest, ""},
		{"rust warns that railpack does not build it", tree(map[string]string{"Cargo.toml": "[dependencies]\naxum = \"0.7\"\n", "Cargo.lock": ""}), "Rust (axum)", "railpack", ConfidenceHigh, WarnRailpackNoRust, ""},
		{"malformed toml still detects", tree(map[string]string{"Cargo.toml": "[dependencies\naxum ="}), "Rust (axum)", "railpack", ConfidenceHigh, WarnNoLockfile, ""},
		{"dockerfile without expose", tree(map[string]string{"Dockerfile": "FROM scratch\n"}), "", "dockerfile", ConfidenceHigh, WarnDockerfileNoPort, ""},
		{"multiple dockerfiles pick shallowest", tree(map[string]string{"Dockerfile": "EXPOSE 80", "api/Dockerfile": "EXPOSE 81", "web/Dockerfile.prod": "EXPOSE 82"}), "", "dockerfile", ConfidenceHigh, WarnMultiDockerfile, ""},
		{"no lockfile", tree(map[string]string{"package.json": pkg(`"express":"4"`, "")}), "Express", "railpack", ConfidenceMedium, WarnNoLockfile, ""},
		{"binary package.json ignored", tree(map[string]string{"package.json": "\x00\x01\x02", "index.html": "x"}), "Static site", "static", ConfidenceHigh, "", WarnMalformedManifest},
		{"compose only", tree(map[string]string{"docker-compose.yml": "services: {}"}), "", "compose", ConfidenceMedium, "", WarnNothingDetected},
		{"conflicting signals: rails beats node tooling", tree(map[string]string{"Gemfile": "gem 'rails'", "package.json": pkg(`"vite":"5"`, ""), "package-lock.json": ""}), "Rails", "railpack", ConfidenceHigh, "", ""},
		{"vendored dockerfile ignored", tree(map[string]string{"vendor/x/Dockerfile": "EXPOSE 1", "go.mod": "module x\ngo 1.22\n"}), "Go (net/http)", "railpack", ConfidenceMedium, "", WarnMultiDockerfile},
		{"dockerfile lifts a generic node guess", tree(map[string]string{"package.json": "{}", "package-lock.json": "", "Dockerfile": "EXPOSE 3000"}), "Node.js", "dockerfile", ConfidenceMedium, "", WarnNoLockfile},
		{"nil reader is safe", Input{Paths: []string{"package.json", "Dockerfile"}}, "", "dockerfile", ConfidenceHigh, WarnDockerfileNoPort, ""},
		{"symlink-loop style repeated paths", Input{Paths: []string{"a/a/a/a/package.json", "a/a/a/a/package.json", "a/a/a/a/a/a/a/index.html"}, Read: func(string) (string, bool) { return "", false }}, "", "railpack", ConfidenceLow, WarnNothingDetected, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Detect(tc.in)
			if d.Framework != tc.framework || d.BuildType != tc.buildType || d.Confidence != tc.confidence {
				t.Fatalf("got %q/%q/%q want %q/%q/%q", d.Framework, d.BuildType, d.Confidence, tc.framework, tc.buildType, tc.confidence)
			}
			if tc.warn != "" && !hasWarn(d, tc.warn) {
				t.Errorf("missing warning %s in %+v", tc.warn, d.Warnings)
			}
			if tc.noWarn != "" && hasWarn(d, tc.noWarn) {
				t.Errorf("unexpected warning %s", tc.noWarn)
			}
			if d.Reasons == nil || d.Warnings == nil || d.EnvVars == nil {
				t.Error("slices must be non-nil for JSON")
			}
		})
	}
}

func TestDetectMonorepoRootWithNoApp(t *testing.T) {
	files := map[string]string{
		"package.json": `{"workspaces":["apps/*"]}`, "pnpm-lock.yaml": "",
		"apps/web/package.json": pkg(`"next":"14"`, ""), "apps/api/package.json": pkg(`"fastify":"4"`, ""),
	}
	in := tree(files)
	layout := repolayout.Analyze(in.Paths, func(p string, _ int) ([]byte, error) {
		return []byte(files[p]), nil
	})
	in.Layout = &layout
	d := Detect(in)
	if !hasWarn(d, WarnMonorepoNoApp) {
		t.Fatalf("expected monorepo warning, got %+v", d.Warnings)
	}
	in.Dir = "apps/web"
	if got := Detect(in); got.Framework != "Next.js" || hasWarn(got, WarnMonorepoNoApp) {
		t.Fatalf("app dir detection: %q %+v", got.Framework, got.Warnings)
	}
}

func TestDetectEnvVarsNeverCarryValues(t *testing.T) {
	d := Detect(tree(map[string]string{
		"package.json": pkg(`"express":"4"`, ""), ".env.example": "DATABASE_URL=\nSTRIPE_SECRET_KEY=sk_live_abc\nPORT=3000\n",
	}))
	if len(d.EnvVars) == 0 {
		t.Fatal("no env vars")
	}
	secret := false
	for _, e := range d.EnvVars {
		if e.Name == "STRIPE_SECRET_KEY" {
			secret = e.SecretGuess
		}
	}
	if !secret {
		t.Errorf("STRIPE_SECRET_KEY should be guessed secret: %+v", d.EnvVars)
	}
}

func TestDetectStaticGeneratorsVersusServers(t *testing.T) {
	cases := []struct {
		name      string
		files     map[string]string
		framework string
		buildType string
		output    string
	}{
		{"astro plain", map[string]string{"package.json": pkg(`"astro":"4"`, "")}, "Astro", "static", "dist"},
		{"astro vercel adapter", map[string]string{"package.json": pkg(`"astro":"4","@astrojs/vercel":"7"`, "")}, "Astro", "railpack", ""},
		{"astro cloudflare adapter", map[string]string{"package.json": pkg(`"astro":"4","@astrojs/cloudflare":"9"`, "")}, "Astro", "railpack", ""},
		{"astro netlify adapter", map[string]string{"package.json": pkg(`"astro":"4","@astrojs/netlify":"5"`, "")}, "Astro", "railpack", ""},
		{"astro server output", map[string]string{"package.json": pkg(`"astro":"4"`, ""), "astro.config.mjs": "export default { output: 'server' }"}, "Astro", "railpack", ""},
		{"docusaurus", map[string]string{"package.json": pkg(`"@docusaurus/core":"3","react":"18"`, "")}, "Docusaurus", "static", "build"},
		{"eleventy", map[string]string{"package.json": pkg(`"@11ty/eleventy":"2"`, "")}, "Eleventy", "static", "_site"},
		{"vite spa with router", map[string]string{"package.json": pkg(`"react":"18","react-router-dom":"6"`, `,"devDependencies":{"vite":"5"},"scripts":{"build":"vite build"}`)}, "Vite", "static", "dist"},
		{"vite with koa", map[string]string{"package.json": pkg(`"koa":"2"`, `,"devDependencies":{"vite":"5"},"scripts":{"build":"vite build"}`)}, "Node.js", "railpack", ""},
		{"vite with node start", map[string]string{"package.json": pkg(`"react":"18"`, `,"devDependencies":{"vite":"5"},"scripts":{"start":"node server.js"}`)}, "Node.js", "railpack", ""},
		{"vite with express", map[string]string{"package.json": pkg(`"express":"4"`, `,"devDependencies":{"vite":"5"}`)}, "Express", "railpack", ""},
		{"sveltekit adapter static", map[string]string{"package.json": pkg(`"@sveltejs/kit":"2","@sveltejs/adapter-static":"3"`, ""), "svelte.config.js": "adapter({ pages: 'public' })"}, "SvelteKit", "static", "public"},
		{"sveltekit adapter node", map[string]string{"package.json": pkg(`"@sveltejs/kit":"2","@sveltejs/adapter-node":"5"`, "")}, "SvelteKit", "railpack", ""},
		{"next without export", map[string]string{"package.json": pkg(`"next":"14"`, ""), "next.config.js": "module.exports = {}"}, "Next.js", "railpack", ""},
		{"nuxt", map[string]string{"package.json": pkg(`"nuxt":"3"`, "")}, "Nuxt", "railpack", ""},
		{"remix", map[string]string{"package.json": pkg(`"@remix-run/node":"2"`, "")}, "Remix", "railpack", ""},
		{"unknown node", map[string]string{"package.json": pkg(`"left-pad":"1"`, "")}, "Node.js", "railpack", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := Detect(tree(tc.files))
			if d.Framework != tc.framework || d.BuildType != tc.buildType || d.OutputDir != tc.output {
				t.Fatalf("got %q/%q/%q want %q/%q/%q", d.Framework, d.BuildType, d.OutputDir, tc.framework, tc.buildType, tc.output)
			}
			if tc.buildType == "static" && d.StartCommand != "" {
				t.Errorf("static site has start command %q", d.StartCommand)
			}
		})
	}
	spa := Detect(tree(map[string]string{"package.json": pkg(`"vue-router":"4"`, `,"devDependencies":{"vite":"5"}`)}))
	if !hasReason(spa, "single page app mode") {
		t.Errorf("router project lacks SPA reason: %+v", spa.Reasons)
	}
	if !spa.SPA {
		t.Error("client routed Vite app should set SPA")
	}
	cra := Detect(tree(map[string]string{"package.json": pkg(`"react-router-dom":"6","react-scripts":"5"`, "")}))
	if !cra.SPA {
		t.Error("client routed CRA app should set SPA")
	}
	noRouter := Detect(tree(map[string]string{"package.json": pkg(`"react":"18"`, `,"devDependencies":{"vite":"5"}`)}))
	if noRouter.SPA {
		t.Error("Vite app without a router must not set SPA")
	}
	plain := Detect(tree(map[string]string{"package.json": pkg(`"astro":"4"`, "")}))
	if plain.SPA {
		t.Error("static generator must not set SPA")
	}
	if hasReason(plain, "single page app mode") {
		t.Error("plain static generator got SPA reason")
	}
}
