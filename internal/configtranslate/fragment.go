package configtranslate

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// Fragment is the proposed app.yaml content, mirroring internal/spec field names.
type Fragment struct {
	Version   int                 `yaml:"version" json:"version"`
	Services  map[string]Service  `yaml:"services,omitempty" json:"services,omitempty"`
	Databases map[string]Database `yaml:"databases,omitempty" json:"databases,omitempty"`
}

// Service is one proposed services: entry.
type Service struct {
	Build     Build          `yaml:"build" json:"build"`
	Port      int            `yaml:"port,omitempty" json:"port,omitempty"`
	Health    *Health        `yaml:"health,omitempty" json:"health,omitempty"`
	Resources *Resources     `yaml:"resources,omitempty" json:"resources,omitempty"`
	Replicas  int            `yaml:"replicas,omitempty" json:"replicas,omitempty"`
	Command   []string       `yaml:"command,omitempty" json:"command,omitempty"`
	Hooks     *Hooks         `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Jobs      []Job          `yaml:"jobs,omitempty" json:"jobs,omitempty"`
	Routes    []Route        `yaml:"routes,omitempty" json:"routes,omitempty"`
	Headers   []HeaderRule   `yaml:"headers,omitempty" json:"headers,omitempty"`
	Static    *StaticOptions `yaml:"static,omitempty" json:"static,omitempty"`
	Env       map[string]Env `yaml:"-" json:"env,omitempty"`
}

// Build is a service's build block.
type Build struct {
	Type    string `yaml:"type" json:"type"`
	Command string `yaml:"command,omitempty" json:"command,omitempty"`
	Publish string `yaml:"publish,omitempty" json:"publish,omitempty"`
}

// Health holds the readiness probe proposal.
type Health struct {
	Readiness *Probe `yaml:"readiness,omitempty" json:"readiness,omitempty"`
}

// Probe is an HTTP readiness check path.
type Probe struct {
	Path string `yaml:"path" json:"path"`
}

// Resources holds proposed container limits.
type Resources struct {
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
}

// Hooks are deploy-lifecycle shell commands run inside the container.
type Hooks struct {
	PreDeploy  string `yaml:"preDeploy,omitempty" json:"preDeploy,omitempty"`
	PostDeploy string `yaml:"postDeploy,omitempty" json:"postDeploy,omitempty"`
}

// Job is one proposed scheduled task under a service's jobs:.
type Job struct {
	Name     string   `yaml:"name" json:"name"`
	Schedule string   `yaml:"schedule" json:"schedule"`
	Command  []string `yaml:"command" json:"command"`
	Timezone string   `yaml:"timezone,omitempty" json:"timezone,omitempty"`
	Mode     string   `yaml:"mode,omitempty" json:"mode,omitempty"`
}

// Route is a static-site redirect or rewrite rule.
type Route struct {
	From   string `yaml:"from" json:"from"`
	To     string `yaml:"to" json:"to"`
	Status int    `yaml:"status,omitempty" json:"status,omitempty"`
}

// HeaderRule sets response headers on a static site for a path pattern.
type HeaderRule struct {
	Path   string            `yaml:"path" json:"path"`
	Values map[string]string `yaml:"values,omitempty" json:"values,omitempty"`
}

// StaticOptions are a static service's serving options.
type StaticOptions struct {
	CleanURLs     bool   `yaml:"cleanUrls,omitempty" json:"cleanUrls,omitempty"`
	TrailingSlash string `yaml:"trailingSlash,omitempty" json:"trailingSlash,omitempty"`
}

// Database is one proposed databases: entry.
type Database struct {
	Engine string `yaml:"engine" json:"engine"`
}

// Env is one env value: a literal, a reference to a database, or a secret
// the operator supplies at deploy time.
type Env struct {
	Value    string `json:"value,omitempty"`
	From     string `json:"from,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
	Required bool   `json:"required,omitempty"`
}

func (e Env) node() (*yaml.Node, error) {
	if e.From == "" && !e.Secret {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: e.Value}, nil
	}
	obj := map[string]any{}
	if e.From != "" {
		obj["from"] = e.From
	}
	if e.Secret {
		obj["secret"] = true
	}
	if e.Required {
		obj["required"] = true
	}
	var n yaml.Node
	if err := n.Encode(obj); err != nil {
		return nil, fmt.Errorf("configtranslate: encode env: %w", err)
	}
	n.Style = yaml.FlowStyle
	return &n, nil
}

// merge adds other into f, keeping f's entry on a name clash.
func (f *Fragment) merge(other Fragment) []Finding {
	var fs []Finding
	if f.Services == nil && len(other.Services) > 0 {
		f.Services = map[string]Service{}
	}
	if f.Databases == nil && len(other.Databases) > 0 {
		f.Databases = map[string]Database{}
	}
	for _, k := range sortedKeys(other.Services) {
		if _, dup := f.Services[k]; dup {
			fs = append(fs, Finding{Code: "merge.service-clash", Severity: SeverityWarn, Source: "vercel.json", Subject: k,
				Message: "both Heroku and Vercel config define this service, the first one is kept",
				Fix:     "Pick one source of truth for this service and rename the other."})
			continue
		}
		f.Services[k] = other.Services[k]
	}
	for _, k := range sortedKeys(other.Databases) {
		if _, dup := f.Databases[k]; !dup {
			f.Databases[k] = other.Databases[k]
		}
	}
	return fs
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
