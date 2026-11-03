package appwizard

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pkgJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
	Workspaces      json.RawMessage   `json:"workspaces"`
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

var (
	astroServerRe  = regexp.MustCompile(`output\s*:\s*['"]server['"]`)
	svelteCfgPages = regexp.MustCompile(`pages\s*:\s*['"]([^'"]+)['"]`)
	nodeStartRe    = regexp.MustCompile(`(^|\s)(node|nodemon|tsx|ts-node|bun)\s|server\.(js|ts|mjs|cjs)`)
)

// astroAdapters are the Astro adapters that add a server runtime.
var astroAdapters = []string{"@astrojs/node", "@astrojs/vercel", "@astrojs/cloudflare", "@astrojs/netlify", "@astrojs/deno"}

// hasServerRuntime is true when a frontend project also ships its own server.
func (n *nodePkg) hasServerRuntime() bool {
	for _, d := range []string{"express", "fastify", "koa", "@hono/node-server", "@nestjs/core"} {
		if n.dep(d) {
			return true
		}
	}
	return n.parsed && nodeStartRe.MatchString(n.pkg.Scripts["start"])
}

func (n *nodePkg) clientRouter() bool {
	return n.dep("react-router") || n.dep("react-router-dom") || n.dep("vue-router")
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

	static := false
	switch {
	case n.dep("next"):
		g.framework, g.buildType = "Next.js", buildRailpack
		g.why("package.json", "package.json has next in dependencies")
		nextDetails(f, &g)
		nextProfile(f, n, &g)
		static = g.output != "" && g.start == ""
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
			if cfg, ok := f.text("svelte.config.js"); ok {
				if m := svelteCfgPages.FindStringSubmatch(cfg); m != nil {
					g.output = strings.Trim(m[1], "/")
				}
			}
			static = true
			g.why("package.json", "@sveltejs/adapter-static is installed, so the site is prerendered into "+g.output+"/")
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
		} else if astroServerOutput(f, n) {
			setPort(4321, "Astro default port")
			g.why("package.json", "an Astro server adapter or output: 'server' is configured, so Astro runs as a server")
		} else {
			g.output, g.start = "dist", ""
			static = true
			g.why("package.json", "no server adapter is installed, so Astro builds a static site into dist/")
		}
	case n.dep("@docusaurus/core"):
		g.framework, g.buildType = "Docusaurus", buildRailpack
		g.build = orStr(g.build, "docusaurus build")
		g.output, g.start = "build", ""
		static = true
		g.why("package.json", "package.json has @docusaurus/core, so the site builds into build/")
	case n.dep("@11ty/eleventy") && !n.hasServerRuntime():
		g.framework, g.buildType = "Eleventy", buildRailpack
		g.build = orStr(g.build, "eleventy")
		g.output, g.start = "_site", ""
		static = true
		g.why("package.json", "package.json has @11ty/eleventy, so the site builds into _site/")
	case n.dep("gatsby"):
		g.framework, g.buildType = "Gatsby", buildRailpack
		g.build = orStr(g.build, "gatsby build")
		g.output, g.start = "public", ""
		static = true
		g.why("package.json", "package.json has gatsby, so the site builds into public/")
	case n.dep("@angular/core"):
		g.framework, g.buildType = "Angular", buildRailpack
		g.build = orStr(g.build, "ng build")
		g.why("package.json", "package.json has @angular/core; ng build writes dist/<project>/browser, set it as the publish directory for a static build")
		setPort(3000, "Node convention, verify in the server code")
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
	case (n.dep("vite") || n.dep("react-scripts")) && !n.hasServerRuntime():
		g.framework, g.buildType = "Vite", buildRailpack
		g.output, g.start = "dist", ""
		static = true
		if n.dep("react-scripts") {
			g.framework, g.output = "Create React App", "build"
		}
		g.why("package.json", "package.json has a frontend build tool and no server framework, so it builds a single page app into "+g.output+"/")
		g.build = orStr(g.build, "vite build")
		if n.clientRouter() {
			g.spa = true
			g.why("package.json", "a client side router is installed, so unknown paths should fall back to index.html (single page app mode)")
		}
	case n.parsed && n.pkg.Scripts["start"] != "":
		g.framework, g.buildType, g.confidence = "Node.js", buildRailpack, ConfidenceMedium
		g.why("package.json", "package.json defines a start script")
	default:
		g.framework, g.buildType, g.confidence = "Node.js", buildRailpack, ConfidenceLow
		g.why("package.json", "package.json exists but no known framework or start script was found")
	}
	if static {
		g.buildType = buildStatic
		g.start = ""
		g.port = PortGuess{staticPort, "static sites are served by the built-in static server"}
	}
	return g, true
}

