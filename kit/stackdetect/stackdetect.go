// Package stackdetect inspects a local project directory and guesses its
// framework and how to build it (Dockerfile, Compose, Node.js, Go, Java, Python,
// or static site), without executing anything from the project.
package stackdetect

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Build types, matching internal/spec's build.type values.
const (
	BuildDockerfile = "dockerfile"
	BuildRailpack   = "railpack"
	BuildStatic     = "static"
	BuildCompose    = "compose"
)

// Stack is the detection result for one project directory.
type Stack struct {
	// Provider is the Railpack provider id, or "dockerfile", "compose",
	// "static", or "" when nothing matched.
	Provider string
	// Label is the human-readable stack name.
	Label string
	// Build is the app.yaml build.type to use, empty when nothing matched.
	Build string
	// Path is build.path, set for dockerfile, compose and static.
	Path string
	// Port is the container port, 0 for static sites and unknown stacks.
	Port int
	// Name is a suggested service name.
	Name string
	// BuildCommand and Publish are set for static generators (build.command
	// and build.publish), empty for sites that are served as they are.
	BuildCommand string
	Publish      string
	// Notes are short hints for the person reading the result.
	Notes []string
}

// Detected reports whether a buildable stack was found.
func (s Stack) Detected() bool { return s.Build != "" }

var composeFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// Detect inspects dir without executing anything from it. Order: Dockerfile,
// compose file, package.json, go.mod, Java, Python, static site.
func Detect(dir string) (Stack, error) {
	s, err := detectCore(dir)
	if err != nil || !exists(dir, "Procfile") {
		return s, err
	}
	web := procfileWeb(filepath.Join(dir, "Procfile"))
	switch {
	case web == "":
	case !s.Detected():
		s = Stack{Provider: "procfile", Label: "Procfile", Build: BuildRailpack, Port: 8080, Name: s.Name, Notes: []string{"web process: " + web, "port assumed 8080, make the process read $PORT"}}
	default:
		s.Notes = append(s.Notes, "Procfile web process: "+web)
	}
	return s, nil
}

func detectCore(dir string) (Stack, error) {
	name := serviceName(dir)

	if exists(dir, "Dockerfile") {
		s := Stack{Provider: "dockerfile", Label: "Dockerfile", Build: BuildDockerfile, Path: "./Dockerfile", Port: dockerfileExpose(filepath.Join(dir, "Dockerfile")), Name: name}
		if s.Port == 0 {
			s.Port = 8080
			s.Notes = append(s.Notes, "no EXPOSE in the Dockerfile, assumed port 8080")
		}
		return s, nil
	}
	for _, f := range composeFiles {
		if exists(dir, f) {
			return Stack{Provider: "compose", Label: "Docker Compose", Build: BuildCompose, Path: "./" + f, Name: name}, nil
		}
	}
	if g, ok := detectGenerator(dir, name); ok {
		return g, nil
	}
	if exists(dir, "package.json") {
		return detectNode(dir, name)
	}
	if s, ok := detectRuntimes(dir, name); ok {
		return s, nil
	}
	if exists(dir, "go.mod") {
		return Stack{Provider: "golang", Label: "Go", Build: BuildRailpack, Port: 8080, Name: name}, nil
	}
	if exists(dir, "pom.xml") || exists(dir, "build.gradle") || exists(dir, "build.gradle.kts") {
		return Stack{Provider: "java", Label: "Java", Build: BuildRailpack, Port: 8080, Name: name}, nil
	}
	if s, ok := detectPython(dir, name); ok {
		return s, nil
	}
	if exists(dir, "index.html") {
		return Stack{Provider: "static", Label: "Static site", Build: BuildStatic, Path: "./", Name: name}, nil
	}
	return Stack{Name: name, Notes: []string{"no Dockerfile, package.json, go.mod, Python or Java project files, or index.html found"}}, nil
}

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Scripts         map[string]string `json:"scripts"`
}

func detectNode(dir, name string) (Stack, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "package.json")) //nolint:gosec // reads the project the operator pointed at
	if err != nil {
		return Stack{}, fmt.Errorf("read package.json: %w", err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return Stack{}, fmt.Errorf("parse package.json: %w", err)
	}
	has := func(dep string) bool {
		_, a := pkg.Dependencies[dep]
		_, b := pkg.DevDependencies[dep]
		return a || b
	}
	s := Stack{Provider: "node", Label: "Node.js", Build: BuildRailpack, Port: 3000, Name: name}
	spa := has("vite") || has("react-scripts")
	switch {
	case has("next"):
		s.Label = "Node.js (Next.js)"
		s.Notes = append(s.Notes, "next builds can run out of memory on 1 GB hosts, add swap or build elsewhere")
	case has("astro") && has("@astrojs/node"):
		s.Label = "Node.js (Astro SSR)"
		s.Port = 4321
		s.Notes = append(s.Notes, "set HOST=0.0.0.0 and PORT=4321, start with node ./dist/server/entry.mjs")
	case has("@11ty/eleventy"):
		return staticGen("eleventy", "Eleventy", "npx @11ty/eleventy", "_site", name), nil
	case has("gatsby"):
		return staticGen("gatsby", "Gatsby", "npx gatsby build", "public", name), nil
	case has("@angular/core"):
		s.Label = "Node.js (Angular)"
		s.Notes = append(s.Notes, "ng build writes dist/<project>/browser, set it as the publish directory of a static build")
	case has("@nestjs/core"):
		s.Label = "Node.js (NestJS)"
	case has("express"):
		s.Label = "Node.js (Express)"
	case has("fastify"):
		s.Label = "Node.js (Fastify)"
	case spa:
		s.Label = "Node.js (Vite or Create React App)"
		s.Notes = append(s.Notes, "single page apps are served by a static server, check the port after the first deploy")
	}
	if _, ok := pkg.Scripts["start"]; !ok && !spa {
		s.Notes = append(s.Notes, "no start script in package.json, the build may not know how to run it")
	}
	return s, nil
}

