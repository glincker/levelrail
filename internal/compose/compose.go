// Package compose parses a Docker Compose file into Levelrail's own
// desired-state model, in two shapes depending on the caller. The
// direct-import path (ToDesiredServices, via Validate) is deliberately
// narrow: every service needs a pre-built image (no build:), since
// there is no build context to build one from (a pasted file, no git
// checkout). The git-sourced expand path (ExpandBuildService, via
// ValidateForBuild) allows build: for exactly that reason: it always
// has a real checkout. Both paths share the same narrow scope
// otherwise: environment/ports/volumes support only their short-form
// syntax, and depends_on parses but is ignored (reconciler-level
// startup ordering, out of scope here). restart: and networks: parse
// and are surfaced as non-blocking Notices instead of being silently
// dropped or translated: see Notices for why neither has a real
// translation onto how Levelrail runs a service. volumes: additionally
// accepts an absolute host path on the left side as a bind mount
// (ValidateForBuild rejects one; see that method's own doc comment for
// why), gated at the HTTP layer to AbilityRoot and, even then, against
// forbiddenBindMountPaths (see validateBindMountHostPath).
package compose

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

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
	// Networks lists this file's own top-level networks: names, sorted.
	// Only used by Notices to detect that custom networks were declared
	// at all; Levelrail doesn't create per-network isolation from this.
	Networks []string
}

// Service is one entry under services:.
type Service struct {
	Image       string
	Build       *rawBuild
	Environment Environment
	Ports       []Port
	Volumes     []Volume
	Labels      map[string]string
	Networks    Networks
	Restart     string
	Healthcheck *Healthcheck
}

// Volume is one short-form "name:/container/path" entry, either a named
// Docker volume (Name set, HostPath empty) or a bind mount of a real
// host directory (HostPath set, Name empty): exactly one of the two is
// ever set, see Volume.UnmarshalYAML (yaml.go) for how the left side of
// the entry decides which.
type Volume struct {
	Name          string
	HostPath      string
	ContainerPath string
	// ReadOnly mounts read-only inside the container, from an optional
	// trailing ":ro" on the short-form entry.
	ReadOnly bool
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
	if len(raw.Networks) > 0 {
		f.Networks = make([]string, 0, len(raw.Networks))
		for name := range raw.Networks {
			f.Networks = append(f.Networks, name)
		}
		sort.Strings(f.Networks)
	}
	return f, nil
}

// Validate reports every unsupported-shape problem across all
// services, not just the first, so a template author can fix them in
// one pass. Used by the direct-import path (ToDesiredServices), which
// has no build context (no git checkout, just a pasted file) to build
// a build: block from, so it rejects one outright. See ValidateForBuild
// for the git-sourced deploy-spec path, which does have one.
func (f *File) Validate() error {
	return f.validate(false, true)
}

// ValidateForBuild is Validate, except a service's build: block is
// allowed rather than rejected: used only by the git-sourced
// expand-a-compose-file-into-services path (ExpandBuildService), which
// has a real checkout to resolve a build context against, unlike the
// direct-import path Validate itself still guards. Bind mounts stay
// rejected on this path even though Validate now allows them: the
// expanded result is a spec.Service (toSpecService, expand.go), and
// spec.Volume has no host-path concept to carry one into, so allowing
// one through here would either drop it silently or produce a
// nonsensical empty-named volume downstream.
func (f *File) ValidateForBuild() error {
	return f.validate(true, false)
}

func (f *File) validate(allowBuild, allowBindMounts bool) error {
	if len(f.Services) == 0 {
		return fmt.Errorf("compose: no services declared")
	}

	errs := validateComposeServices(f, allowBuild, allowBindMounts)
	errs = append(errs, validateComposeDomainRefs(f)...)

	if len(errs) == 0 {
		return nil
	}
	return joinErrors(errs)
}

// forbiddenBindMountPaths are host paths a bind mount may never target,
// enforced even for an AbilityRoot caller (internal/api's ability gate,
// not this list, is the primary boundary; this is defense in depth): an
// exact match or a match of clean+"/" as a prefix. Each one grants
// something categorically worse than ordinary bind-mount access, host
// root compromise for most of these. /var/run/docker.sock (and
// /var/run generally, since a socket can be bind-mounted from anywhere
// under it) is deliberately excluded from this feature by design, not
// an oversight: Docker-socket access is a full container-escape-to-
// host-root vector via the Docker API, a categorically different and
// unreviewed capability that needs its own explicit design decision
// later, not bundled into general bind-mount support here.
var forbiddenBindMountPaths = []string{
	"/",
	"/etc",
	"/root",
	"/boot",
	"/sys",
	"/proc",
	"/var/lib/docker",
	"/var/run/docker.sock",
	"/var/run",
}

// validateBindMountHostPath rejects a relative path and every path
// forbiddenBindMountPaths covers; anything else is a real, operator-
// owned host directory this feature exists to allow.
func validateBindMountHostPath(hostPath string) error {
	if !strings.HasPrefix(hostPath, "/") {
		return fmt.Errorf("bind-mount host path %q must be an absolute path", hostPath)
	}
	clean := filepath.Clean(hostPath)
	for _, forbidden := range forbiddenBindMountPaths {
		if clean == forbidden || strings.HasPrefix(clean, forbidden+"/") {
			return fmt.Errorf("bind-mount host path %q is not allowed: %q is a protected system path", hostPath, forbidden)
		}
	}
	return nil
}

// validateComposeServices checks each service's own build:/image:
// declaration and volume names, split out of validate purely to keep
// that function's own cognitive complexity low.
func validateComposeServices(f *File, allowBuild, allowBindMounts bool) []error {
	var errs []error
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if svc.Build != nil && !allowBuild {
			errs = append(errs, fmt.Errorf("service %q: build: is not supported, declare a pre-built image: instead", name))
		}
		if svc.Image == "" && svc.Build == nil {
			errs = append(errs, fmt.Errorf("service %q: image is required", name))
		}
		for _, v := range svc.Volumes {
			if v.HostPath != "" {
				if !allowBindMounts {
					errs = append(errs, fmt.Errorf("service %q: bind-mount volume %q is not supported here, use a named volume instead", name, v.ContainerPath))
					continue
				}
				if err := validateBindMountHostPath(v.HostPath); err != nil {
					errs = append(errs, fmt.Errorf("service %q: %w", name, err))
				}
				continue
			}
			if v.Name == "" {
				errs = append(errs, fmt.Errorf("service %q: volume mounted at %q must be a named volume (\"name:/path\") or an absolute bind-mount path", name, v.ContainerPath))
			}
		}
	}
	return errs
}

// validateComposeDomainRefs checks that every x-levelrail-domains key
// names a real service in this file, split out of validate for the
// same reason validateComposeServices's own doc comment gives.
func validateComposeDomainRefs(f *File) []error {
	var errs []error
	for svcKey := range f.Domains {
		if _, ok := f.Services[svcKey]; !ok {
			errs = append(errs, fmt.Errorf("x-levelrail-domains: %q is not a service in this file", svcKey))
		}
	}
	return errs
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
