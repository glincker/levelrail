// Package compose parses a Docker Compose file into Levelrail's own
// desired-state model, in two shapes depending on the caller. The
// direct-import path (ToDesiredServices, via Validate) is deliberately
// narrow: every service needs a pre-built image (no build:), since
// there is no build context to build one from (a pasted file, no git
// checkout). The git-sourced expand path (ExpandBuildService, via
// ValidateForBuild) allows build: for exactly that reason: it always
// has a real checkout. Both paths share the same narrow scope
// otherwise: environment/ports/volumes accept both Compose's short and
// long forms (see yaml.go), but only the fields Levelrail's own model
// has room for: no port ranges, no UDP, no tmpfs/npipe mounts.
// restart: and networks: both parse and are surfaced as non-blocking
// Notices instead of being silently dropped or translated: see Notices
// for why neither has a real translation onto how Levelrail runs a
// service. depends_on: parses into spec.Service.DependsOn / store.
// DesiredService.DependsOn and IS enforced: internal/reconcile/
// application.Controller waits for a dependency's container to start
// before creating this service's own, real Docker Compose's own default
// depends_on semantic (service_started, not service_healthy). command: and
// entrypoint: both parse and translate into store.DesiredService's own
// Command and Entrypoint fields. volumes: additionally accepts an
// absolute host path on the left side as a bind mount (ValidateForBuild
// rejects one; see that method's own doc comment for why), gated at the
// HTTP layer to AbilityRoot and, even then, against
// internal/bindmount's own forbidden-path list (see
// validateBindMountHostPath).
package compose

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bindmount"
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
	// HasSecrets/HasConfigs record whether this file declares a top-level
	// secrets: or configs: block: neither has a translation onto this
	// platform's own env-var/Levelrail-secrets model, so validate rejects
	// either outright (unlike restart:/networks:/depends_on:, which parse
	// and are merely non-blocking Notices) rather than silently dropping
	// values an operator would reasonably expect to reach a container.
	HasSecrets bool
	HasConfigs bool
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
	// DependsOn is depends_on:, enforced by the reconciler as a start-
	// order guarantee (see this file's own doc comment and Notices).
	DependsOn DependsOn
	// Command overrides the image's own default CMD
	// (store.DesiredService.Command), parsed from command:'s own
	// string-or-list union (Command's own UnmarshalYAML in yaml.go): a
	// plain string is shell-wrapped as ["/bin/sh", "-c", "<string>"],
	// matching Compose's own documented behavior for that form.
	Command Command
	// Entrypoint overrides the image's own default ENTRYPOINT
	// (store.DesiredService.Entrypoint), parsed with the same
	// string-or-list union as Command.
	Entrypoint Command
	// PullPolicy is pull_policy:, normalized by normalizePullPolicy into
	// exactly the two states store.DesiredService.PullPolicy
	// distinguishes: PullPolicyAlways forces a fresh pull even when the
	// image is already present locally, empty means today's existing
	// pull-if-absent behavior. Real Compose's other recognized spellings
	// ("missing", "if_not_present", an unset field) all normalize to
	// empty too; "never" and "build" are rejected, see
	// normalizePullPolicy.
	PullPolicy string
	// Deploy carries the GPU device reservation and replicas count
	// (gpu.go).
	Deploy *Deploy
	// Secrets/Configs are this service's own secrets:/configs: references
	// (short string form or long {source, target,...} map form, either
	// decodes fine into []any since only their presence matters here),
	// the per-service counterpart to File.HasSecrets/HasConfigs; same
	// rejection reasoning, see validateComposeServices.
	Secrets []any
	Configs []any
}

// PullPolicyAlways is pull_policy: always, the only non-default value
// this package supports.
const PullPolicyAlways = "always"

// normalizePullPolicy maps pull_policy:'s recognized spellings onto the
// two states store.DesiredService.PullPolicy actually distinguishes.
// "never" and "build" are rejected outright: neither has a real
// translation onto docker.Client's own pull-if-absent/force-pull model.
func normalizePullPolicy(raw string) (string, error) {
	switch raw {
	case "", "missing", "if_not_present":
		return "", nil
	case PullPolicyAlways:
		return PullPolicyAlways, nil
	default:
		return "", fmt.Errorf("pull_policy: %q is not supported, use %q or leave it unset", raw, PullPolicyAlways)
	}
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

	f := &File{
		Version:    raw.Version,
		Services:   make(map[string]Service, len(raw.Services)),
		Domains:    raw.Domains,
		HasSecrets: len(raw.Secrets) > 0,
		HasConfigs: len(raw.Configs) > 0,
	}
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
	errs = append(errs, validateComposeDependsOn(f)...)
	errs = append(errs, validateUnsupportedTopLevel(f)...)

	if len(errs) == 0 {
		return nil
	}
	return joinErrors(errs)
}

