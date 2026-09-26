package importplan

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ErrEmptyInput is returned for blank input.
var ErrEmptyInput = errors.New("importplan: input is empty")

// ErrUnclassified is returned when the input matches no known source shape.
var ErrUnclassified = errors.New("importplan: could not tell what this input is")

// MaxInputBytes caps Input.Text, compose files and Dockerfiles included.
const MaxInputBytes = 512 * 1024

var (
	imageRefRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]*(:[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(@sha256:[a-f0-9]{64})?$`)
	scpGitRe    = regexp.MustCompile(`^git@([a-z0-9.-]+):(.+?)(\.git)?$`)
	dockerfileR = regexp.MustCompile(`(?im)^\s*FROM\s+\S+`)
	composeR    = regexp.MustCompile(`(?m)^services:\s*$`)
	repoHostRe  = regexp.MustCompile(`^(github\.com|gitlab\.com|bitbucket\.org|codeberg\.org)/[^/\s]+/[^/\s]+`)
)

// Deps are the outside effects Plan may use; every field is optional.
type Deps struct {
	// Files reads repository files; nil disables repo inspection.
	Files FileSource
	// Detect returns a Railpack provider id for a repo; nil skips the fallback.
	Detect func(ctx context.Context, repoURL, ref string) (string, error)
}

// Classify decides what kind of input text is.
func Classify(in Input) (SourceKind, error) {
	if in.Kind != "" {
		return in.Kind, nil
	}
	t := strings.TrimSpace(in.Text)
	if t == "" {
		return "", ErrEmptyInput
	}
	first := strings.Fields(t)
	if len(first) >= 2 {
		i := 0
		if first[0] == "sudo" {
			i++
		}
		if first[i] == "docker" && i+1 < len(first) && (first[i+1] == "run" || (first[i+1] == "container" && i+2 < len(first) && first[i+2] == "run")) {
			return SourceDockerRun, nil
		}
	}
	if composeR.MatchString(t) {
		return SourceCompose, nil
	}
	if dockerfileR.MatchString(t) && (strings.Contains(t, "\n") || strings.HasPrefix(strings.ToUpper(t), "FROM ")) {
		return SourceDockerfile, nil
	}
	if strings.ContainsAny(t, " \t\n") {
		return "", ErrUnclassified
	}
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") || scpGitRe.MatchString(t) || repoHostRe.MatchString(t) {
		return SourceRepo, nil
	}
	if imageRefRe.MatchString(t) {
		return SourceImage, nil
	}
	return "", ErrUnclassified
}

// Plan builds the deployment plan preview. It performs no writes.
func Plan(ctx context.Context, in Input, deps Deps) (*DeploymentPlan, error) {
	if len(in.Text) > MaxInputBytes {
		return nil, fmt.Errorf("importplan: input is larger than %d bytes", MaxInputBytes)
	}
	kind, err := Classify(in)
	if err != nil {
		return nil, err
	}
	var p *DeploymentPlan
	switch kind {
	case SourceRepo:
		p, err = planRepo(ctx, in, deps)
	case SourceDockerRun:
		p, err = planDockerRun(in)
	case SourceImage:
		p, err = planImage(in)
	case SourceCompose:
		p, err = planCompose(in)
	case SourceDockerfile:
		p = planDockerfile(in)
	default:
		return nil, fmt.Errorf("importplan: unknown source kind %q", kind)
	}
	if err != nil {
		return nil, err
	}
	finalize(p, in)
	return p, nil
}

// finalize applies operator overrides, supplied env values and the missing-env list.
func finalize(p *DeploymentPlan, in Input) {
	if in.Name != "" {
		p.SuggestedName = in.Name
	}
	for i := range p.Services {
		s := &p.Services[i]
		if in.Port > 0 && len(p.Services) == 1 {
			s.Port = in.Port
		}
		for j := range s.Env {
			if v, ok := in.Env[s.Env[j].Key]; ok && v != "" {
				s.Env[j].Required = false
			}
		}
	}
	if p.Deploy == DeployCompose && p.Source == SourceDockerRun {
		p.ComposeYAML = generateCompose(p.Services, in.Env)
	}
	if p.Warnings == nil {
		p.Warnings = []Warning{}
	}
	p.MissingRequiredEnv = missingRequired(p.Services)
	if p.MissingRequiredEnv == nil {
		p.MissingRequiredEnv = []string{}
	}
}

// splitImage returns repository, tag and digest of an image reference.
func splitImage(ref string) (repo, tag, digest string) {
	if i := strings.Index(ref, "@"); i >= 0 {
		digest = ref[i+1:]
		ref = ref[:i]
	}
	last := ref[strings.LastIndex(ref, "/")+1:]
	if i := strings.LastIndex(last, ":"); i >= 0 {
		tag = last[i+1:]
		ref = ref[:len(ref)-len(last)+i]
	}
	return ref, tag, digest
}

func imageBaseName(ref string) string {
	repo, _, _ := splitImage(ref)
	name := repo[strings.LastIndex(repo, "/")+1:]
	return slugify(name)
}

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		return "app"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

// wellKnownPorts hints the container port of popular images.
var wellKnownPorts = map[string]int{
	"nginx": 80, "httpd": 80, "caddy": 80, "traefik": 80, "postgres": 5432, "redis": 6379,
	"mysql": 3306, "mariadb": 3306, "mongo": 27017, "minio": 9000, "portainer-ce": 9000,
	"n8n": 5678, "vaultwarden": 80, "jellyfin": 8096, "grafana": 3000, "ghost": 2368,
	"wordpress": 80, "uptime-kuma": 3001, "memcached": 11211, "rabbitmq": 5672,
}

func imageWarnings(ref string) []Warning {
	_, tag, digest := splitImage(ref)
	if digest == "" && (tag == "" || tag == "latest") {
		return []Warning{{Code: "unpinned_tag", Message: "image uses the latest tag, which can change between deploys; pin a version or digest for repeatable rollbacks"}}
	}
	return nil
}

func planImage(in Input) (*DeploymentPlan, error) {
	ref := strings.TrimSpace(in.Text)
	if !imageRefRe.MatchString(ref) {
		return nil, fmt.Errorf("importplan: %q is not a valid image reference", ref)
	}
	name := imageBaseName(ref)
	svc := ServicePlan{Name: name, Image: ref, Build: BuildImage, BuildReason: "prebuilt image reference, pulled as is"}
	p := &DeploymentPlan{Source: SourceImage, SuggestedName: name, Deploy: DeployApp, Services: []ServicePlan{svc}, Warnings: imageWarnings(ref)}
	if port, ok := wellKnownPorts[name]; ok {
		p.Services[0].Port = port
	} else {
		p.Warnings = append(p.Warnings, Warning{Code: "port_unknown", Message: "the container port is not known for this image; set it before deploying"})
	}
	return p, nil
}

func planDockerRun(in Input) (*DeploymentPlan, error) {
	dr, err := parseDockerRun(in.Text)
	if err != nil {
		return nil, err
	}
	name := dr.Name
	if name == "" {
		name = imageBaseName(dr.Image)
	}
	name = slugify(name)
	svc := ServicePlan{
		Name: name, Image: dr.Image, Build: BuildImage, BuildReason: "image from the docker run command",
		Ports: dr.Ports, Command: dr.Command, Entrypoint: dr.Entrypoint, Env: dr.Env,
		Volumes: dr.Volumes, Restart: dr.Restart, MemoryBytes: dr.Memory, NanoCPUs: dr.NanoCPUs,
	}
	warns := append(imageWarnings(dr.Image), dr.Warnings...)
	switch {
	case len(dr.Ports) > 0:
		svc.Port = dr.Ports[0].Container
	default:
		if port, ok := wellKnownPorts[imageBaseName(dr.Image)]; ok {
			svc.Port = port
		} else {
			warns = append(warns, Warning{Code: "port_unknown", Message: "no -p flag was given, so no container port is known; set one before deploying"})
		}
	}
	if len(dr.Ports) > 1 {
		warns = append(warns, Warning{Code: "extra_ports", Message: "only the first published port is routed through the ingress; other ports are published on the host"})
	}
	deploy := DeployApp
	if len(dr.Volumes) > 0 || len(dr.Command) > 0 || len(dr.Entrypoint) > 0 || dr.Restart != "" {
		deploy = DeployCompose
	}
	return &DeploymentPlan{Source: SourceDockerRun, SuggestedName: name, Deploy: deploy, Services: []ServicePlan{svc}, Warnings: warns}, nil
}

var (
	exposeRe = regexp.MustCompile(`(?im)^\s*EXPOSE\s+(.+)$`)
	envInstr = regexp.MustCompile(`(?im)^\s*ENV\s+(.+)$`)
	argInstr = regexp.MustCompile(`(?im)^\s*ARG\s+([A-Za-z_][A-Za-z0-9_]*)(?:=(.*))?$`)
)

// parseDockerfile extracts ports and ENV entries; the text is never executed.
func parseDockerfile(text string) (ports []int, env []EnvVar) {
	for _, m := range exposeRe.FindAllStringSubmatch(text, -1) {
		for _, f := range strings.Fields(m[1]) {
			f, _, _ = strings.Cut(f, "/")
			var n int
			if _, err := fmt.Sscanf(f, "%d", &n); err == nil && n > 0 && n < 65536 {
				ports = append(ports, n)
			}
		}
	}
	for _, m := range envInstr.FindAllStringSubmatch(text, -1) {
		env = append(env, parseEnvInstruction(m[1])...)
	}
	return ports, env
}

func parseEnvInstruction(rest string) []EnvVar {
	rest = strings.TrimSpace(rest)
	toks, err := Tokenize(rest)
	if err != nil || len(toks) == 0 {
		return nil
	}
	var out []EnvVar
	if strings.Contains(toks[0], "=") {
		for _, t := range toks {
			if k, v, ok := strings.Cut(t, "="); ok && envKeyRe.MatchString(k) {
				out = append(out, newEnvVar(k, v, "Dockerfile ENV"))
			}
		}
		return out
	}
	if envKeyRe.MatchString(toks[0]) {
		out = append(out, newEnvVar(toks[0], strings.Join(toks[1:], " "), "Dockerfile ENV"))
	}
	return out
}

func planDockerfile(in Input) *DeploymentPlan {
	ports, env := parseDockerfile(in.Text)
	svc := ServicePlan{Name: "app", Build: BuildDockerfile, BuildReason: "Dockerfile snippet", Env: mergeEnv(env)}
	if len(ports) > 0 {
		svc.Port = ports[0]
	}
	p := &DeploymentPlan{Source: SourceDockerfile, SuggestedName: "app", Deploy: DeployNone, Services: []ServicePlan{svc}}
	p.Warnings = append(p.Warnings, Warning{Code: "needs_repo", Message: "a Dockerfile alone has no build context; put it in a git repository and import the repository URL to deploy"})
	if len(ports) == 0 {
		p.Warnings = append(p.Warnings, Warning{Code: "port_unknown", Message: "the Dockerfile has no EXPOSE line"})
	}
	return p
}

func domainHint(name string) string {
	if name == "" {
		return ""
	}
	return name + ".example.com"
}

// repoParts parses a repo URL into a normalized https URL, owner and repo name.
func repoParts(raw string) (normalized, repo string, err error) {
	raw = strings.TrimSpace(raw)
	if m := scpGitRe.FindStringSubmatch(raw); m != nil {
		raw = "https://" + m[1] + "/" + m[2]
	} else if repoHostRe.MatchString(raw) {
		raw = "https://" + raw
	}
	u, perr := url.Parse(raw)
	if perr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", "", fmt.Errorf("importplan: repo URL must be http or https: %q", raw)
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segs) < 2 || segs[0] == "" || segs[1] == "" {
		return "", "", fmt.Errorf("importplan: repo URL needs an owner and a repository: %q", raw)
	}
	last := len(segs) - 1
	if !strings.Contains(strings.ToLower(u.Hostname()), "gitlab") {
		last = 1
	}
	repo = strings.TrimSuffix(segs[last], ".git")
	segs = append(segs[:last:last], repo)
	u.Path = "/" + strings.Join(segs, "/")
	return u.String(), repo, nil
}
