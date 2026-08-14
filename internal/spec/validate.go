package spec

import (
	"fmt"
	"regexp"
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
		if db.Engine != EnginePostgres && db.Engine != EngineRedis && db.Engine != EngineMySQL {
			return fmt.Errorf("spec: database %q: engine %q is not supported (supports %q, %q, and %q)", name, db.Engine, EnginePostgres, EngineRedis, EngineMySQL)
		}
	}

	return nil
}

func (svc *Service) validate(name string) error {
	if svc.Build.Type == BuildCompose && svc.Build.Path == "" {
		return fmt.Errorf("spec: service %q: build.path is required for build.type: compose", name)
	}

	if svc.Build.Type != BuildStatic && svc.Port == 0 {
		return fmt.Errorf("spec: service %q: port is required unless build.type is %q", name, BuildStatic)
	}
	if svc.Build.Type == BuildStatic && svc.Port != 0 {
		return fmt.Errorf("spec: service %q: port must not be set when build.type is %q, static sites have no running container to route to", name, BuildStatic)
	}

	if svc.Strategy != "" && svc.Strategy != StrategyRolling && svc.Strategy != StrategyRecreate && svc.Strategy != StrategyBlueGreen {
		// Unreachable while the JSON Schema's enum stays in sync with the
		// constants above, kept as a direct check anyway since Validate
		// is documented as safe to call on a hand-built Spec that never
		// went through schema validation.
		return fmt.Errorf("spec: service %q: strategy %q is not one of rolling, recreate, blue-green", name, svc.Strategy)
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
