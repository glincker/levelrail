package appwizard

import (
	"encoding/json"
	"regexp"
	"strings"
)

type pkgJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
}

type nodePkg struct {
	raw    string
	parsed bool
	pkg    pkgJSON
}

func (n *nodePkg) dep(name string) bool {
	if n.parsed {
		_, a := n.pkg.Dependencies[name]
		_, b := n.pkg.DevDependencies[name]
		return a || b
	}
	return strings.Contains(n.raw, `"`+name+`"`)
}

func (n *nodePkg) depPrefix(prefix string) bool {
	if n.parsed {
		for _, m := range []map[string]string{n.pkg.Dependencies, n.pkg.DevDependencies} {
			for k := range m {
				if strings.HasPrefix(k, prefix) {
					return true
				}
			}
		}
		return false
	}
	return strings.Contains(n.raw, `"`+prefix)
}

var nextOutputRe = regexp.MustCompile(`output\s*:\s*['"](export|standalone)['"]`)

func lockfile(f *files) (pm, file string) {
	for _, c := range []struct{ pm, file string }{
		{"pnpm", "pnpm-lock.yaml"}, {"yarn", "yarn.lock"}, {"bun", "bun.lockb"}, {"bun", "bun.lock"}, {"npm", "package-lock.json"}, {"npm", "npm-shrinkwrap.json"},
	} {
		if f.has(c.file) || f.present[c.file] {
			return c.pm, c.file
		}
	}
	return "npm", ""
}

func detectNode(f *files) (found, bool) {
	raw, ok := f.text("package.json")
	if !ok {
		return found{}, false
	}
	n := &nodePkg{raw: raw}
	if err := json.Unmarshal([]byte(raw), &n.pkg); err == nil {
		n.parsed = true
	}
	g := found{language: "node", malformed: !n.parsed, confidence: ConfidenceHigh}
	if !n.parsed {
		g.why("package.json", "package.json could not be parsed, so dependencies were matched from its raw text")
	}
	pm, lock := lockfile(f)
	if lock == "" {
		g.noLockfile = true
	} else {
		g.why("lockfile", lock+" is present, so "+pm+" installs dependencies")
	}
	g.version = nodeVersion(f, n)
	if n.parsed {
		if n.pkg.Scripts["start"] != "" {
			g.start = pm + " start"
		}
		if n.pkg.Scripts["build"] != "" {
			g.build = pm + " run build"
		}
		if p, why := portFromScripts(raw); p > 0 {
			g.port = PortGuess{p, why}
		}
	}
	setPort := func(p int, src string) {
		if g.port.Value == 0 {
			g.port = PortGuess{p, src}
		}
	}

	switch {
	case n.dep("next"):
		g.framework, g.buildType = "Next.js", buildRailpack
		g.why("package.json", "package.json has next in dependencies")
		nextDetails(f, &g)
		setPort(3000, "Next.js default port")
	case n.dep("nuxt") || n.dep("nuxt3"):
		g.framework, g.buildType = "Nuxt", buildRailpack
		g.why("package.json", "package.json has nuxt in dependencies")
		g.build = orStr(g.build, "nuxt build")
		g.start = orStr(g.start, "node .output/server/index.mjs")
		setPort(3000, "Nuxt default port")
	case n.dep("@sveltejs/kit"):
		g.framework, g.buildType = "SvelteKit", buildRailpack
		g.why("package.json", "package.json has @sveltejs/kit in dependencies")
		g.build = orStr(g.build, "vite build")
		if n.dep("@sveltejs/adapter-static") {
			g.output, g.start = "build", ""
			g.why("package.json", "@sveltejs/adapter-static is installed, so the site is prerendered into build/")
		} else {
			g.start = orStr(g.start, "node build")
			setPort(3000, "SvelteKit node adapter default port")
		}
	case n.dep("astro"):
		g.framework, g.buildType = "Astro", buildRailpack
		g.why("package.json", "package.json has astro in dependencies")
		g.build = orStr(g.build, "astro build")
		if n.dep("@astrojs/node") {
			g.start = orStr(g.start, "node ./dist/server/entry.mjs")
			setPort(4321, "Astro node adapter default port")
			g.why("package.json", "@astrojs/node is installed, so Astro runs as a server")
		} else {
			g.output, g.start = "dist", ""
			g.why("package.json", "no server adapter is installed, so Astro builds a static site into dist/")
		}
	case n.depPrefix("@remix-run/") || n.depPrefix("@react-router/"):
		g.framework, g.buildType = "Remix", buildRailpack
		g.why("package.json", "package.json has a @remix-run or @react-router package")
		g.build = orStr(g.build, "remix vite:build")
		g.start = orStr(g.start, "remix-serve ./build/server/index.js")
		setPort(3000, "Remix default port")
	case n.dep("@nestjs/core"):
		g.framework, g.buildType = "NestJS", buildRailpack
		g.why("package.json", "package.json has @nestjs/core in dependencies")
		g.build = orStr(g.build, "nest build")
		g.start = orStr(g.start, "node dist/main")
		setPort(3000, "NestJS default port")
	case n.dep("fastify"):
		g.framework, g.buildType, g.confidence = "Fastify", buildRailpack, ConfidenceMedium
		g.why("package.json", "package.json has fastify in dependencies")
		setPort(3000, "Fastify convention, verify in the server code")
	case n.dep("express"):
		g.framework, g.buildType, g.confidence = "Express", buildRailpack, ConfidenceMedium
		g.why("package.json", "package.json has express in dependencies")
		setPort(3000, "Express convention, verify in the server code")
	case n.dep("vite") || n.dep("react-scripts"):
		g.framework, g.buildType = "Vite", buildRailpack
		g.output, g.start = "dist", ""
		if n.dep("react-scripts") {
			g.framework, g.output = "Create React App", "build"
		}
		g.why("package.json", "package.json has a frontend build tool and no server framework, so it builds a single page app into "+g.output+"/")
		g.build = orStr(g.build, "vite build")
	case n.parsed && n.pkg.Scripts["start"] != "":
		g.framework, g.buildType, g.confidence = "Node.js", buildRailpack, ConfidenceMedium
		g.why("package.json", "package.json defines a start script")
	default:
		g.framework, g.buildType, g.confidence = "Node.js", buildRailpack, ConfidenceLow
		g.why("package.json", "package.json exists but no known framework or start script was found")
	}
	return g, true
}

