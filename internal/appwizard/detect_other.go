package appwizard

import (
	"path"
	"regexp"
	"strings"
)

const (
	rankWeak    = 1
	rankGeneric = 2
	rankNamed   = 3
)

type candidate struct {
	g    found
	rank int
}

// detectManifest runs every language detector and keeps the most specific
// result, so a Rails app with a package.json for its assets is still Rails.
func detectManifest(f *files) found {
	var cands []candidate
	if g, ok := detectNode(f); ok {
		cands = append(cands, candidate{g, nodeRank(g)})
	}
	for _, d := range []func(*files) (found, int, bool){detectPython, detectRuby, detectPHP, detectGo, detectRust, detectJava} {
		if g, rank, ok := d(f); ok {
			cands = append(cands, candidate{g, rank})
		}
	}
	if len(cands) == 0 {
		if f.has("index.html") {
			g := found{framework: "Static site", language: "html", buildType: buildStatic, confidence: ConfidenceHigh, output: ".", health: "/", port: PortGuess{staticPort, "static sites are served by the built-in static server"}}
			g.why("file", "index.html is present and no application manifest was found")
			return g
		}
		return found{}
	}
	best := 0
	for i, c := range cands {
		if c.rank > cands[best].rank {
			best = i
		}
	}
	out := cands[best].g
	for i, c := range cands {
		if i != best {
			out.why("conflict", "also found "+c.g.framework+" ("+c.g.language+"); "+out.framework+" is more specific")
		}
	}
	return out
}

func nodeRank(g found) int {
	switch g.framework {
	case "Node.js":
		return rankWeak
	case "Express", "Fastify", "Vite", "Create React App":
		return rankGeneric
	}
	return rankNamed
}

func lower(f *files, names ...string) (string, []string) {
	var sb strings.Builder
	var used []string
	for _, n := range names {
		if s, ok := f.text(n); ok {
			sb.WriteString(strings.ToLower(s))
			sb.WriteByte('\n')
			used = append(used, n)
		}
	}
	return sb.String(), used
}

func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

var (
	pyReqRe     = regexp.MustCompile(`requires-python\s*=\s*"([^"]+)"`)
	pyPoetryRe  = regexp.MustCompile(`(?m)^python\s*=\s*"([^"]+)"`)
	pyRuntimeRe = regexp.MustCompile(`python-?(\d+(\.\d+){0,2})`)
)

func detectPython(f *files) (found, int, bool) {
	text, used := lower(f, "requirements.txt", "pyproject.toml", "Pipfile")
	if len(used) == 0 {
		return found{}, 0, false
	}
	g := found{language: "python", confidence: ConfidenceHigh, buildType: buildRailpack}
	g.why("file", strings.Join(used, ", ")+" present")
	has := func(name string) bool {
		return regexp.MustCompile(`(?m)(^|[^a-z0-9_-])` + regexp.QuoteMeta(name) + `([^a-z0-9_-]|$)`).MatchString(text)
	}
	rank := rankNamed
	switch {
	case has("django") || f.has("manage.py"):
		g.framework = "Django"
		g.why("dependency", "django is listed in the Python dependencies or manage.py exists")
		g.port = PortGuess{8000, "Django convention"}
		if w := findSuffix(f, "wsgi.py"); w != "" {
			g.start = "gunicorn " + strings.ReplaceAll(strings.TrimSuffix(w, ".py"), "/", ".") + ":application"
			g.why("file", w+" exists, so gunicorn serves it")
		}
	case has("fastapi"):
		g.framework = "FastAPI"
		g.why("dependency", "fastapi is listed in the Python dependencies")
		g.port = PortGuess{8000, "FastAPI convention (uvicorn)"}
		mod := "main"
		if f.has("app/main.py") {
			mod = "app.main"
		}
		g.start = "uvicorn " + mod + ":app --host 0.0.0.0 --port 8000"
	case has("flask"):
		g.framework = "Flask"
		g.why("dependency", "flask is listed in the Python dependencies")
		g.port = PortGuess{5000, "Flask convention"}
		if f.has("app.py") {
			g.start = "gunicorn app:app"
		}
	default:
		g.framework, g.confidence = "Python", ConfidenceLow
		g.why("file", "no known web framework is listed")
		rank = rankWeak
	}
	g.version = pythonVersion(f)
	hasLock := f.has("poetry.lock") || f.has("uv.lock") || f.has("Pipfile.lock") || f.has("requirements.txt")
	g.noLockfile = !hasLock
	return g, rank, true
}

