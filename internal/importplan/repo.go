package importplan

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
)

var composeFileNames = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml"}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// frameworkGuess is a stack detected from manifest files alone.
type frameworkGuess struct {
	Name   string
	Method BuildMethod
	Port   int
	Health string
	Start  string
	Why    string
	Env    []EnvVar
}

func planRepo(ctx context.Context, in Input, deps Deps) (*DeploymentPlan, error) {
	repoURL, repo, err := repoParts(in.Text)
	if err != nil {
		return nil, err
	}
	name := slugify(repo)
	p := &DeploymentPlan{Source: SourceRepo, SuggestedName: name, RepoURL: repoURL, Ref: in.Ref, Deploy: DeployBuild, Warnings: []Warning{}, DomainSuggestion: domainHint(name)}
	if deps.Files == nil {
		p.Deploy = DeployNone
		p.Services = []ServicePlan{{Name: name, Build: BuildUnknown, BuildReason: "repository inspection is not available"}}
		p.Warnings = append(p.Warnings, Warning{Code: "no_inspection", Message: "the repository could not be inspected; choose a build type manually"})
		return p, nil
	}
	read := func(path string) (string, bool, error) {
		b, ok, err := deps.Files.ReadFile(ctx, repoURL, in.Ref, path)
		return string(b), ok, err
	}
	svc, extra, err := inspectRepo(ctx, repoURL, in.Ref, read, deps, p)
	if err != nil {
		if errors.Is(err, ErrFileLimit) {
			p.Warnings = append(p.Warnings, Warning{Code: "file_limit", Message: "the repository inspection hit its file limit; results may be incomplete"})
		} else {
			return nil, err
		}
	}
	svc.Name = name
	svc.Env = mergeEnv(svc.Env, extra)
	if svc.Build == BuildUnknown {
		p.Deploy = DeployNone
		p.Warnings = append(p.Warnings, Warning{Code: "nothing_detected", Message: "no Dockerfile, compose file or known framework was found; choose a build type manually"})
	}
	if svc.Port == 0 && svc.Build != BuildUnknown && svc.Build != BuildStatic {
		p.Warnings = append(p.Warnings, Warning{Code: "port_unknown", Message: "no port could be detected; set the port the app listens on"})
	}
	if p.Services == nil {
		p.Services = []ServicePlan{svc}
	}
	return p, nil
}

// inspectRepo reads the repo's key files and returns the primary service plan
// and any env variables discovered in example env files. Repo text is parsed
// as data only.
func inspectRepo(ctx context.Context, repoURL, ref string, read func(string) (string, bool, error), deps Deps, p *DeploymentPlan) (ServicePlan, []EnvVar, error) {
	var envs []EnvVar
	for _, f := range EnvExampleFiles {
		text, ok, err := read(f)
		if err != nil {
			return ServicePlan{Build: BuildUnknown}, envs, err
		}
		if ok {
			envs = append(envs, ParseEnvFile(text, f)...)
		}
	}

	for _, cf := range composeFileNames {
		text, ok, err := read(cf)
		if err != nil {
			return ServicePlan{Build: BuildUnknown}, envs, err
		}
		if !ok {
			continue
		}

		// Extract depends_on targets to pass to service selection.
		depTargets := make(map[string]bool)
		parsed, _ := compose.Parse([]byte(text))
		if parsed != nil {
			for _, s := range parsed.Services {
				for _, dep := range s.DependsOn {
					depTargets[dep] = true
				}
			}
		}

		cp, cerr := composePlanFromText([]byte(text), cf, depTargets)
		if cerr == nil && !hasBuildService(cp) && cp.Deploy == DeployCompose {
			p.Deploy = DeployCompose
			p.ComposeYAML = cp.ComposeYAML
			p.Services = cp.Services
			for i := range p.Services {
				p.Services[i].BuildReason = "compose file " + cf + " in the repository"
			}
			p.Warnings = append(p.Warnings, cp.Warnings...)
			return p.Services[0], envs, nil
		}
		p.Warnings = append(p.Warnings, Warning{Code: "compose_skipped", Message: cf + " builds from source or is unsupported, so the Dockerfile or framework path is used instead"})
		break
	}

	if text, ok, err := read("Dockerfile"); err != nil {
		return ServicePlan{Build: BuildUnknown}, envs, err
	} else if ok {
		ports, denv := parseDockerfile(text)
		svc := ServicePlan{Build: BuildDockerfile, BuildReason: "Dockerfile found at the repository root", Env: denv}
		if len(ports) > 0 {
			svc.Port = ports[0]
		}
		if g, ok := guessFramework(read); ok && svc.Port == 0 {
			svc.Port = g.Port
		}
		return svc, envs, nil
	}

	g, ok := guessFramework(read)
	if ok {
		svc := ServicePlan{Build: g.Method, BuildReason: g.Why, Port: g.Port, HealthPath: g.Health, Env: g.Env}
		if g.Start != "" {
			svc.Command = []string{g.Start}
		}
		return svc, envs, nil
	}
	if deps.Detect != nil {
		if provider, err := deps.Detect(ctx, repoURL, ref); err == nil && provider != "" {
			return ServicePlan{Build: BuildRailpack, BuildReason: "Railpack detected a " + provider + " project", Port: defaultProviderPort(provider), HealthPath: "/"}, envs, nil
		}
	}
	return ServicePlan{Build: BuildUnknown}, envs, nil
}