func nextDetails(f *files, g *found) {
	cfgName, _ := f.anyOf("next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs")
	if cfgName != "" {
		g.why("config", cfgName+" exists")
		if cfg, ok := f.text(cfgName); ok {
			if m := nextOutputRe.FindStringSubmatch(cfg); m != nil {
				switch m[1] {
				case "export":
					g.output, g.start = "out", ""
					g.why("config", cfgName+" sets output: 'export', so the site is static and lives in out/")
				case "standalone":
					g.start = "node .next/standalone/server.js"
					g.why("config", cfgName+" sets output: 'standalone', so the server starts from .next/standalone")
				}
			}
		}
	}
	switch {
	case f.anyUnder("app") || f.anyUnder("src/app"):
		g.why("directory", "an app/ directory exists, so the App Router is in use")
	case f.anyUnder("pages") || f.anyUnder("src/pages"):
		g.why("directory", "a pages/ directory exists, so the Pages Router is in use")
	}
	g.build = orStr(g.build, "next build")
	if g.start == "" && g.output == "" {
		g.start = "next start"
	}
}

var nvmRe = regexp.MustCompile(`\d+(\.\d+){0,2}`)

func nodeVersion(f *files, n *nodePkg) Sourced {
	if n.parsed {
		if v := strings.TrimSpace(n.pkg.Engines["node"]); v != "" {
			return Sourced{v, "package.json engines.node"}
		}
	}
	for _, name := range []string{".nvmrc", ".node-version"} {
		if s, ok := f.text(name); ok {
			if v := nvmRe.FindString(s); v != "" {
				return Sourced{v, name}
			}
		}
	}
	return Sourced{}
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
