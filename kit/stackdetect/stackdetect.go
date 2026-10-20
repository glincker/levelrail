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
	// Notes are short hints for the person reading the result.
	Notes []string
}

// Detected reports whether a buildable stack was found.
func (s Stack) Detected() bool { return s.Build != "" }

var composeFiles = []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"}

// Detect inspects dir without executing anything from it. Order: Dockerfile,
// compose file, package.json, go.mod, Java, Python, static site.
func Detect(dir string) (Stack, error) {
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
	if exists(dir, "package.json") {
		return detectNode(dir, name)
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
