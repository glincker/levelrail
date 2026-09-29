package spec

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
)

// egressHostLike restricts egress.allow hosts to a hostname or IPv4
// literal, since the egress sidecar's shell script word-splits the list.
var egressHostLike = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// nameLike matches the pattern service and database keys must follow:
// lowercase alphanumeric and hyphens, since these become components of
// Docker container names, network names, and DNS-visible identifiers
// later (the same shape used for the brand's namespace prefix on those
// same identifiers). The JSON Schema can validate map value shapes but
// has no way to constrain map keys by pattern in the draft this schema
// targets without a much less readable propertyNames construct, so this
// is checked here instead.
var nameLike = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate checks rules the JSON Schema can't express: values whose
// validity depends on another field, or on the rest of the document, not
// just their own shape. Parse always runs this after schema validation
// succeeds, callers building a Spec by hand (tests, future tooling)
// should call it too before trusting a Spec.
func (s *Spec) Validate() error {
	seenDomains := make(map[string]string) // domain -> service name that claims it

	for name, svc := range s.Services {
		if !nameLike.MatchString(name) {
			return fmt.Errorf("spec: service %q: name must be lowercase alphanumeric and hyphens, starting with a letter", name)
		}

		if err := svc.validate(name); err != nil {
			return err
		}

		for _, domain := range svc.Domains {
			if owner, exists := seenDomains[domain]; exists {
				return fmt.Errorf("spec: domain %q is claimed by both service %q and service %q, a domain can only route to one service", domain, owner, name)
			}
			seenDomains[domain] = name
		}
	}

	if err := s.validateDependsOn(); err != nil {
		return err
	}

	for name, db := range s.Databases {
		if !nameLike.MatchString(name) {
			return fmt.Errorf("spec: database %q: name must be lowercase alphanumeric and hyphens, starting with a letter", name)
		}
		if db.Engine != EnginePostgres && db.Engine != EngineRedis && db.Engine != EngineMySQL && db.Engine != EngineMongoDB && db.Engine != EngineMariaDB && db.Engine != EngineKeyDB && db.Engine != EngineClickHouse && db.Engine != EngineDragonfly {
			return fmt.Errorf("spec: database %q: engine %q is not supported (supports %q, %q, %q, %q, %q, %q, %q, and %q)", name, db.Engine, EnginePostgres, EngineRedis, EngineMySQL, EngineMongoDB, EngineMariaDB, EngineKeyDB, EngineClickHouse, EngineDragonfly)
		}
	}

	return nil
}

func (svc *Service) validate(name string) error {
	if err := svc.validateBuild(name); err != nil {
		return err
	}
	if err := svc.validatePorts(name); err != nil {
		return err
	}
	if err := svc.validateBindAddress(name); err != nil {
		return err
	}
	if svc.Strategy != "" && svc.Strategy != StrategyRolling && svc.Strategy != StrategyRecreate && svc.Strategy != StrategyBlueGreen {
		// Unreachable while the JSON Schema's enum stays in sync with the
		// constants above, kept as a direct check anyway since Validate
		// is documented as safe to call on a hand-built Spec that never
		// went through schema validation.
		return fmt.Errorf("spec: service %q: strategy %q is not one of rolling, recreate, blue-green", name, svc.Strategy)
	}
	if svc.Resources != nil && svc.Resources.SwapMemory != "" && svc.Resources.Memory == "" {
		return fmt.Errorf("spec: service %q: resources.swapMemory requires resources.memory to also be set", name)
	}
	if err := ValidateLabels(svc.Labels); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	if err := svc.validateHooks(name); err != nil {
		return err
	}
	if err := svc.Health.Validate(name); err != nil {
		return err
	}
	if err := svc.validateEgress(name); err != nil {
		return err
	}
	if err := svc.LoadBalancer.Validate(); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	return svc.validateVolumes(name)
}

