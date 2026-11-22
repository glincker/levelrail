package frameworks

import (
	"encoding/json"
	"strings"
)

// Reader returns the text of a file relative to the app root.
type Reader func(rel string) (string, bool)

// Match is a detected preset plus the file that decided it.
type Match struct {
	Preset   Preset `json:"preset"`
	Evidence string `json:"evidence"`
}

// Detect picks the best preset from manifest files, or false when nothing
// recognisable is present.
func Detect(read Reader) (Match, bool) {
	if m, ok := detectDeno(read); ok {
		return m, true
	}
	if text, ok := read("package.json"); ok {
		m, ok := detectNode(text, read)
		if ok && m.Preset.ID != "node" {
			return m, true
		}
		if bm, bok := detectBun(read); bok {
			return bm, true
		}
		if ok {
			return m, true
		}
	}
	if m, ok := detectDotnet(read); ok {
		return m, true
	}
	if _, ok := read("go.mod"); ok {
		return match("go", "go.mod found"), true
	}
	if _, ok := read("Cargo.toml"); ok {
		return match("rust", "Cargo.toml found"), true
	}
	for _, f := range []string{"hugo.toml", "hugo.yaml", "hugo.json"} {
		if _, ok := read(f); ok {
			return match("hugo", f+" found"), true
		}
	}
	if gem, ok := read("Gemfile"); ok && strings.Contains(strings.ToLower(gem), "jekyll") {
		if _, ok := read("_config.yml"); ok {
			return match("jekyll", "Gemfile lists jekyll and _config.yml exists"), true
		}
	}
	if _, ok := read("mkdocs.yml"); ok {
		return match("mkdocs", "mkdocs.yml found"), true
	}
	if text, ok := read("config.toml"); ok && strings.Contains(text, "base_url") {
		return match("zola", "config.toml sets base_url"), true
	}
	if text, ok := read("mix.exs"); ok && strings.Contains(text, ":phoenix,") {
		return match("phoenix", "mix.exs lists phoenix"), true
	}
	for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		text, ok := read(f)
		if !ok {
			continue
		}
		low := strings.ToLower(text)
		switch {
		case strings.Contains(low, "quarkus-maven-plugin") || strings.Contains(low, "io.quarkus"):
			return match("quarkus", f+" lists quarkus"), true
		case strings.Contains(low, "micronaut"):
			return match("micronaut", f+" lists micronaut"), true
		case strings.Contains(low, "spring-boot"):
			return match("spring-boot", f+" lists spring-boot"), true
		}
	}
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile"} {
		text, ok := read(f)
		if !ok {
			continue
		}
		low := strings.ToLower(text)
		if strings.Contains(low, "django") {
			return match("django", f+" lists django"), true
		}
		if strings.Contains(low, "fastapi") {
			return match("fastapi", f+" lists fastapi"), true
		}
		if strings.Contains(low, "flask") {
			return match("flask", f+" lists flask"), true
		}
	}
	if text, ok := read("Gemfile"); ok && strings.Contains(strings.ToLower(text), "rails") {
		return match("rails", "Gemfile lists rails"), true
	}
	if text, ok := read("composer.json"); ok && strings.Contains(text, "laravel/framework") {
		return match("laravel", "composer.json lists laravel/framework"), true
	}
	return Match{}, false
}

func match(id, evidence string) Match {
	p, _ := Get(id)
	return Match{Preset: p, Evidence: evidence}
}