func findSuffix(f *files, suffix string) string {
	best := ""
	for p := range f.present {
		if !strings.HasSuffix(p, "/"+suffix) && p != suffix {
			continue
		}
		rel := p
		if f.dir != "" {
			if !strings.HasPrefix(p, f.dir+"/") {
				continue
			}
			rel = strings.TrimPrefix(p, f.dir+"/")
		}
		if best == "" || len(rel) < len(best) || (len(rel) == len(best) && rel < best) {
			best = rel
		}
	}
	return best
}

func pythonVersion(f *files) Sourced {
	if s, ok := f.text(".python-version"); ok {
		if v := nvmRe.FindString(s); v != "" {
			return Sourced{v, ".python-version"}
		}
	}
	if s, ok := f.text("runtime.txt"); ok {
		if v := firstMatch(pyRuntimeRe, strings.ToLower(s)); v != "" {
			return Sourced{v, "runtime.txt"}
		}
	}
	if s, ok := f.text("pyproject.toml"); ok {
		if v := firstMatch(pyReqRe, s); v != "" {
			return Sourced{v, "pyproject.toml requires-python"}
		}
		if v := firstMatch(pyPoetryRe, s); v != "" {
			return Sourced{v, "pyproject.toml tool.poetry python"}
		}
	}
	return Sourced{}
}

var (
	gemRailsRe = regexp.MustCompile(`(?m)^\s*gem\s+['"]rails['"]`)
	gemRubyRe  = regexp.MustCompile(`(?m)^\s*ruby\s+['"]([^'"]+)['"]`)
)

func detectRuby(f *files) (found, int, bool) {
	text, ok := f.text("Gemfile")
	if !ok {
		return found{}, 0, false
	}
	g := found{language: "ruby", buildType: buildRailpack, confidence: ConfidenceHigh, noLockfile: !f.has("Gemfile.lock")}
	rank := rankNamed
	switch {
	case gemRailsRe.MatchString(text):
		g.framework, g.health = "Rails", "/up"
		g.port = PortGuess{3000, "Rails default port"}
		g.start = "bin/rails server -b 0.0.0.0"
		g.why("dependency", "Gemfile lists the rails gem")
		if f.has("config/application.rb") {
			g.why("file", "config/application.rb exists")
		}
	case strings.Contains(strings.ToLower(text), "sinatra"):
		g.framework, g.port = "Sinatra", PortGuess{4567, "Sinatra default port"}
		g.why("dependency", "Gemfile lists sinatra")
	default:
		g.framework, g.confidence = "Ruby", ConfidenceLow
		g.port = PortGuess{3000, "Ruby convention, verify in the app"}
		g.why("file", "Gemfile found with no known web framework")
		rank = rankWeak
	}
	if s, ok := f.text(".ruby-version"); ok {
		if v := nvmRe.FindString(s); v != "" {
			g.version = Sourced{v, ".ruby-version"}
		}
	}
	if g.version.Value == "" {
		if v := firstMatch(gemRubyRe, text); v != "" {
			g.version = Sourced{v, "Gemfile ruby directive"}
		}
	}
	return g, rank, true
}

var phpReqRe = regexp.MustCompile(`"php"\s*:\s*"([^"]+)"`)

func detectPHP(f *files) (found, int, bool) {
	text, ok := f.text("composer.json")
	if !ok {
		return found{}, 0, false
	}
	low := strings.ToLower(text)
	g := found{language: "php", buildType: buildRailpack, confidence: ConfidenceHigh, noLockfile: !f.has("composer.lock")}
	g.port = PortGuess{80, "PHP images serve on port 80"}
	rank := rankNamed
	switch {
	case strings.Contains(low, "laravel/framework"):
		g.framework = "Laravel"
		g.why("dependency", "composer.json requires laravel/framework")
		if f.has("artisan") {
			g.why("file", "artisan exists")
		}
	case strings.Contains(low, "symfony/framework-bundle"):
		g.framework = "Symfony"
		g.why("dependency", "composer.json requires symfony/framework-bundle")
	default:
		g.framework, g.confidence = "PHP", ConfidenceLow
		g.why("file", "composer.json found with no known framework")
		rank = rankWeak
	}
	if v := firstMatch(phpReqRe, text); v != "" {
		g.version = Sourced{v, "composer.json require.php"}
	}
	return g, rank, true
}

