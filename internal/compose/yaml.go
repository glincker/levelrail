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
	DependsOn   DependsOn         `yaml:"depends_on"`
	Command     Command           `yaml:"command"`
	Entrypoint  Command           `yaml:"entrypoint"`
	PullPolicy  string            `yaml:"pull_policy"`
}

// Healthcheck is one service's healthcheck: block, Docker Compose's own
// command-based health check schema (test/interval/timeout/retries/
// start_period). Docker never runs it: resolveHealthcheck (healthcheck.go)
// translates it into this platform's own readiness probe.
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

// DependsOn is a service's own depends_on:'s list-or-map union, the same
// shape as Networks: either a plain list of service names, or a map of
// name to per-dependency config (condition, restart, ...) decoded down
// to just the names.
type DependsOn []string

// UnmarshalYAML implements the list-or-map union described above.
func (d *DependsOn) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return fmt.Errorf("depends_on: %w", err)
		}
		*d = items
		return nil
	case yaml.MappingNode:
		names := make([]string, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			names = append(names, node.Content[i].Value)
		}
		*d = names
		return nil
	default:
		return fmt.Errorf("depends_on: must be a list or map of service names")
	}
}

// UnmarshalYAML supports both of ports:'s forms: the short scalar form
// ("container" or "host:container") and the long mapping form
// (target/published/protocol/mode keys). mode: is accepted but ignored,
// since Levelrail has no host-vs-ingress publishing distinction to
// carry it onto.
func (p *Port) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		return p.parsePortShortForm(node.Value)
	case yaml.MappingNode:
		return p.parsePortLongForm(node)
	default:
		return fmt.Errorf("ports: entry must be a string (short form) or a mapping (long form)")
	}
}

func (p *Port) parsePortShortForm(raw string) error {
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

// rawLongPort is ports:'s long mapping form. Target and Published use
// yamlScalarString rather than int, since real Compose allows either
// spelling (target: 80 or target: "80") for both keys.
type rawLongPort struct {
	Target    yamlScalarString `yaml:"target"`
	Published yamlScalarString `yaml:"published"`
	Protocol  string           `yaml:"protocol"`
	Mode      string           `yaml:"mode"`
}

func (p *Port) parsePortLongForm(node *yaml.Node) error {
	var raw rawLongPort
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("ports: %w", err)
	}
	if raw.Target == "" {
		return fmt.Errorf("ports: target is required")
	}
	target, err := parsePort(string(raw.Target))
	if err != nil {
		return fmt.Errorf("ports: target: %w", err)
	}
	switch raw.Protocol {
	case "", "tcp":
	default:
		return fmt.Errorf("ports: target %d: protocol %q is not supported, only tcp is", target, raw.Protocol)
	}
	p.ContainerPort = target
	if raw.Published == "" {
		return nil
	}
	if strings.Contains(string(raw.Published), "-") {
		return fmt.Errorf("ports: target %d: a published port range is not supported, publish a single port", target)
	}
	host, err := parsePort(string(raw.Published))
	if err != nil {
		return fmt.Errorf("ports: target %d: published: %w", target, err)
	}
	p.HostPort = host
	return nil
}

// yamlScalarString decodes any YAML scalar (string or number) into its
// raw text, so a long-form key written as an unquoted number (target:
// 80) and one written as a string (target: "80") both parse the same.
type yamlScalarString string

func (s *yamlScalarString) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("must be a string or a number")
	}
	*s = yamlScalarString(node.Value)
	return nil
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

// UnmarshalYAML supports both of volumes:'s forms: the short scalar
// form ("name:/path", optionally with a trailing ":ro"/":rw") and the
// long mapping form (type/source/target/read_only keys). See
// parseVolumeShortForm and parseVolumeLongForm.
func (v *Volume) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		return v.parseVolumeShortForm(node.Value)
	case yaml.MappingNode:
		return v.parseVolumeLongForm(node)
	default:
		return fmt.Errorf("volumes: entry must be a string (short form) or a mapping (long form)")
	}
}