// validateUnsupportedTopLevel rejects the blocks that have no
// translation onto this platform's model at all (unlike restart:/
// networks:, which parse into a non-blocking Notice): secrets: and
// configs: name external sources this platform has no way to fetch or
// mount, so a referencing service would otherwise silently start
// without whatever it expected there.
func validateUnsupportedTopLevel(f *File) []error {
	var errs []error
	if f.HasSecrets {
		errs = append(errs, fmt.Errorf("top-level secrets: is not supported yet; move the value into your app's own env vars or Levelrail secrets instead"))
	}
	if f.HasConfigs {
		errs = append(errs, fmt.Errorf("top-level configs: is not supported yet; move the value into your app's own env vars, a baked-in file, or a named volume instead"))
	}
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		if len(svc.Secrets) > 0 {
			errs = append(errs, fmt.Errorf("service %q: secrets: is not supported yet; move the value into env or Levelrail secrets instead", name))
		}
		if len(svc.Configs) > 0 {
			errs = append(errs, fmt.Errorf("service %q: configs: is not supported yet; move the value into env, a baked-in file, or a named volume instead", name))
		}
		if svc.Deploy != nil {
			for _, key := range svc.Deploy.unsupported {
				errs = append(errs, fmt.Errorf("service %q: deploy.%s is not supported yet; it is a Swarm-specific field with no meaning outside a Swarm cluster (only deploy.resources.reservations.devices, for GPU reservations, and deploy.replicas are read)", name, key))
			}
		}
	}
	return errs
}

// validateBindMountHostPath delegates to internal/bindmount, shared with
// internal/spec (see that package's own bindmount.go for why neither
// compose nor spec can hold this directly without an import cycle).
func validateBindMountHostPath(hostPath string) error {
	return bindmount.ValidateHostPath(hostPath)
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

// validateComposeDependsOn checks that every depends_on: entry names a
// real sibling service in this file, and that no dependency cycle
// exists: internal/spec.detectDependsOnCycle's own doc comment explains
// why a cycle must be rejected rather than left to deadlock the
// reconciler. Reference checks and cycle detection run on this file's
// own service graph, independent of internal/spec's identical check on
// app.yaml's services: map, since a compose file is validated (and its
// depends_on: entries only ever mean something) on its own, before
// internal/spec.Service.DependsOn is populated from it.
func validateComposeDependsOn(f *File) []error {
	var errs []error
	for _, name := range sortedServiceNames(f) {
		svc := f.Services[name]
		for _, dep := range svc.DependsOn {
			if dep == name {
				errs = append(errs, fmt.Errorf("service %q: depends_on must not reference itself", name))
				continue
			}
			if _, ok := f.Services[dep]; !ok {
				errs = append(errs, fmt.Errorf("service %q: depends_on references %q, which is not a service in this file", name, dep))
			}
		}
	}
	if len(errs) > 0 {
		return errs
	}
	if err := detectComposeDependsOnCycle(f); err != nil {
		return []error{err}
	}
	return nil
}

// detectComposeDependsOnCycle is internal/spec.detectDependsOnCycle's own
// algorithm, duplicated rather than shared: the two packages' service
// graphs (compose.Service vs. spec.Service) are different types, and this
// is the only place either package needs it.
func detectComposeDependsOnCycle(f *File) error {
	const (
		visiting = 1
		done     = 2
	)
	state := make(map[string]int, len(f.Services))

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("depends_on cycle: %s -> %s", strings.Join(path, " -> "), name)
		}
		state[name] = visiting
		for _, dep := range f.Services[name].DependsOn {
			if err := visit(dep, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = done
		return nil
	}

	for _, name := range sortedServiceNames(f) {
		if err := visit(name, nil); err != nil {
			return err
		}
	}
	return nil
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
	var b strings.Builder
	fmt.Fprintf(&b, "%d service(s) failed validation:", len(errs))
	for _, err := range errs {
		b.WriteString("\n  - ")
		b.WriteString(err.Error())
	}
	return errors.New(b.String())
}