func hasBuildService(p *DeploymentPlan) bool {
	for _, s := range p.Services {
		if s.Build != BuildImage {
			return true
		}
	}
	return false
}

func defaultProviderPort(provider string) int {
	switch provider {
	case "golang", "java":
		return 8080
	case "python":
		return 8000
	case "php":
		return 80
	default:
		return 3000
	}
}

// guessFramework inspects manifest files, in a fixed order, to pick a stack.
func guessFramework(read func(string) (string, bool, error)) (frameworkGuess, bool) {
	if text, ok, _ := read("package.json"); ok {
		var pj packageJSON
		if err := json.Unmarshal([]byte(text), &pj); err == nil {
			return guessNode(pj), true
		}
		return frameworkGuess{Name: "Node.js", Method: BuildRailpack, Port: 3000, Health: "/", Why: "package.json found (unparseable), Railpack will build it"}, true
	}
	if _, ok, _ := read("go.mod"); ok {
		return frameworkGuess{Name: "Go", Method: BuildRailpack, Port: 8080, Health: "/", Why: "go.mod found, built with Railpack"}, true
	}
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile"} {
		if text, ok, _ := read(f); ok {
			return guessPython(strings.ToLower(text)), true
		}
	}
	for _, f := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		if _, ok, _ := read(f); ok {
			return frameworkGuess{Name: "Java", Method: BuildRailpack, Port: 8080, Health: "/actuator/health", Why: f + " found, built with Railpack"}, true
		}
	}
	if text, ok, _ := read("Gemfile"); ok {
		return guessRuby(text, read), true
	}
	if text, ok, _ := read("composer.json"); ok {
		return guessPHP(text, read), true
	}
	if _, ok, _ := read("index.html"); ok {
		return frameworkGuess{Name: "Static site", Method: BuildStatic, Port: 0, Health: "/", Why: "index.html at the repository root, served as a static site"}, true
	}
	return frameworkGuess{}, false
}

func guessNode(pj packageJSON) frameworkGuess {
	has := func(dep string) bool {
		_, a := pj.Dependencies[dep]
		_, b := pj.DevDependencies[dep]
		return a || b
	}
	start := pj.Scripts["start"]
	switch {
	case has("next"):
		return frameworkGuess{Name: "Next.js", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "next dependency found, built with Railpack"}
	case has("nuxt"):
		return frameworkGuess{Name: "Nuxt", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "nuxt dependency found, built with Railpack"}
	case has("@nestjs/core"):
		return frameworkGuess{Name: "NestJS", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "NestJS dependency found, built with Railpack"}
	case has("express") || has("fastify") || has("koa") || has("hono"):
		return frameworkGuess{Name: "Node.js server", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "Node web server dependency found, built with Railpack"}
	case has("vite") || has("react-scripts") || has("astro") || has("svelte"):
		return frameworkGuess{Name: "Static frontend", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "frontend build tool found, built with Railpack"}
	default:
		return frameworkGuess{Name: "Node.js", Method: BuildRailpack, Port: 3000, Health: "/", Start: start, Why: "package.json found, built with Railpack"}
	}
}

