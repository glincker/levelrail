package importplan

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
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
		cp, cerr := composePlanFromText([]byte(text), cf)
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
		svc := ServicePlan{Build: g.Method, BuildReason: g.Why, Port: g.Port, HealthPath: g.Health}
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