// validateEgress checks the egress: block's shape (mode must be a
// recognized value, allow must be non-empty when mode is allowlist,
// since an allowlist with nothing allowed is almost certainly a mistake
// rather than an intentional deny-all) and rejects it for a build.type
// with no single container for the egress sidecar to attach to, the same
// restriction validateHooks already applies for the same reason.
func (svc *Service) validateEgress(name string) error {
	if svc.Egress == nil {
		return nil
	}
	if svc.Build.Type == BuildStatic || svc.Build.Type == BuildCompose {
		return fmt.Errorf("spec: service %q: egress is not meaningful for build.type %q, there is no single container to attach the egress sidecar to", name, svc.Build.Type)
	}
	if svc.Egress.Mode != EgressModeAllowlist {
		// Unreachable while the JSON Schema's enum stays in sync with
		// EgressModeAllowlist, kept as a direct check anyway for the same
		// "safe to call on a hand-built Spec" reasoning the strategy
		// check above already gives.
		return fmt.Errorf("spec: service %q: egress.mode %q is not one of %q", name, svc.Egress.Mode, EgressModeAllowlist)
	}
	if len(svc.Egress.Allow) == 0 {
		return fmt.Errorf("spec: service %q: egress.mode: allowlist requires at least one entry in egress.allow", name)
	}
	for _, allow := range svc.Egress.Allow {
		if !egressHostLike.MatchString(allow.Host) {
			return fmt.Errorf("spec: service %q: egress.allow host %q must be a hostname or IPv4 address", name, allow.Host)
		}
		if allow.Port < 1 || allow.Port > 65535 {
			return fmt.Errorf("spec: service %q: egress.allow host %q: port %d is not a valid port", name, allow.Host, allow.Port)
		}
	}
	return nil
}

// validateHooks rejects a hooks: block on a build.type with no single
// container for the reconciler to exec a command inside: static (no
// container at all) and compose (a wrapper that expands into N real
// services at deploy time, per validatePorts' own comment on the same
// build.type, none of which this one hooks: block could unambiguously
// target).
func (svc *Service) validateHooks(name string) error {
	if svc.Hooks == nil {
		return nil
	}
	if svc.Build.Type == BuildStatic || svc.Build.Type == BuildCompose {
		return fmt.Errorf("spec: service %q: hooks is not meaningful for build.type %q, there is no single container to run a command in", name, svc.Build.Type)
	}
	return nil
}

// validateBuild checks the build.type/path/image/args/baseDirectory
// interactions svc.validate delegates to it, split out from the port
// checks below purely to keep each method's own cognitive complexity
// low; the two are independent concerns that happen to both key off
// svc.Build.Type.
func (svc *Service) validateBuild(name string) error {
	if svc.Build.Type == BuildCompose && svc.Build.Path == "" {
		return fmt.Errorf("spec: service %q: build.path is required for build.type: compose", name)
	}
	if svc.Build.Type == BuildImage && svc.Build.Image == "" {
		return fmt.Errorf("spec: service %q: build.image is required for build.type: image", name)
	}
	if svc.Build.Type != BuildImage && svc.Build.Image != "" {
		return fmt.Errorf("spec: service %q: build.image is only meaningful for build.type: image", name)
	}
	if svc.Build.Type == BuildImage && svc.Build.Path != "" {
		return fmt.Errorf("spec: service %q: build.path is not meaningful for build.type: image, there is nothing to build", name)
	}
	if len(svc.Build.Args) > 0 && svc.Build.Type != BuildDockerfile {
		return fmt.Errorf("spec: service %q: build.args is not meaningful for build.type %q", name, svc.Build.Type)
	}
	if svc.Build.BaseDirectory == "" {
		return nil
	}
	if svc.Build.Type == BuildImage {
		return fmt.Errorf("spec: service %q: build.baseDirectory is not meaningful for build.type: image, there is nothing to build", name)
	}
	if svc.Build.Type == BuildCompose {
		return fmt.Errorf("spec: service %q: build.baseDirectory is not meaningful for build.type: compose, use the compose file's own context: field instead", name)
	}
	if err := validateBaseDirectory(svc.Build.BaseDirectory); err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	return nil
}