// parseVolumeShortForm parses "name:/path", optionally with a trailing
// ":ro"/":rw". The left side is a bind mount (HostPath set, Name left
// empty) when it starts with "/", a real Docker Compose absolute host
// path; one starting with "." is rejected outright, since there's no
// defined working directory here to resolve a relative path against
// (real Compose resolves it against the compose file's own directory,
// which this package's direct-import path, unlike the git-sourced
// expand path, doesn't have). Anything else is a named volume,
// unchanged from before bind mounts existed.
func (v *Volume) parseVolumeShortForm(raw string) error {
	parts := strings.SplitN(raw, ":", 3)
	if len(parts) < 2 {
		return fmt.Errorf("volumes: %q must be \"name:/path\"", raw)
	}
	left, path := parts[0], parts[1]
	if strings.HasPrefix(left, ".") {
		return fmt.Errorf("volumes: %q: relative bind-mount paths are not supported, use an absolute path", raw)
	}
	if strings.HasPrefix(left, "/") {
		v.HostPath = left
	} else {
		v.Name = left
	}
	v.ContainerPath = path
	if len(parts) == 3 {
		v.ReadOnly = parts[2] == "ro"
	}
	return nil
}

// rawLongVolume is volumes:'s long mapping form. tmpfs and npipe
// (real Compose's other two type: values) have no equivalent in
// store.ServiceVolume/store.ServiceBindMount, so parseVolumeLongForm
// rejects them rather than silently dropping the mount.
type rawLongVolume struct {
	Type     string `yaml:"type"`
	Source   string `yaml:"source"`
	Target   string `yaml:"target"`
	ReadOnly bool   `yaml:"read_only"`
}

func (v *Volume) parseVolumeLongForm(node *yaml.Node) error {
	var raw rawLongVolume
	if err := node.Decode(&raw); err != nil {
		return fmt.Errorf("volumes: %w", err)
	}
	if raw.Target == "" {
		return fmt.Errorf("volumes: target is required")
	}
	switch raw.Type {
	case "", "volume":
		if raw.Source == "" {
			return fmt.Errorf("volumes: target %q must be a named volume (set source:) or use type: bind with an absolute source path", raw.Target)
		}
		if strings.HasPrefix(raw.Source, "/") || strings.HasPrefix(raw.Source, ".") {
			return fmt.Errorf("volumes: target %q: source %q looks like a path, set type: bind instead", raw.Target, raw.Source)
		}
		v.Name = raw.Source
	case "bind":
		if raw.Source == "" {
			return fmt.Errorf("volumes: target %q: type: bind requires source", raw.Target)
		}
		if strings.HasPrefix(raw.Source, ".") {
			return fmt.Errorf("volumes: target %q: relative bind-mount paths are not supported, use an absolute path", raw.Target)
		}
		if !strings.HasPrefix(raw.Source, "/") {
			return fmt.Errorf("volumes: target %q: type: bind requires an absolute source path", raw.Target)
		}
		v.HostPath = raw.Source
	default:
		return fmt.Errorf("volumes: target %q: type: %q is not supported, use \"volume\" or \"bind\"", raw.Target, raw.Type)
	}
	v.ContainerPath = raw.Target
	v.ReadOnly = raw.ReadOnly
	return nil
}

// Command is command:'s and entrypoint:'s shared string-or-list union: a
// list passes through as exec-form args, a bare string is Compose's own
// shorthand for shell form, wrapped here as ["/bin/sh", "-c", "<string>"]
// to match Docker's own documented interpretation of a string CMD or
// ENTRYPOINT.
type Command []string

// UnmarshalYAML implements the string-or-list union described above.
func (c *Command) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*c = []string{"/bin/sh", "-c", node.Value}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return fmt.Errorf("command: %w", err)
		}
		*c = items
		return nil
	default:
		return fmt.Errorf("command: must be a string or a list")
	}
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
