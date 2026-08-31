package compose

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type rawFile struct {
	Version  string                `yaml:"version"`
	Services map[string]rawService `yaml:"services"`
	Domains  map[string]string     `yaml:"x-levelrail-domains"`
	// Networks is top-level networks:, decoded down to just its keys
	// (a network's driver/config body decodes into the zero-field
	// struct{} and is discarded): Notices only needs to know whether
	// custom networks were declared at all, not their configuration.
	Networks map[string]struct{} `yaml:"networks"`
}

type rawService struct {
	Image       string            `yaml:"image"`
	Build       *rawBuild         `yaml:"build"`
	Environment Environment       `yaml:"environment"`
	Ports       []Port            `yaml:"ports"`
	Volumes     []Volume          `yaml:"volumes"`
	Labels      map[string]string `yaml:"labels"`
	Networks    Networks          `yaml:"networks"`
	Restart     string            `yaml:"restart"`
	Healthcheck *Healthcheck      `yaml:"healthcheck"`
}

// Healthcheck is one service's healthcheck: block, Docker Compose's own
// command-based health check schema (test/interval/timeout/retries/
// start_period). Never executed: this platform's own health model is
// HTTP-path based, so resolveHealthcheck (healthcheck.go) extracts a
// readiness path from a curl/wget test when it can, and leaves health
// unset otherwise.
type Healthcheck struct {
	Test        healthcheckTest `yaml:"test"`
	Interval    string          `yaml:"interval"`
	Timeout     string          `yaml:"timeout"`
	Retries     int             `yaml:"retries"`
	StartPeriod string          `yaml:"start_period"`
}

// healthcheckTest is healthcheck.test's string-or-list union: a bare
// string is Compose's own shorthand for ["CMD-SHELL", "<string>"].
type healthcheckTest []string

// UnmarshalYAML implements the union described above.
func (t *healthcheckTest) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*t = []string{"CMD-SHELL", node.Value}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return fmt.Errorf("healthcheck: test: %w", err)
		}
		*t = items
		return nil
	default:
		return fmt.Errorf("healthcheck: test: must be a string or a list")
	}
}

// Environment is environment:'s string-or-list union: a KEY: VALUE map,
// or a list of "KEY=VALUE" strings (a bare "KEY" means "inherit from
// the host shell" in real Compose, decoded here as an empty value
// since there's no host shell to inherit from).
type Environment map[string]string

// UnmarshalYAML implements the map-or-list union described above.
func (e *Environment) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("environment: %w", err)
		}
		*e = m
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return fmt.Errorf("environment: %w", err)
		}
		m := make(map[string]string, len(items))
		for _, item := range items {
			key, value, _ := strings.Cut(item, "=")
			m[key] = value
		}
		*e = m
		return nil
	default:
		return fmt.Errorf("environment: must be a mapping or a list of KEY=VALUE strings")
	}
}

// Networks is a service's own networks:'s list-or-map union: a plain
// list of network names, or a map of name to per-network config
// (aliases, ipv4_address, ...) this decodes down to just the name,
// same reasoning as rawFile.Networks above.
type Networks []string

// UnmarshalYAML implements the list-or-map union described above.
func (n *Networks) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return fmt.Errorf("networks: %w", err)
		}
		*n = items
		return nil
	case yaml.MappingNode:
		names := make([]string, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			names = append(names, node.Content[i].Value)
		}
		*n = names
		return nil
	default:
		return fmt.Errorf("networks: must be a list or map of network names")
	}
}

// UnmarshalYAML supports ports:'s short form only: "container" or
// "host:container". Long-form mapping entries are rejected rather than
// silently dropped.
func (p *Port) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("ports: long-form entries are not supported, use \"container\" or \"host:container\"")
	}
	raw := node.Value

	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 1:
		port, err := parsePort(parts[0])
		if err != nil {
			return fmt.Errorf("ports: %w", err)
		}
		p.ContainerPort = port
		return nil
	case 2:
		host, err := parsePort(parts[0])
		if err != nil {
			return fmt.Errorf("ports: host port: %w", err)
		}
		container, err := parsePort(parts[1])
		if err != nil {
			return fmt.Errorf("ports: container port: %w", err)
		}
		p.HostPort = host
		p.ContainerPort = container
		return nil
	default:
		return fmt.Errorf("ports: %q: an IP-bound form (\"ip:host:container\") is not supported", raw)
	}
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid port number", s)
	}
	if n <= 0 || n > 65535 {
		return 0, fmt.Errorf("%q is out of range for a port number", s)
	}
	return n, nil
}

// UnmarshalYAML supports volumes:'s short form only: "name:/path",
// optionally with a trailing ":ro"/":rw" this parses but doesn't use.
// A path-like "name" (bind mount) decodes with an empty Name so
// Validate reports it, rather than failing the whole parse here.
func (v *Volume) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("volumes: long-form entries are not supported, use \"name:/path\"")
	}
	parts := strings.SplitN(node.Value, ":", 3)
	if len(parts) < 2 {
		return fmt.Errorf("volumes: %q must be \"name:/path\"", node.Value)
	}
	name, path := parts[0], parts[1]
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "/") {
		name = ""
	}
	v.Name = name
	v.ContainerPath = path
	return nil
}

// UnmarshalYAML supports build:'s string-or-object union, just enough
// to detect it's present; Validate rejects build: outright.
func (b *rawBuild) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		b.Context = node.Value
		return nil
	}
	var obj struct {
		Context    string `yaml:"context"`
		Dockerfile string `yaml:"dockerfile"`
	}
	if err := node.Decode(&obj); err != nil {
		return fmt.Errorf("build: %w", err)
	}
	b.Context, b.Dockerfile = obj.Context, obj.Dockerfile
	return nil
}