func detectPython(dir, name string) (Stack, bool) {
	var marker string
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "setup.py"} {
		if exists(dir, f) {
			marker = f
			break
		}
	}
	if marker == "" {
		return Stack{}, false
	}
	s := Stack{Provider: "python", Label: "Python", Build: BuildRailpack, Port: 8000, Name: name}
	deps := lowerFile(filepath.Join(dir, marker))
	switch {
	case strings.Contains(deps, "django"):
		s.Label = "Python (Django)"
	case strings.Contains(deps, "fastapi"):
		s.Label = "Python (FastAPI)"
	case strings.Contains(deps, "flask"):
		s.Label = "Python (Flask)"
	}
	return s, true
}

func dockerfileExpose(path string) int {
	f, err := os.Open(path) //nolint:gosec // reads the project the operator pointed at
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 || !strings.EqualFold(fields[0], "EXPOSE") {
			continue
		}
		p := strings.TrimSuffix(strings.TrimSuffix(fields[1], "/tcp"), "/udp")
		if n, err := strconv.Atoi(p); err == nil && n > 0 && n < 65536 {
			return n
		}
	}
	return 0
}

func lowerFile(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // reads the project the operator pointed at
	if err != nil {
		return ""
	}
	return strings.ToLower(string(b))
}

func exists(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

// serviceName turns the directory name into a valid spec service name
// (lowercase alphanumerics and hyphens, starting with a letter).
func serviceName(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "web"
	}
	var b strings.Builder
	for _, r := range strings.ToLower(filepath.Base(abs)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		return "web"
	}
	return out
}

func staticGen(provider, label, cmd, publish, name string) Stack {
	return Stack{Provider: provider, Label: label, Build: BuildStatic, Path: "./", BuildCommand: cmd, Publish: publish, Name: name}
}

// detectGenerator finds static site generators that are not npm projects.
func detectGenerator(dir, name string) (Stack, bool) {
	for _, f := range []string{"hugo.toml", "hugo.yaml", "hugo.json"} {
		if exists(dir, f) {
			return staticGen("hugo", "Hugo", "hugo --minify", "public", name), true
		}
	}
	if exists(dir, "_config.yml") && strings.Contains(lowerFile(filepath.Join(dir, "Gemfile")), "jekyll") {
		return staticGen("jekyll", "Jekyll", "bundle exec jekyll build", "_site", name), true
	}
	return Stack{}, false
}

// detectRuntimes covers the languages that have no framework split here.
func detectRuntimes(dir, name string) (Stack, bool) {
	switch {
	case exists(dir, "Gemfile"):
		s := Stack{Provider: "ruby", Label: "Ruby", Build: BuildRailpack, Port: 3000, Name: name}
		gems := lowerFile(filepath.Join(dir, "Gemfile"))
		switch {
		case strings.Contains(gems, "rails"):
			s.Label = "Ruby (Rails)"
			s.Notes = append(s.Notes, "rails builds can run out of memory on 1 GB hosts, add swap or build elsewhere")
		case strings.Contains(gems, "sinatra"):
			s.Label, s.Port = "Ruby (Sinatra)", 4567
		}
		return s, true
	case exists(dir, "composer.json"):
		s := Stack{Provider: "php", Label: "PHP", Build: BuildRailpack, Port: 80, Name: name}
		low := lowerFile(filepath.Join(dir, "composer.json"))
		switch {
		case strings.Contains(low, "laravel/framework"):
			s.Label = "PHP (Laravel)"
		case strings.Contains(low, "symfony/framework-bundle"):
			s.Label = "PHP (Symfony)"
		}
		return s, true
	case exists(dir, "Cargo.toml"):
		s := Stack{Provider: "rust", Label: "Rust", Build: BuildRailpack, Port: 8080, Name: name}
		low := lowerFile(filepath.Join(dir, "Cargo.toml"))
		switch {
		case strings.Contains(low, "axum"):
			s.Label, s.Port = "Rust (axum)", 3000
		case strings.Contains(low, "actix-web"):
			s.Label = "Rust (Actix Web)"
		}
		s.Notes = append(s.Notes, "cargo release builds can run out of memory on 1 GB hosts, add swap or build elsewhere", "the port is a convention, read it from the server code")
		return s, true
	case exists(dir, "mix.exs"):
		s := Stack{Provider: "elixir", Label: "Elixir", Build: BuildRailpack, Port: 4000, Name: name}
		if strings.Contains(lowerFile(filepath.Join(dir, "mix.exs")), "phoenix") {
			s.Label = "Elixir (Phoenix)"
		}
		return s, true
	case exists(dir, "deno.json") || exists(dir, "deno.jsonc"):
		return Stack{Provider: "deno", Label: "Deno", Build: BuildRailpack, Port: 8000, Name: name}, true
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.csproj")); len(m) > 0 {
		return Stack{Provider: "dotnet", Label: ".NET", Build: BuildRailpack, Port: 8080, Name: name, Notes: []string{"the port is the .NET 8+ container default, set ASPNETCORE_URLS to change it"}}, true
	}
	return Stack{}, false
}

func procfileWeb(path string) string {
	f, err := os.Open(path) //nolint:gosec // reads the project the operator pointed at
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if cmd, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "web:"); ok {
			return strings.TrimSpace(cmd)
		}
	}
	return ""
}
