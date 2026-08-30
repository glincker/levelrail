package spec

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

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

	if svc.Build.BaseDirectory != "" {
		if svc.Build.Type == BuildImage {
			return fmt.Errorf("spec: service %q: build.baseDirectory is not meaningful for build.type: image, there is nothing to build", name)
		}
		if svc.Build.Type == BuildCompose {
			return fmt.Errorf("spec: service %q: build.baseDirectory is not meaningful for build.type: compose, use the compose file's own context: field instead", name)
		}
		if err := validateBaseDirectory(svc.Build.BaseDirectory); err != nil {
			return fmt.Errorf("spec: service %q: %w", name, err)
		}
	}

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

	if svc.HostPort != 0 {
		if svc.HostPort < 1 || svc.HostPort > 65535 {
			return fmt.Errorf("spec: service %q: host_port must be between 1 and 65535", name)
		}
		if svc.Build.Type == BuildStatic {
			return fmt.Errorf("spec: service %q: host_port must not be set when build.type is %q, static sites have no running container to publish a port for", name, BuildStatic)
		}
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

	seenVolumeNames := make(map[string]bool, len(svc.Volumes))
	seenVolumePaths := make(map[string]bool, len(svc.Volumes))
	for _, v := range svc.Volumes {
		if !nameLike.MatchString(v.Name) {
			return fmt.Errorf("spec: service %q: volume name %q must be lowercase alphanumeric and hyphens, starting with a letter", name, v.Name)
		}
		if seenVolumeNames[v.Name] {
			return fmt.Errorf("spec: service %q: duplicate volume name %q", name, v.Name)
		}
		seenVolumeNames[v.Name] = true
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
	if svc.Strategy == "" {
		return StrategyBlueGreen
	}
	return svc.Strategy
}