// validatePorts checks svc.Port and svc.HostPort against svc.Build.Type,
// split out from validateBuild above for the same reason that doc
// comment gives.
func (svc *Service) validatePorts(name string) error {
	// compose joins static here: a compose-typed service is a wrapper
	// that expands into N real services at deploy time
	// (internal/deploy.Pipeline.DeploySpec's own expandComposeServices),
	// each with its own port from the compose file's own ports:, so the
	// wrapper itself has no single container to route to either.
	if svc.Build.Type != BuildStatic && svc.Build.Type != BuildCompose && svc.Port == 0 {
		return fmt.Errorf("spec: service %q: port is required unless build.type is %q or %q", name, BuildStatic, BuildCompose)
	}
	if svc.Build.Type == BuildStatic && svc.Port != 0 {
		return fmt.Errorf("spec: service %q: port must not be set when build.type is %q, static sites have no running container to route to", name, BuildStatic)
	}
	if svc.Build.Type == BuildCompose && svc.Port != 0 {
		return fmt.Errorf("spec: service %q: port must not be set when build.type is %q, each of the compose file's own services has its own port instead", name, BuildCompose)
	}
	if svc.HostPort == 0 {
		return nil
	}
	if svc.HostPort < 1 || svc.HostPort > 65535 {
		return fmt.Errorf("spec: service %q: host_port must be between 1 and 65535", name)
	}
	if svc.Build.Type == BuildStatic {
		return fmt.Errorf("spec: service %q: host_port must not be set when build.type is %q, static sites have no running container to publish a port for", name, BuildStatic)
	}
	return nil
}

// validateBindAddress checks svc.BindAddress: either empty (falls
// through to EffectiveBindAddress's default), or a value
// internal/bindaddr.Resolve accepts. Gated the same as HostPort above:
// not meaningful for a build.type with no single running container.
func (svc *Service) validateBindAddress(name string) error {
	if svc.BindAddress == "" {
		return nil
	}
	if err := bindaddr.Validate(svc.BindAddress); err != nil {
		return fmt.Errorf("spec: service %q: bind_address: %w", name, err)
	}
	if svc.Build.Type == BuildStatic {
		return fmt.Errorf("spec: service %q: bind_address must not be set when build.type is %q, static sites have no running container to publish a port for", name, BuildStatic)
	}
	return nil
}

// validateVolumes checks svc.Volumes for a bad name, a forbidden bind-mount
// host path, and for two volumes colliding on name or mount path, split
// out from svc.validate for the same reason validateBuild's own doc
// comment gives.
func (svc *Service) validateVolumes(name string) error {
	seenVolumeNames := make(map[string]bool, len(svc.Volumes))
	seenVolumePaths := make(map[string]bool, len(svc.Volumes))
	for _, v := range svc.Volumes {
		switch {
		case v.Name != "" && v.HostPath != "":
			return fmt.Errorf("spec: service %q: volume mounted at %q must set exactly one of name or hostPath, not both", name, v.Path)
		case v.HostPath != "":
			if err := validateBindMountHostPath(v.HostPath); err != nil {
				return fmt.Errorf("spec: service %q: %w", name, err)
			}
		case v.Name != "":
			if v.ReadOnly {
				return fmt.Errorf("spec: service %q: volume %q: readOnly is only meaningful alongside hostPath", name, v.Name)
			}
			if !nameLike.MatchString(v.Name) {
				return fmt.Errorf("spec: service %q: volume name %q must be lowercase alphanumeric and hyphens, starting with a letter", name, v.Name)
			}
			if seenVolumeNames[v.Name] {
				return fmt.Errorf("spec: service %q: duplicate volume name %q", name, v.Name)
			}
			seenVolumeNames[v.Name] = true
		default:
			// Unreachable while the JSON Schema's own oneOf (name xor
			// hostPath) stays in sync with this, kept anyway since
			// Validate is documented as safe to call on a hand-built Spec
			// that never went through schema validation.
			return fmt.Errorf("spec: service %q: volume mounted at %q must set either name or hostPath", name, v.Path)
		}
		if seenVolumePaths[v.Path] {
			return fmt.Errorf("spec: service %q: two volumes both mount %q", name, v.Path)
		}
		seenVolumePaths[v.Path] = true
	}
	return nil
}