var (
	goVerRe    = regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(\.\d+)?)`)
	goModNames = map[string]string{"gin-gonic/gin": "Gin", "labstack/echo": "Echo", "gofiber/fiber": "Fiber", "go-chi/chi": "chi"}
)

func detectGo(f *files) (found, int, bool) {
	text, ok := f.text("go.mod")
	if !ok {
		return found{}, 0, false
	}
	g := found{language: "go", framework: "Go (net/http)", buildType: buildRailpack, confidence: ConfidenceMedium, noLockfile: !f.has("go.sum")}
	g.why("file", "go.mod is present")
	for mod, name := range goModNames {
		if strings.Contains(text, mod) {
			g.framework = "Go (" + name + ")"
			g.why("dependency", "go.mod requires "+mod)
			break
		}
	}
	g.port = PortGuess{8080, "Go convention, verify in the server code"}
	g.build, g.start = "go build -o app .", "./app"
	if v := firstMatch(goVerRe, text); v != "" {
		g.version = Sourced{v, "go.mod go directive"}
	}
	return g, rankGeneric, true
}

var rustVerRe = regexp.MustCompile(`rust-version\s*=\s*"([^"]+)"`)

func detectRust(f *files) (found, int, bool) {
	text, ok := f.text("Cargo.toml")
	if !ok {
		return found{}, 0, false
	}
	low := strings.ToLower(text)
	g := found{language: "rust", framework: "Rust", buildType: buildRailpack, confidence: ConfidenceMedium, noLockfile: !f.has("Cargo.lock")}
	g.why("file", "Cargo.toml is present")
	rank := rankGeneric
	switch {
	case strings.Contains(low, "axum"):
		g.framework, g.confidence = "Rust (axum)", ConfidenceHigh
		g.why("dependency", "Cargo.toml lists axum")
		g.port = PortGuess{3000, "axum examples listen on 3000, verify in the server code"}
		rank = rankNamed
	case strings.Contains(low, "actix-web"):
		g.framework, g.port = "Rust (Actix Web)", PortGuess{8080, "Actix convention, verify in the server code"}
		g.why("dependency", "Cargo.toml lists actix-web")
	}
	g.build = "cargo build --release"
	if v := firstMatch(rustVerRe, text); v != "" {
		g.version = Sourced{v, "Cargo.toml rust-version"}
	} else if s, ok := f.text("rust-toolchain.toml"); ok {
		if v := firstMatch(regexp.MustCompile(`channel\s*=\s*"([^"]+)"`), s); v != "" {
			g.version = Sourced{v, "rust-toolchain.toml"}
		}
	}
	return g, rank, true
}

var (
	javaPropRe   = regexp.MustCompile(`<(?:java\.version|maven\.compiler\.release|maven\.compiler\.source)>\s*(\d+(?:\.\d+)?)\s*<`)
	javaGradleRe = regexp.MustCompile(`(?:JavaLanguageVersion\.of\(|sourceCompatibility\s*=\s*['"]?(?:JavaVersion\.VERSION_)?)(\d+(?:[._]\d+)?)`)
	serverPortRe = regexp.MustCompile(`(?m)^\s*server\.port\s*[=:]\s*(\d+)`)
)

func detectJava(f *files) (found, int, bool) {
	name, ok := f.anyOf("pom.xml", "build.gradle", "build.gradle.kts")
	if !ok {
		return found{}, 0, false
	}
	text, _ := f.text(name)
	low := strings.ToLower(text)
	g := found{language: "java", framework: "Java", buildType: buildRailpack, confidence: ConfidenceMedium}
	g.why("file", name+" is present")
	rank := rankGeneric
	if strings.Contains(low, "spring-boot") {
		g.framework, g.confidence = "Spring Boot", ConfidenceHigh
		g.why("dependency", name+" uses spring-boot")
		g.port = PortGuess{8080, "Spring Boot default port"}
		if props, ok := f.text("src/main/resources/application.properties"); ok {
			if p := firstMatch(serverPortRe, props); p != "" {
				g.port = PortGuess{atoiSafe(p), "server.port in application.properties"}
			}
		}
		if strings.Contains(low, "actuator") {
			g.health = "/actuator/health"
			g.why("dependency", "spring-boot-starter-actuator is present, so /actuator/health exists")
		}
		rank = rankNamed
	}
	if v := firstMatch(javaPropRe, text); v != "" {
		g.version = Sourced{v, name + " java version property"}
	} else if v := firstMatch(javaGradleRe, text); v != "" {
		g.version = Sourced{strings.ReplaceAll(v, "_", "."), name + " Java toolchain"}
	}
	if path.Base(name) == "pom.xml" {
		g.build = "mvn -B package -DskipTests"
	} else {
		g.build = "gradle build -x test"
	}
	return g, rank, true
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' || n > 65535 {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
