package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
)

// Capability tiers, ordered review < elevated < privileged.
const (
	CapTierReview     = "review"
	CapTierElevated   = "elevated"
	CapTierPrivileged = "privileged"
)

// Capability classes a manifest may declare. Only the first two have a grant
// path today; the rest are reserved so their names cannot be reused.
const (
	CapClassCapAdd      = "cap_add"
	CapClassNetAdmin    = "net_admin"
	CapClassHostLibs    = "host_libraries"
	CapClassDockerAPI   = "docker_api"
	CapClassDevices     = "devices"
	CapClassHostNetwork = "host_network"
	CapClassPrivileged  = "privileged"
)

// ErrCapabilityDisabled means a declared class is off in runtime settings.
var ErrCapabilityDisabled = errors.New("capability class is disabled")

type capClassSpec struct {
	tier      string
	caps      []string
	available bool
}

var capClasses = map[string]capClassSpec{
	CapClassCapAdd:      {CapTierReview, dockerguard.ReviewTierCaps, true},
	CapClassNetAdmin:    {CapTierElevated, dockerguard.GrantableCaps, true},
	CapClassHostLibs:    {CapTierReview, nil, false},
	CapClassDockerAPI:   {CapTierElevated, nil, false},
	CapClassDevices:     {CapTierElevated, nil, false},
	CapClassHostNetwork: {CapTierElevated, nil, false},
	CapClassPrivileged:  {CapTierPrivileged, nil, false},
}

// ManifestCapability is one entry of the manifest capabilities block: what is
// needed, for which service, and why. Caps carries the class parameters.
type ManifestCapability struct {
	Class   string   `yaml:"class" json:"class"`
	Service string   `yaml:"service" json:"service"`
	Reason  string   `yaml:"reason" json:"reason"`
	Caps    []string `yaml:"caps" json:"caps,omitempty"`
}

// CapabilityGrant is a validated declaration with its tier resolved.
type CapabilityGrant struct {
	Class   string   `json:"class"`
	Tier    string   `json:"tier"`
	Service string   `json:"service"`
	Reason  string   `json:"reason"`
	Caps    []string `json:"caps,omitempty"`
}

// CapabilityPolicy says which Elevated or Privileged classes are switched on.
// The caller reads runtime settings and passes the result in, so this package
// never imports them. Review classes need no entry.
type CapabilityPolicy struct {
	Enabled map[string]bool
}

func normCaps(in []string) []string {
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, dockerguard.NormalizeCap(c))
	}
	sort.Strings(out)
	return out
}

func capSubset(caps, allowed []string) []string {
	var bad []string
	for _, c := range caps {
		ok := false
		for _, a := range allowed {
			if dockerguard.NormalizeCap(a) == c {
				ok = true
				break
			}
		}
		if !ok {
			bad = append(bad, c)
		}
	}
	return bad
}

// validateCapabilityShape checks classes, fields and params without the compose body.
func (m Manifest) validateCapabilityShape() []error {
	var errs []error
	seen := map[string]bool{}
	for i, c := range m.Capabilities {
		at := fmt.Sprintf("capabilities[%d]", i)
		spec, known := capClasses[c.Class]
		switch {
		case !known:
			errs = append(errs, fmt.Errorf("%s: unknown class %q", at, c.Class))
			continue
		case spec.tier == CapTierPrivileged:
			errs = append(errs, fmt.Errorf("%s: class %q is never allowed in a community or marketplace template", at, c.Class))
			continue
		case !spec.available:
			errs = append(errs, fmt.Errorf("%s: class %q is not available yet", at, c.Class))
			continue
		}
		if c.Service == "" {
			errs = append(errs, fmt.Errorf("%s: missing service", at))
		}
		if strings.TrimSpace(c.Reason) == "" {
			errs = append(errs, fmt.Errorf("%s: missing reason", at))
		}
		caps := normCaps(c.Caps)
		if len(caps) == 0 {
			errs = append(errs, fmt.Errorf("%s: class %q needs caps", at, c.Class))
		}
		if bad := capSubset(caps, spec.caps); len(bad) > 0 {
			errs = append(errs, fmt.Errorf("%s: caps %s are outside the %s allowlist", at, strings.Join(bad, ", "), c.Class))
		}
		key := c.Class + "/" + c.Service
		if seen[key] {
			errs = append(errs, fmt.Errorf("%s: class %q declared twice for service %q", at, c.Class, c.Service))
		}
		seen[key] = true
	}
	return errs
}