// validateBaseDirectory rejects an absolute path or a "../" traversal at
// parse time, an early, friendly check; the authoritative one runs
// against the real checkout at deploy time (internal/deploy's
// resolveBuildRoot), since a relative path that looks safe here can
// still resolve outside the repo root once joined with it.
func validateBaseDirectory(dir string) error {
	if filepath.IsAbs(dir) {
		return fmt.Errorf("build.baseDirectory %q must be a relative path", dir)
	}
	clean := filepath.ToSlash(filepath.Clean(dir))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("build.baseDirectory %q must not escape the repository root", dir)
	}
	return nil
}

// EffectiveReplicas returns svc.Replicas, or DefaultReplicas if unset.
// Schema validation guarantees Replicas is never negative when set; zero
// means "not specified in app.yaml", not "zero replicas".
func (svc *Service) EffectiveReplicas() int {
	if svc.Replicas == 0 {
		return DefaultReplicas
	}
	return svc.Replicas
}

// EffectiveStrategy returns svc.Strategy, or the default (blue-green,
// since it's easier to get right than rolling with a single replica)
// if unset.
func (svc *Service) EffectiveStrategy() string {
	if svc == nil {
		return StrategyBlueGreen
	}
	if svc.Strategy == "" {
		return StrategyBlueGreen
	}
	return svc.Strategy
}

// EffectiveBindAddress returns svc.BindAddress, or bindaddr.Default
// (private, loopback-only) if unset. Mirrors EffectiveStrategy/
// EffectiveReplicas' own "resolve the default here, once" shape.
func (svc *Service) EffectiveBindAddress() string {
	if svc.BindAddress == "" {
		return bindaddr.Default
	}
	return svc.BindAddress
}

// validateDependsOn checks every service's dependsOn: entries reference a
// real sibling service with a single running container to gate on
// (BuildStatic never runs one, BuildCompose is a wrapper that expands
// into others at deploy time, not a container itself), and that no cycle
// exists: a cycle would deadlock the reconciler, since every member of
// it would wait forever for another member that is itself waiting.
func (s *Spec) validateDependsOn() error {
	for name, svc := range s.Services {
		for _, dep := range svc.DependsOn {
			if dep == name {
				return fmt.Errorf("spec: service %q: dependsOn must not reference itself", name)
			}
			target, ok := s.Services[dep]
			if !ok {
				return fmt.Errorf("spec: service %q: dependsOn references %q, which is not a service in this file", name, dep)
			}
			if target.Build.Type == BuildStatic || target.Build.Type == BuildCompose {
				return fmt.Errorf("spec: service %q: dependsOn references %q, whose build.type %q has no single running container to depend on", name, dep, target.Build.Type)
			}
		}
	}
	return detectDependsOnCycle(s.Services)
}

// detectDependsOnCycle runs a depth-first search over services' dependsOn
// edges, returning an error naming the cycle's path the first time one is
// found. Service names are visited in sorted order so the same cyclic
// spec always reports the same path, not whichever map iteration order
// Go happened to pick.
func detectDependsOnCycle(services map[string]Service) error {
	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(services))

	var visit func(name string, path []string) error
	visit = func(name string, path []string) error {
		switch state[name] {
		case done:
			return nil
		case visiting:
			return fmt.Errorf("spec: dependsOn cycle: %s -> %s", strings.Join(path, " -> "), name)
		}
		state[name] = visiting
		for _, dep := range services[name].DependsOn {
			if err := visit(dep, append(path, name)); err != nil {
				return err
			}
		}
		state[name] = done
		return nil
	}

	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := visit(name, nil); err != nil {
			return err
		}
	}
	return nil
}
