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
	if text, ok := read("package.json"); ok {
		if m, ok := detectNode(text, read); ok {
			return m, true
		}
	}
	if _, ok := read("go.mod"); ok {
		return match("go", "go.mod found"), true
	}
	for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		if text, ok := read(f); ok && strings.Contains(strings.ToLower(text), "spring-boot") {
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
	case has("astro"):
		return match("astro", "package.json lists astro"), true
	case has("vite"):
		return match("vite", "package.json lists vite"), true
	}
	for _, name := range []string{"express", "fastify", "koa", "hono", "@nestjs/core"} {
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
