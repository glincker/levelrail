// Package spec parses and validates the app.yaml file: the one
// declarative file a user writes in their repo, per the app spec design.
// Validation happens in two layers. Structural shape (required fields,
// enums, string patterns, unknown-key rejection) is checked against the
// embedded JSON Schema (schema/app.schema.json), kept as the single
// source of truth so CI and the deploy-time check share one definition,
// not two hand-maintained copies that drift. Rules the schema can't
// express (a field's validity depending on another field's value) are
// checked by Validate after parsing.
package spec

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Spec is a fully parsed, schema-valid app.yaml.
type Spec struct {
	Version   int                 `yaml:"version"`
	Services  map[string]Service  `yaml:"services"`
	Databases map[string]Database `yaml:"databases,omitempty"`
}

// Service is one entry under services:.
type Service struct {
	Build     Build             `yaml:"build"`
	Domains   []string          `yaml:"domains,omitempty"`
	Port      int               `yaml:"port,omitempty"`
	Health    *Health           `yaml:"health,omitempty"`
	Resources *Resources        `yaml:"resources,omitempty"`
	Env       map[string]EnvVar `yaml:"env,omitempty"`
	Replicas  int               `yaml:"replicas,omitempty"`
	Strategy  string            `yaml:"strategy,omitempty"`
	// Labels are arbitrary operator-supplied Docker labels applied to the
	// service's container at create time, an escape hatch for tooling
	// this platform doesn't know about (a monitoring agent or log
	// shipper that keys off container labels, a homegrown script, and
	// so on). See ValidateLabels (labels.go) for what's rejected:
	// notably, any key under ReservedLabelPrefix, kept open for this
	// platform's own bookkeeping labels.
	Labels map[string]string `yaml:"labels,omitempty"`

	// Volumes are named Docker volumes this service's container mounts,
	// previously a database-only capability. Name is a logical name
	// scoped to this service, not a global Docker volume name (see
	// internal/deploy's translation into store.ServiceVolume for the
	// actual, platform-prefixed name); two services can each declare a
	// volume named "data" without colliding.
	Volumes []Volume `yaml:"volumes,omitempty"`
}

// Volume is one entry under a service's volumes:.
type Volume struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

// Build input types: the app spec's build.type values, matching the
// BuildKit-based build detection (Dockerfile, Compose, Railpack, static)
// plus one non-build type: a prebuilt image already sitting in a
// registry, deployed as-is with no build step at all (see Build.Image
// and internal/deploy's deployImage).
const (
	BuildDockerfile = "dockerfile"
	BuildCompose    = "compose"
	BuildRailpack   = "railpack"
	BuildStatic     = "static"
	BuildImage      = "image"
)

// Deploy strategies, part of the app spec. Blue-green is the effective
// default, since it's easier to get right than rolling with a single
// replica, applied by DefaultStrategy when Strategy is empty, not
// baked into the schema itself.
const (
	StrategyRolling   = "rolling"
	StrategyRecreate  = "recreate"
	StrategyBlueGreen = "blue-green"
)

// DefaultReplicas is used when a service doesn't set replicas.
const DefaultReplicas = 1

// Build describes how a service's image gets built.
type Build struct {
	Type string `yaml:"type"`
	Path string `yaml:"path,omitempty"`
	// BaseDirectory scopes the build context to a subdirectory of the
	// repo, e.g. "apps/web" in a monorepo. Empty means the repo root.
	// Meaningful for dockerfile, railpack, and static; not meaningful
	// for image (nothing gets built) or compose (the compose file's own
	// context: field already scopes each service).
	BaseDirectory string `yaml:"baseDirectory,omitempty"`
	// Image is a full registry reference (e.g.
	// "ghcr.io/org/app:v1.2.3"), only meaningful for build.type: image:
	// a CI pipeline (or anything else) already built and pushed this
	// exact image, so there is nothing for this control plane to build,
	// only to deploy as-is. See internal/deploy's deployImage.
	Image string `yaml:"image,omitempty"`
	// RegistryCredential names a store.RegistryCredential (by its Name,
	// not ID: app.yaml is hand-written, an opaque ID would be hostile
	// to author) to authenticate with when pulling Image from a private
	// registry. Empty means an unauthenticated (public) pull.
	RegistryCredential string `yaml:"registryCredential,omitempty"`
	// Args are Dockerfile build-time ARG values (e.g. a base image
	// version, a build-time feature flag), passed through as
	// --build-arg equivalents to BuildKit. Only meaningful for
	// build.type: dockerfile.
	Args map[string]string `yaml:"args,omitempty"`
}

// Health holds a service's readiness and liveness probe configuration.
type Health struct {
	Readiness *Probe `yaml:"readiness,omitempty"`
	Liveness  *Probe `yaml:"liveness,omitempty"`
}

// Probe is a single HTTP health check.
type Probe struct {
	Path     string `yaml:"path"`
	Interval string `yaml:"interval,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
	Failures int    `yaml:"failures,omitempty"`
}

// Resources holds a service's resource limits.
type Resources struct {
	Memory string  `yaml:"memory,omitempty"`
	CPU    float64 `yaml:"cpu,omitempty"`
}

// Supported managed database engines: Postgres and Redis shipped as
// first-class resources in the initial release, MySQL, MongoDB,
// MariaDB, and KeyDB joined once internal/reconcile/database's
// controller grew matching engine cases.
const (
	EngineFake     = "" // zero value only, never valid; see Validate
	EnginePostgres = "postgres"
	EngineRedis    = "redis"
	EngineMySQL    = "mysql"
	EngineMongoDB  = "mongodb"
	EngineMariaDB  = "mariadb"
	EngineKeyDB    = "keydb"
)

// Database is one entry under databases:.
type Database struct {
	Engine  string  `yaml:"engine"`
	Version string  `yaml:"version,omitempty"`
	Backup  *Backup `yaml:"backup,omitempty"`
}

// Backup describes a database's backup schedule.
type Backup struct {
	Schedule string `yaml:"schedule"`
	Retain   int    `yaml:"retain,omitempty"`
}

// Parse validates raw app.yaml bytes against the embedded JSON Schema,
// then unmarshals into a Spec and runs the semantic checks the schema
// can't express. Both layers must pass for Parse to succeed; a caller
// never sees a Spec that's schema-valid but semantically broken, or vice
// versa.
func Parse(data []byte) (*Spec, error) {
	if err := validateAgainstSchema(data); err != nil {
		return nil, err
	}

	var s Spec
	if err := yamlUnmarshalStrict(data, &s); err != nil {
		return nil, fmt.Errorf("spec: parse: %w", err)
	}

	if err := s.Validate(); err != nil {
		return nil, err
	}

	return &s, nil
}

// yamlUnmarshalStrict decodes with KnownFields(true), rejecting any YAML
// key with no matching struct field. The embedded JSON Schema already
// rejects unknown top-level shape via additionalProperties: false, but
// this is a second, independent guard: if the schema and these structs
// ever drift apart (a field added to one and not the other), this catches
// it as a decode error during development rather than a silently
// dropped field in production.
func yamlUnmarshalStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	return dec.Decode(out)
}