func guessPython(text string) frameworkGuess {
	switch {
	case strings.Contains(text, "django"):
		return frameworkGuess{Name: "Django", Method: BuildRailpack, Port: 8000, Health: "/", Why: "Django dependency found, built with Railpack"}
	case strings.Contains(text, "fastapi"):
		return frameworkGuess{Name: "FastAPI", Method: BuildRailpack, Port: 8000, Health: "/docs", Why: "FastAPI dependency found, built with Railpack"}
	case strings.Contains(text, "flask"):
		return frameworkGuess{Name: "Flask", Method: BuildRailpack, Port: 5000, Health: "/", Why: "Flask dependency found, built with Railpack"}
	}
	return frameworkGuess{Name: "Python", Method: BuildRailpack, Port: 8000, Health: "/", Why: "Python manifest found, built with Railpack"}
}

var (
	gemRe      = regexp.MustCompile(`(?m)^\s*gem\s+["']([\w-]+)["']`)
	portFlagRe = regexp.MustCompile(`(?:--port[= ]|-p\s*)(\d+)|PORT(?:=|:-)(\d+)`)
	pumaPortRe = regexp.MustCompile(`(?m)^\s*port\s+(?:ENV\.fetch\(\s*["']PORT["']\s*(?:,\s*|\)\s*\{\s*)|ENV\[["']PORT["']\]\s*\|\|\s*)?(\d+)`)
	procWebRe  = regexp.MustCompile(`(?m)^web\s*:\s*(.+)$`)
)

// Railpack's ruby start binds ${PORT:-3000} and its php image serves
// Caddy on :${PORT:-80}; the platform does not inject PORT.
func portFromCommand(cmd string) int {
	m := portFlagRe.FindStringSubmatch(cmd)
	for _, g := range m[min(1, len(m)):] {
		if n, err := strconv.Atoi(g); err == nil && n > 0 && n <= 65535 {
			return n
		}
	}
	return 0
}

func procfileWeb(read func(string) (string, bool, error)) string {
	if text, ok, _ := read("Procfile"); ok {
		if m := procWebRe.FindStringSubmatch(text); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func guessRuby(gemfile string, read func(string) (string, bool, error)) frameworkGuess {
	gems := map[string]bool{}
	for _, m := range gemRe.FindAllStringSubmatch(gemfile, -1) {
		gems[m[1]] = true
	}
	g := frameworkGuess{Name: "Ruby", Method: BuildRailpack, Port: 3000, Health: "/", Why: "Gemfile found, built with Railpack"}
	switch {
	case gems["rails"]:
		g.Name, g.Health, g.Why = "Rails", "/up", "rails gem found, built with Railpack"
		// Railpack's rails start passes -e $RAILS_ENV and never sets it.
		g.Env = []EnvVar{
			{Key: "RAILS_ENV", Value: "production", HasDefault: true, Source: "rails start command"},
			{Key: "RAILS_SERVE_STATIC_FILES", Value: "true", HasDefault: true, Source: "rails start command"},
			{Key: "SECRET_KEY_BASE", Required: true, Secret: true, Source: "rails production boot"},
		}
		if text, ok, _ := read("config/puma.rb"); ok {
			if m := pumaPortRe.FindStringSubmatch(text); len(m) > 1 {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 && n <= 65535 {
					g.Port = n
				}
			}
		}
	case gems["sinatra"]:
		g.Name, g.Why = "Sinatra", "sinatra gem found, built with Railpack"
	default:
		if _, ok, _ := read("config.ru"); ok {
			g.Name, g.Why = "Rack", "Gemfile and config.ru found, built with Railpack"
		}
	}
	if cmd := procfileWeb(read); cmd != "" {
		if n := portFromCommand(cmd); n > 0 {
			g.Port = n
		}
	}
	return g
}

func guessPHP(composer string, read func(string) (string, bool, error)) frameworkGuess {
	g := frameworkGuess{Name: "PHP", Method: BuildRailpack, Port: 80, Health: "/", Why: "composer.json found, built with Railpack"}
	_, hasArtisan, _ := read("artisan")
	if strings.Contains(composer, "laravel/framework") || hasArtisan {
		g.Name, g.Why = "Laravel", "laravel/framework found, built with Railpack"
		g.Env = []EnvVar{{Key: "APP_KEY", Required: true, Secret: true, Source: "laravel boot"}}
	}
	if n := portFromCommand(procfileWeb(read)); n > 0 {
		g.Port = n
	}
	return g
}
