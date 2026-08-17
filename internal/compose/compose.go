// Package compose parses a Docker Compose file into Levelrail's own
// desired-state model. Scope is deliberately narrow: every service
// needs a pre-built image (no build:), environment/ports/volumes
// support only their short-form syntax, and everything else (networks,
// depends_on, restart, ...) parses but is ignored.
package compose

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// File is a parsed compose.yaml.
type File struct {
	Version  string
	Services map[string]Service
	// Domains maps a service key to the real domain it should be
	// reachable at, from the top-level x-levelrail-domains extension
	// (Compose's own reserved x- prefix for tool-specific keys). Used
	// both to set that service's own store.DesiredService.Domains (real
	// ingress routing) and to resolve any ${SERVICE_FQDN_*} reference
	// within that same service's environment (ResolveMagicVars).
	Domains map[string]string
}

// Service is one entry under services:.
type Service struct {
	Image       string
	Build       *rawBuild
	Environment Environment
	Ports       []Port
	Volumes     []Volume
	Labels      map[string]string
}

// Volume is one short-form "name:/container/path" entry.
type Volume struct {
	Name          string
	ContainerPath string
}

// Port is one short-form ports: entry. ContainerPort is what
// store.DesiredService.Port (a single container port, not a
// host:container pair) actually uses.
type Port struct {
	HostPort      int
	ContainerPort int
}

// rawBuild exists so Parse can detect and reject build:; never
// populated into a translated Service.
type rawBuild struct {
	Context    string
	Dockerfile string
}

// Parse decodes a compose.yaml document.
func Parse(data []byte) (*File, error) {
	var raw rawFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("compose: parse: %w", err)
	}

	f := &File{Version: raw.Version, Services: make(map[string]Service, len(raw.Services)), Domains: raw.Domains}
	for name, svc := range raw.Services {
		f.Services[name] = Service(svc)
	}
	return f, nil
}

// Validate reports every unsupported-shape problem across all
// services, not just the first, so a template author can fix them in
// one pass.
func (f *File) Validate() error {
	if len(f.Services) == 0 {
		return fmt.Errorf("compose: no services declared")
	}

	var errs []error
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if svc.Build != nil {
			errs = append(errs, fmt.Errorf("service %q: build: is not supported, declare a pre-built image: instead", name))
		}
		if svc.Image == "" && svc.Build == nil {
			errs = append(errs, fmt.Errorf("service %q: image is required", name))
		}
		for _, v := range svc.Volumes {
			if v.Name == "" {
				errs = append(errs, fmt.Errorf("service %q: volume mounted at %q must be a named volume (\"name:/path\"), not a bind mount", name, v.ContainerPath))
			}
		}
	}
	for svcKey := range f.Domains {
		if _, ok := f.Services[svcKey]; !ok {
			errs = append(errs, fmt.Errorf("x-levelrail-domains: %q is not a service in this file", svcKey))
		}
	}

	if len(errs) == 0 {
		return nil
	}
	return joinErrors(errs)
}

func sortedServiceNames(f *File) []string {
	names := make([]string, 0, len(f.Services))
	for name := range f.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func joinErrors(errs []error) error {
	msg := fmt.Sprintf("%d service(s) failed validation:", len(errs))
	for _, err := range errs {
		msg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", msg)
}
