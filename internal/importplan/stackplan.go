package importplan

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/glincker/levelrail/internal/compose"
)

// Resource kinds a plan lists as "will be created".
const (
	ResourceApp      = "app"
	ResourceDatabase = "database"
	ResourceVolume   = "volume"
	ResourceDomain   = "domain"
)

// PlannedResource is one thing the import would create.
type PlannedResource struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Detail string `json:"detail,omitempty"`
}

// Unsupported is a compose feature the import cannot carry over.
type Unsupported struct {
	Service     string `json:"service,omitempty"`
	Feature     string `json:"feature"`
	Reason      string `json:"reason"`
	Alternative string `json:"alternative,omitempty"`
}

// Conflict is a planned resource whose name is already taken.
type Conflict struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

var databaseImages = map[string]bool{
	"postgres": true, "mysql": true, "mariadb": true, "mongo": true, "redis": true,
	"valkey": true, "memcached": true, "clickhouse": true, "timescaledb": true,
}

var (
	hostRuleRe      = regexp.MustCompile("Host\\(`([^`]+)`\\)")
	hostNameRe      = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	unsupportedKeys = []struct{ key, reason, alt string }{
		{"privileged", "privileged containers are not run", "grant only the specific capabilities the service needs"},
		{"cap_add", "extra Linux capabilities are not applied", "use an image that runs without added capabilities"},
		{"devices", "host device passthrough is not available", "run the service on a node that exposes the device, outside this import"},
		{"pid", "sharing the host process namespace is not allowed", "remove it, or run the service outside the stack"},
		{"ipc", "sharing the host IPC namespace is not allowed", "remove it, or set shm_size instead"},
		{"security_opt", "custom security options are not applied", "rely on the platform defaults"},
		{"sysctls", "kernel parameter overrides are not applied", "tune the host or use an image default"},
		{"env_file", "env files on disk are not read by a pasted stack", "pass the values with --env or add them as environment entries"},
		{"extends", "extends needs the referenced files", "inline the extended service"},
		{"network_mode", "this network mode is not supported", "publish the required ports and use the stack network"},
	}
)

// enrichStack fills the resource, secret reference and unsupported lists of a
// compose plan. It only reads the document and never echoes a secret value.
func enrichStack(p *DeploymentPlan, data []byte) {
	f, err := compose.Parse(data)
	if err != nil {
		return
	}
	appName := p.SuggestedName
	if appName == "" {
		appName = "app"
	}
	p.Resources = append(p.Resources, PlannedResource{Kind: ResourceApp, Name: appName, Detail: serviceCount(len(f.Services))})
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	seenVol, seenDomain := map[string]bool{}, map[string]bool{}
	for _, n := range names {
		s := f.Services[n]
		if s.Image != "" {
			if repo, _, _ := splitImage(s.Image); databaseImages[imageBaseName(repo)] {
				p.Resources = append(p.Resources, PlannedResource{Kind: ResourceDatabase, Name: n, Detail: "image " + s.Image})
			}
		}
		for _, v := range s.Volumes {
			if v.Name != "" && !seenVol[v.Name] {
				seenVol[v.Name] = true
				p.Resources = append(p.Resources, PlannedResource{Kind: ResourceVolume, Name: v.Name, Detail: "used by " + n})
			}
		}
		for _, h := range serviceDomains(s) {
			if !seenDomain[h] {
				seenDomain[h] = true
				p.Resources = append(p.Resources, PlannedResource{Kind: ResourceDomain, Name: h, Detail: "from " + n})
			}
		}
		for _, e := range p.serviceEnv(n) {
			if e.Secret {
				p.SecretRefs = append(p.SecretRefs, n+"/"+e.Key)
			}
		}
	}
	p.Unsupported = append(p.Unsupported, scanUnsupported(data)...)
}

func (p *DeploymentPlan) serviceEnv(name string) []EnvVar {
	for _, s := range p.Services {
		if s.Name == name {
			return s.Env
		}
	}
	return nil
}

func serviceCount(n int) string {
	if n == 1 {
		return "1 service"
	}
	return strconv.Itoa(n) + " services"
}

// serviceDomains reads reverse proxy Host rules from labels and VIRTUAL_HOST.
func serviceDomains(s compose.Service) []string {
	var out []string
	for _, v := range s.Labels {
		for _, m := range hostRuleRe.FindAllStringSubmatch(v, -1) {
			out = append(out, strings.ToLower(m[1]))
		}
	}
	if v, ok := s.Environment["VIRTUAL_HOST"]; ok {
		for _, h := range strings.Split(v, ",") {
			if h = strings.ToLower(strings.TrimSpace(h)); hostNameRe.MatchString(h) {
				out = append(out, h)
			}
		}
	}
	sort.Strings(out)
	return out
}

// scanUnsupported reads the raw document, because the typed parser drops or
// rejects keys the import cannot honor.
func scanUnsupported(data []byte) []Unsupported {
	var doc map[string]any
	if yaml.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []Unsupported
	for _, top := range []string{"secrets", "configs"} {
		if _, ok := doc[top]; ok {
			out = append(out, Unsupported{Feature: top, Reason: "file based " + top + " are not read from a pasted stack", Alternative: "create Levelrail secrets and reference them as environment entries"})
		}
	}
	services, _ := doc["services"].(map[string]any)
	names := make([]string, 0, len(services))
	for n := range services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		svc, _ := services[n].(map[string]any)
		for _, u := range unsupportedKeys {
			v, ok := svc[u.key]
			if !ok || (u.key == "privileged" && v != true) {
				continue
			}
			if u.key == "network_mode" && v != "host" {
				continue
			}
			out = append(out, Unsupported{Service: n, Feature: u.key, Reason: u.reason, Alternative: u.alt})
		}
		if _, ok := svc["build"]; ok {
			out = append(out, Unsupported{Service: n, Feature: "build", Reason: "a pasted stack has no source to build from", Alternative: "import the repository instead, or push the image and reference it"})
		}
	}
	return out
}

// MarkConflicts records planned resources whose name is already in use.
// existingApps holds app names the caller can see.
func (p *DeploymentPlan) MarkConflicts(existingApps map[string]bool) {
	for _, r := range p.Resources {
		if r.Kind == ResourceApp && existingApps[r.Name] {
			p.Conflicts = append(p.Conflicts, Conflict{Kind: r.Kind, Name: r.Name})
		}
	}
}