func astroServerOutput(f *files, n *nodePkg) bool {
	for _, a := range astroAdapters {
		if n.dep(a) {
			return true
		}
	}
	name, ok := f.anyOf("astro.config.mjs", "astro.config.ts", "astro.config.js", "astro.config.cjs")
	if !ok {
		return false
	}
	cfg, _ := f.text(name)
	return astroServerRe.MatchString(cfg)
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
					g.why("config", cfgName+" sets output: 'standalone', so the server starts from .next/standalone; the build copies .next/static and public next to it and binds HOSTNAME 0.0.0.0")
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

// EnvMaxNextScanFiles bounds how many source files one Next.js scan reads.
const EnvMaxNextScanFiles = "APP_WIZARD_NEXT_SCAN_FILES"

var (
	nextCommentRe     = regexp.MustCompile(`(?s)/\*.*?\*/|(^|[\s;,{(])//[^\n]*`)
	nextHandlerRe     = regexp.MustCompile(`\bcacheHandlers?\s*:`)
	nextMaxMemRe      = regexp.MustCompile(`\bcacheMaxMemorySize\s*:\s*(\d+)`)
	nextTracingRe     = regexp.MustCompile(`\boutputFileTracingRoot\b`)
	nextPublicRe      = regexp.MustCompile(`\bNEXT_PUBLIC_[A-Z0-9_]+`)
	nextRevalidateRe  = regexp.MustCompile(`\brevalidate\s*=\s*[1-9]|\brevalidate\s*:\s*[1-9]|\b(revalidatePath|revalidateTag|unstable_cache|cacheLife)\b|\bgetStaticProps\b`)
	nextVersionRe     = regexp.MustCompile(`(\d+)(?:\.(\d+))?(?:\.(\d+))?`)
	nextSourceExtRe   = regexp.MustCompile(`\.(js|jsx|ts|tsx|mjs|cjs)$`)
	nextExperimentals = []string{"cacheComponents", "dynamicIO", "ppr", "useCache", "reactCompiler", "inlineCss"}
	nextExperimentRes = func() map[string]*regexp.Regexp {
		m := map[string]*regexp.Regexp{}
		for _, k := range nextExperimentals {
			m[k] = regexp.MustCompile(`\b` + k + `\s*:\s*(true|['"]incremental['"]|\{)`)
		}
		return m
	}()
)

// nextProfile fills the Next.js profile and its hints. It reads text only,
// so a hostile config cannot run anything.
func nextProfile(f *files, n *nodePkg, g *found) {
	p := &NextProfile{Output: "none", Experimental: []string{}}
	if n.parsed {
		p.Version, p.Major = nextVersion(firstNonEmpty(n.pkg.Dependencies["next"], n.pkg.DevDependencies["next"]))
		w := strings.TrimSpace(string(n.pkg.Workspaces))
		p.Monorepo = w != "" && w != "null"
	}
	p.Sharp = n.dep("sharp")
	if f.dir != "" || f.present["pnpm-workspace.yaml"] {
		p.Monorepo = true
	}
	if cfgName, ok := f.anyOf("next.config.js", "next.config.mjs", "next.config.ts", "next.config.cjs"); ok {
		cfg, _ := f.text(cfgName)
		cfg = nextCommentRe.ReplaceAllString(cfg, "$1")
		if m := nextOutputRe.FindStringSubmatch(cfg); m != nil {
			p.Output = m[1]
		}
		p.CacheHandler = nextHandlerRe.MatchString(cfg)
		if m := nextMaxMemRe.FindStringSubmatch(cfg); m != nil {
			p.CacheMaxMemory, _ = strconv.ParseInt(m[1], 10, 64)
		}
		p.TracingRoot = nextTracingRe.MatchString(cfg)
		for _, k := range nextExperimentals {
			if nextExperimentRes[k].MatchString(cfg) {
				p.Experimental = append(p.Experimental, k)
				if k == "cacheComponents" {
					p.CacheComponents = true
				}
			}
		}
	}
	for _, name := range []string{"middleware.ts", "middleware.js", "proxy.ts", "proxy.js", "src/middleware.ts", "src/middleware.js", "src/proxy.ts", "src/proxy.js"} {
		if f.has(name) {
			p.Middleware = name
			break
		}
	}
	scanNextSources(f, p)
	p.MultiReplicaRisk = (p.UsesRevalidate || p.CacheComponents) && !p.CacheHandler && p.Output != "export"
	g.next = p
	g.nextHints = nextHints(p)
}

// scanNextSources counts NEXT_PUBLIC_ names and spots ISR style caching in a
// bounded, sorted sample of source files.
func scanNextSources(f *files, p *NextProfile) {
	var names []string
	for k := range f.present {
		rel := k
		if f.dir != "" {
			if !strings.HasPrefix(k, f.dir+"/") {
				continue
			}
			rel = strings.TrimPrefix(k, f.dir+"/")
		}
		if strings.Contains(rel, "node_modules/") || strings.HasPrefix(rel, ".next/") || !nextSourceExtRe.MatchString(rel) {
			continue
		}
		top := strings.SplitN(rel, "/", 2)[0]
		switch top {
		case "app", "pages", "src", "components", "lib", "utils":
			names = append(names, rel)
		}
	}
	sort.Strings(names)
	limit := envInt(EnvMaxNextScanFiles, 200)
	if len(names) > limit {
		names = names[:limit]
	}
	public := map[string]bool{}
	for _, rel := range names {
		s, ok := f.text(rel)
		if !ok {
			continue
		}
		for _, v := range nextPublicRe.FindAllString(s, -1) {
			public[v] = true
		}
		if !p.UsesRevalidate && nextRevalidateRe.MatchString(s) {
			p.UsesRevalidate = true
		}
	}
	for _, name := range []string{".env.example", ".env.sample", ".env.template"} {
		if s, ok := f.text(name); ok {
			for _, v := range nextPublicRe.FindAllString(s, -1) {
				public[v] = true
			}
		}
	}
	p.PublicVars = len(public)
}

func nextHints(p *NextProfile) []Warning {
	var out []Warning
	add := func(code, msg string) { out = append(out, Warning{Code: code, Message: msg}) }
	if p.Output == "none" {
		add(HintNextNoOutput, "next.config has no output mode; output: 'standalone' gives a smaller image that starts with node server.js")
	}
	if p.Output == "standalone" && !p.Sharp && p.Major > 0 && p.Major < 15 {
		add(HintNextSharp, "this Next.js version needs sharp installed for image optimization in standalone mode")
	}
	if p.Monorepo && p.Output == "standalone" && !p.TracingRoot {
		add(HintNextTracingRoot, "this looks like a monorepo; set outputFileTracingRoot so the standalone build includes files from the workspace root")
	}
	if p.Middleware != "" && p.Output == "export" {
		add(HintNextExportServer, p.Middleware+" does not run with output: 'export'; the static build ignores it")
	}
	if p.MultiReplicaRisk {
		add(HintNextMultiReplica, "revalidate or cache components are used without a shared cacheHandler; replicas keep separate caches, so keep one replica or add a shared cache handler")
	}
	if p.PublicVars > 0 {
		add(HintNextPublicVars, strconv.Itoa(p.PublicVars)+" NEXT_PUBLIC_ variables are inlined at build time; set them as build variables, not only at runtime")
	}
	return out
}

func nextVersion(spec string) (string, int) {
	m := nextVersionRe.FindStringSubmatch(spec)
	if m == nil {
		return "", 0
	}
	v := m[1]
	for _, part := range m[2:] {
		if part == "" {
			break
		}
		v += "." + part
	}
	major, _ := strconv.Atoi(m[1])
	return v, major
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