// validateCapabilityUsage compares declarations with the cap_add keys the
// rendered compose actually sets, in both directions.
func (m Manifest) validateCapabilityUsage(body string) []error {
	f, err := compose.Parse([]byte(body))
	if err != nil {
		return nil
	}
	declared := map[string]map[string]bool{}
	var errs []error
	for _, c := range m.Capabilities {
		if _, ok := f.Services[c.Service]; !ok {
			errs = append(errs, fmt.Errorf("capability %s names service %q, which the compose file does not declare", c.Class, c.Service))
			continue
		}
		if declared[c.Service] == nil {
			declared[c.Service] = map[string]bool{}
		}
		for _, capName := range normCaps(c.Caps) {
			declared[c.Service][capName] = true
		}
	}
	names := make([]string, 0, len(f.Services))
	for n := range f.Services {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		used := map[string]bool{}
		for _, c := range f.Services[n].HostAccess.CapAdd {
			capName := dockerguard.NormalizeCap(c)
			used[capName] = true
			if !declared[n][capName] {
				errs = append(errs, fmt.Errorf("service %q adds capability %s in cap_add but the manifest does not declare it under capabilities", n, capName))
			}
		}
		for capName := range declared[n] {
			if !used[capName] {
				errs = append(errs, fmt.Errorf("capability %s is declared for service %q but its cap_add does not use it", capName, n))
			}
		}
	}
	return errs
}

// validateSubmissionCapabilities keeps the Privileged tier out of submissions,
// even when Validate has not run on this value.
func (m Manifest) validateSubmissionCapabilities() []error {
	var errs []error
	for i, c := range m.Capabilities {
		if spec, ok := capClasses[c.Class]; ok && spec.tier == CapTierPrivileged {
			errs = append(errs, fmt.Errorf("capabilities[%d]: class %q is never allowed in a community or marketplace template", i, c.Class))
		}
	}
	return errs
}

// CapabilityGrants resolves the declared capabilities against the policy.
// Elevated and Privileged classes fail with ErrCapabilityDisabled unless on.
func (m Manifest) CapabilityGrants(p CapabilityPolicy) ([]CapabilityGrant, error) {
	var out []CapabilityGrant
	for _, c := range m.Capabilities {
		spec, ok := capClasses[c.Class]
		if !ok || !spec.available {
			return nil, fmt.Errorf("capability class %q is not available", c.Class)
		}
		if spec.tier != CapTierReview && !p.Enabled[c.Class] {
			return nil, fmt.Errorf("%w: %s (%s tier) is off in runtime settings", ErrCapabilityDisabled, c.Class, spec.tier)
		}
		out = append(out, CapabilityGrant{Class: c.Class, Tier: spec.tier, Service: c.Service, Reason: c.Reason, Caps: normCaps(c.Caps)})
	}
	return out, nil
}

// CapGrantMap turns grants into the per-service cap_add set compose accepts.
func CapGrantMap(grants []CapabilityGrant) map[string][]string {
	if len(grants) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, g := range grants {
		out[g.Service] = append(out[g.Service], g.Caps...)
	}
	return out
}

// CapabilityDigest identifies the compose body plus declarations, for the audit event.
func (m Manifest) CapabilityDigest() string {
	raw, _ := json.Marshal(m.Capabilities)
	sum := sha256.Sum256(append([]byte(m.Compose+"\x00"), raw...))
	return hex.EncodeToString(sum[:])
}

// DeclaredCapabilities lists the manifest declarations with tiers, for the requirements card.
func (m Manifest) DeclaredCapabilities() []CapabilityGrant {
	var out []CapabilityGrant
	for _, c := range m.Capabilities {
		out = append(out, CapabilityGrant{Class: c.Class, Tier: capClasses[c.Class].tier, Service: c.Service, Reason: c.Reason, Caps: normCaps(c.Caps)})
	}
	return out
}