func detectNode(text string, read Reader) (Match, bool) {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(text), &pkg); err != nil {
		return Match{}, false
	}
	has := func(name string) bool {
		_, a := pkg.Dependencies[name]
		_, b := pkg.DevDependencies[name]
		return a || b
	}
	switch {
	case has("next"):
		if nextIsStaticExport(read) {
			return match("nextjs-static", "next.config sets output: export"), true
		}
		return match("nextjs", "package.json lists next"), true
	case has("@remix-run/node") || has("@remix-run/react") || has("@remix-run/dev") || has("@remix-run/serve"):
		return match("remix", "package.json lists @remix-run packages"), true
	case has("nuxt"):
		return match("nuxt", "package.json lists nuxt"), true
	case has("@sveltejs/kit"):
		return match("sveltekit", "package.json lists @sveltejs/kit"), true
	case has("@docusaurus/core"):
		return match("docusaurus", "package.json lists @docusaurus/core"), true
	case has("@angular/core") || has("@angular/cli"):
		return angularMatch(read), true
	case has("@vue/cli-service"):
		return match("vue-cli", "package.json lists @vue/cli-service"), true
	case has("@solidjs/start"):
		return match("solidstart", "package.json lists @solidjs/start"), true
	case has("@builder.io/qwik") || has("@builder.io/qwik-city"):
		return match("qwik", "package.json lists @builder.io/qwik"), true
	case has("vitepress"):
		return match("vitepress", "package.json lists vitepress"), true
	case has("gatsby"):
		return match("gatsby", "package.json lists gatsby"), true
	case has("astro") && has("@astrojs/node"):
		return match("astro-ssr", "package.json lists astro and @astrojs/node"), true
	case has("@11ty/eleventy"):
		return match("eleventy", "package.json lists @11ty/eleventy"), true
	case has("@nestjs/core"):
		return match("nestjs", "package.json lists @nestjs/core"), true
	case has("astro"):
		return match("astro", "package.json lists astro"), true
	case has("vite"):
		return match("vite", "package.json lists vite"), true
	}
	for _, name := range []string{"express", "fastify", "koa", "hono"} {
		if has(name) {
			return match("node", "package.json lists "+name), true
		}
	}
	return Match{}, false
}

func nextIsStaticExport(read Reader) bool {
	for _, f := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"} {
		text, ok := read(f)
		if !ok {
			continue
		}
		compact := strings.NewReplacer(" ", "", "\"", "'", "`", "'").Replace(text)
		return strings.Contains(compact, "output:'export'")
	}
	return false
}

// angularMatch points the publish dir at dist/<project>/browser when
// angular.json names exactly one project, since the output path embeds it.
func angularMatch(read Reader) Match {
	m := match("angular", "package.json lists @angular/core")
	text, ok := read("angular.json")
	if !ok {
		return m
	}
	var cfg struct {
		Projects map[string]json.RawMessage `json:"projects"`
	}
	if json.Unmarshal([]byte(text), &cfg) != nil || len(cfg.Projects) != 1 {
		return m
	}
	for name := range cfg.Projects {
		m.Preset.PublishDir = "dist/" + name + "/browser"
		m.Evidence = "angular.json has the single project " + name
	}
	return m
}

// detectDeno matches Deno projects by their config file; Fresh is told apart
// by its generated manifest or its import in the config.
func detectDeno(read Reader) (Match, bool) {
	for _, f := range []string{"deno.json", "deno.jsonc"} {
		text, ok := read(f)
		if !ok {
			continue
		}
		if _, gen := read("fresh.gen.ts"); gen {
			return match("fresh", "fresh.gen.ts found"), true
		}
		if strings.Contains(text, "$fresh/") || strings.Contains(text, "@fresh/core") {
			return match("fresh", f+" imports fresh"), true
		}
		return match("deno", f+" found"), true
	}
	return Match{}, false
}

// detectBun is a hint only: a Bun lockfile next to package.json. Framework
// presets win over it, so it never overrides Next.js or similar.
func detectBun(read Reader) (Match, bool) {
	for _, f := range []string{"bun.lockb", "bun.lock"} {
		if _, ok := read(f); ok {
			return match("bun", f+" found"), true
		}
	}
	return Match{}, false
}

// detectDotnet works from the entry point files because Reader cannot list
// *.csproj; the web host builder call is what marks an ASP.NET Core app.
func detectDotnet(read Reader) (Match, bool) {
	for _, f := range []string{"Program.cs", "Startup.cs"} {
		text, ok := read(f)
		if !ok {
			continue
		}
		if strings.Contains(text, "WebApplication.CreateBuilder") || strings.Contains(text, "Microsoft.AspNetCore") || strings.Contains(text, "UseStartup") {
			return match("aspnet-core", f+" builds an ASP.NET Core host"), true
		}
	}
	return Match{}, false
}
