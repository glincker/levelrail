package importplan

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func planCompose(in Input) (*DeploymentPlan, error) {
	p, err := composePlanFromText([]byte(in.Text), "compose file")
	if err != nil {
		return nil, err
	}
	p.Source = SourceCompose
	if len(in.Env) > 0 {
		out, err := injectComposeEnv([]byte(in.Text), in.Env)
		if err != nil {
			return nil, err
		}
		p.ComposeYAML = out
	}
	return p, nil
}

// composePlanFromText parses and validates compose text into per-service plans.
func composePlanFromText(data []byte, source string) (*DeploymentPlan, error) {
	f, err := compose.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("importplan: %w", err)
	}
	p := &DeploymentPlan{Deploy: DeployCompose, ComposeYAML: string(data), Warnings: []Warning{}}
	if verr := f.Validate(); verr != nil {
		p.Deploy = DeployNone
		for _, line := range strings.Split(verr.Error(), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				p.Warnings = append(p.Warnings, Warning{Code: "compose_invalid", Message: line})
			}
		}
	}
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p.Services = append(p.Services, servicePlanFromCompose(n, f.Services[n], source, &p.Warnings))
	}
	if len(names) == 1 {
		p.SuggestedName = slugify(names[0])
	} else if len(names) > 1 {
		p.SuggestedName = "stack"
	}
	if len(names) == 0 {
		p.Deploy = DeployNone
	}
	return p, nil
}

func servicePlanFromCompose(name string, s compose.Service, source string, warns *[]Warning) ServicePlan {
	sp := ServicePlan{Name: name, Image: s.Image, Build: BuildImage, BuildReason: "image from " + source, Restart: s.Restart, Command: s.Command, Entrypoint: s.Entrypoint}
	if s.Build != nil {
		sp.Build = BuildDockerfile
		sp.BuildReason = "service has a build: block"
		*warns = append(*warns, Warning{Code: "compose_build", Message: fmt.Sprintf("service %q builds from source, which a pasted compose file cannot do; import its repository instead", name)})
	}
	for _, pt := range s.Ports {
		sp.Ports = append(sp.Ports, PortMapping{Host: pt.HostPort, Container: pt.ContainerPort})
	}
	if len(s.Ports) > 0 {
		sp.Port = s.Ports[0].ContainerPort
	}
	for _, v := range s.Volumes {
		sp.Volumes = append(sp.Volumes, VolumeMount{Name: v.Name, HostPath: v.HostPath, ContainerPath: v.ContainerPath, ReadOnly: v.ReadOnly, NeedsApproval: v.HostPath != ""})
	}
	keys := make([]string, 0, len(s.Environment))
	for k := range s.Environment {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sp.Env = append(sp.Env, composeEnvVar(k, s.Environment[k]))
	}
	if s.Image != "" {
		for _, w := range imageWarnings(s.Image) {
			w.Message = name + ": " + w.Message
			*warns = append(*warns, w)
		}
	}
	return sp
}

// composeEnvVar reads ${VAR}, ${VAR:-default} and ${VAR:?err} references.
func composeEnvVar(key, value string) EnvVar {
	if strings.HasPrefix(value, "${") && strings.HasSuffix(value, "}") {
		inner := value[2 : len(value)-1]
		if _, def, ok := strings.Cut(inner, ":-"); ok {
			return newEnvVar(key, def, "compose environment")
		}
		e := newEnvVar(key, "", "compose environment")
		return e
	}
	return newEnvVar(key, value, "compose environment")
}

// injectComposeEnv sets supplied values on every service environment that already declares the key.
func injectComposeEnv(data []byte, env map[string]string) (string, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("importplan: reparse compose: %w", err)
	}
	services, _ := doc["services"].(map[string]any)
	for _, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		switch e := svc["environment"].(type) {
		case map[string]any:
			for k := range e {
				if v, ok := env[k]; ok {
					e[k] = v
				}
			}
		case []any:
			for i, item := range e {
				s, _ := item.(string)
				k, _, _ := strings.Cut(s, "=")
				if v, ok := env[k]; ok {
					e[i] = k + "=" + v
				}
			}
		}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("importplan: render compose: %w", err)
	}
	return string(out), nil
}

type genService struct {
	Image       string            `yaml:"image"`
	Restart     string            `yaml:"restart,omitempty"`
	Entrypoint  []string          `yaml:"entrypoint,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
	Ports       []string          `yaml:"ports,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Volumes     []string          `yaml:"volumes,omitempty"`
}

type genFile struct {
	Services map[string]genService `yaml:"services"`
	Volumes  map[string]struct{}   `yaml:"volumes,omitempty"`
}

// generateCompose renders services as a compose document. Env values come
// from supplied first, then non-secret defaults. Bind mounts are emitted as
// is; the compose endpoint gates them behind root.
func generateCompose(services []ServicePlan, supplied map[string]string) string {
	f := genFile{Services: map[string]genService{}}
	for _, s := range services {
		g := genService{Image: s.Image, Restart: s.Restart, Entrypoint: s.Entrypoint, Command: s.Command}
		for _, p := range s.Ports {
			if p.Host > 0 {
				g.Ports = append(g.Ports, fmt.Sprintf("%d:%d", p.Host, p.Container))
			} else {
				g.Ports = append(g.Ports, fmt.Sprintf("%d", p.Container))
			}
		}
		for _, e := range s.Env {
			v, ok := supplied[e.Key]
			if !ok {
				v = e.Value
			}
			if v == "" {
				continue
			}
			if g.Environment == nil {
				g.Environment = map[string]string{}
			}
			g.Environment[e.Key] = v
		}
		for _, v := range s.Volumes {
			src := v.Name
			if v.HostPath != "" {
				src = v.HostPath
			} else if src != "" {
				if f.Volumes == nil {
					f.Volumes = map[string]struct{}{}
				}
				f.Volumes[src] = struct{}{}
			}
			if src == "" {
				continue
			}
			spec := src + ":" + v.ContainerPath
			if v.ReadOnly {
				spec += ":ro"
			}
			g.Volumes = append(g.Volumes, spec)
		}
		f.Services[s.Name] = g
	}
	out, err := yaml.Marshal(f)
	if err != nil {
		return ""
	}
	return string(out)
}
