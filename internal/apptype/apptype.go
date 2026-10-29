// Package apptype models a governed app type (ADR 052): a constraint and
// defaults layer over the app spec that an admin publishes and apps pin.
package apptype

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/policy"
)

// Env names for the feature flag and its limits.
const (
	EnvEnabled         = "LEVELRAIL_APPTYPE_ENABLED"
	EnvMaxParams       = "LEVELRAIL_APPTYPE_MAX_PARAMS"
	EnvMaxSchemaBytes  = "LEVELRAIL_APPTYPE_MAX_SCHEMA_BYTES"
	EnvMaxPatternLen   = "LEVELRAIL_APPTYPE_MAX_PATTERN_LEN"
	EnvMaxVersions     = "LEVELRAIL_APPTYPE_MAX_VERSIONS"
	defaultMaxParams   = 32
	defaultSchemaBytes = 16384
	defaultPatternLen  = 256
	defaultVersions    = 50
)

// Limits caps what a type document may carry.
type Limits struct {
	MaxParams      int
	MaxSchemaBytes int
	MaxPatternLen  int
	MaxVersions    int
}

// Enabled reports whether the feature flag is on; it is read per call so a
// restart is the only switch an operator needs.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvEnabled))) {
	case "1", "true", "on", "yes":
		return true
	}
	return false
}

// LimitsFromEnv reads the caps, falling back to the ADR defaults.
func LimitsFromEnv() Limits {
	return Limits{
		MaxParams:      envInt(EnvMaxParams, defaultMaxParams),
		MaxSchemaBytes: envInt(EnvMaxSchemaBytes, defaultSchemaBytes),
		MaxPatternLen:  envInt(EnvMaxPatternLen, defaultPatternLen),
		MaxVersions:    envInt(EnvMaxVersions, defaultVersions),
	}
}

func envInt(name string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && n > 0 {
		return n
	}
	return def
}

// AppType is the published document.
type AppType struct {
	Name              string             `json:"name"`
	Version           int                `json:"version,omitempty"`
	Title             string             `json:"title,omitempty"`
	Description       string             `json:"description,omitempty"`
	Workload          string             `json:"workload"`
	Enforcement       policy.Mode        `json:"enforcement,omitempty"`
	Parameters        json.RawMessage    `json:"parameters,omitempty"`
	Binds             map[string]string  `json:"binds,omitempty"`
	EnvironmentConfig map[string]EnvRule `json:"environmentConfig,omitempty"`
	Build             BuildRule          `json:"build,omitempty"`
	Addons            Addons             `json:"addons,omitempty"`
	Fixed             map[string]string  `json:"fixed,omitempty"`
	Defaults          map[string]string  `json:"defaults,omitempty"`
	Messages          map[string]string  `json:"messages,omitempty"`
}

// EnvRule tightens a type for one environment kind.
type EnvRule struct {
	ReplicasMin       *int `json:"replicasMin,omitempty"`
	ReplicasMax       *int `json:"replicasMax,omitempty"`
	ReadinessRequired bool `json:"readinessRequired,omitempty"`
}

// BuildRule limits build methods; empty means any.
type BuildRule struct {
	Allowed []string `json:"allowed,omitempty"`
}

// Addons limits optional extras; a nil rule means unrestricted.
type Addons struct {
	Databases *DatabaseRule `json:"databases,omitempty"`
	Volumes   *Toggle       `json:"volumes,omitempty"`
	Domains   *DomainRule   `json:"domains,omitempty"`
	Cron      *Toggle       `json:"cron,omitempty"`
}

// Toggle allows or forbids an add-on.
type Toggle struct {
	Allowed bool `json:"allowed"`
}

// DatabaseRule limits database engines and count.
type DatabaseRule struct {
	AllowedEngines []string `json:"allowed_engines,omitempty"`
	Max            int      `json:"max,omitempty"`
}

// DomainRule limits custom domains.
type DomainRule struct {
	Allowed  bool     `json:"allowed"`
	Suffixes []string `json:"suffixes,omitempty"`
}

var (
	nameRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	bindRe       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.*-]{0,127}$`)
	workloads    = map[string]bool{"web": true, "worker": true, "cron": true, "static": true}
	buildMethods = map[string]bool{"dockerfile": true, "compose": true, "railpack": true, "static": true, "image": true}
	envKinds     = map[string]bool{"dev": true, "test": true, "uat": true, "production": true, "preview": true, "custom": true}
)

// ParseDocument decodes and validates a type document. Unknown fields are
// refused so a typo in a rule cannot silently weaken a type.
func ParseDocument(raw []byte, lim Limits) (AppType, error) {
	var t AppType
	if len(raw) > lim.MaxSchemaBytes {
		return t, fmt.Errorf("document is %d bytes, the limit is %d", len(raw), lim.MaxSchemaBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return t, fmt.Errorf("invalid type document: %w", err)
	}
	return t, t.Validate(lim)
}

// Validate checks the document's own consistency.
func (t AppType) Validate(lim Limits) error {
	if !nameRe.MatchString(t.Name) {
		return fmt.Errorf("name %q must be lowercase letters, digits and dashes, starting with a letter", t.Name)
	}
	if !workloads[t.Workload] {
		return fmt.Errorf("workload %q must be web, worker, cron or static", t.Workload)
	}
	switch t.Enforcement {
	case "", policy.ModeWarn, policy.ModeEnforce:
	default:
		return fmt.Errorf("enforcement %q must be warn or enforce", t.Enforcement)
	}
	for _, b := range t.Build.Allowed {
		if !buildMethods[b] {
			return fmt.Errorf("build.allowed has %q, which is not a build method", b)
		}
	}
	for kind := range t.EnvironmentConfig {
		if !envKinds[kind] {
			return fmt.Errorf("environmentConfig key %q is not an environment kind (dev, test, uat, production, preview, custom)", kind)
		}
	}
	schema, err := ParseSchema(t.Parameters, lim)
	if err != nil {
		return err
	}
	return t.checkBinds(schema.ParamPaths())
}

func (t AppType) checkBinds(params []string) error {
	known := map[string]bool{}
	for _, p := range params {
		known[p] = true
		target := t.Binds[p]
		if target == "" {
			return fmt.Errorf("parameter %q has no entry in binds, so it would change nothing", p)
		}
		if !bindRe.MatchString(target) {
			return fmt.Errorf("binds.%s: %q is not a spec path", p, target)
		}
	}
	extra := []string{}
	for p := range t.Binds {
		if !known[p] {
			extra = append(extra, p)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		return fmt.Errorf("binds names %s which is not a parameter", strings.Join(extra, ", "))
	}
	return nil
}
