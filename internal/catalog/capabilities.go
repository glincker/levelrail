package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
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

// kernelModuleName keeps a module name safe to join into a /sys/module path.
var kernelModuleName = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

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
	CapClassDockerAPI:   {CapTierElevated, []string{"READ_ONLY", "PROJECT"}, true},
	CapClassDevices:     {CapTierElevated, nil, true},
	CapClassHostNetwork: {CapTierElevated, nil, true},
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
	for _, mod := range m.RequiresKernelModules {
		if !kernelModuleName.MatchString(mod) {
			errs = append(errs, fmt.Errorf("requires_kernel_modules: %q must be lowercase letters, digits and underscores", mod))
		}
	}
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
		caps := classCaps(c.Class, c.Caps)
		if len(caps) == 0 && c.Class != CapClassHostNetwork {
			errs = append(errs, fmt.Errorf("%s: class %q needs caps", at, c.Class))
		}
		if len(caps) > 0 && c.Class == CapClassHostNetwork {
			errs = append(errs, fmt.Errorf("%s: class %q takes no caps", at, c.Class))
		}
		for _, d := range caps {
			if c.Class == CapClassDevices && !dockerguard.GrantableDevice(d) {
				errs = append(errs, fmt.Errorf("%s: device %q is outside the devices allowlist", at, d))
			}
		}
		if c.Class == CapClassDockerAPI && len(caps) != 1 {
			errs = append(errs, fmt.Errorf("%s: class %q takes exactly one profile (read_only or project)", at, c.Class))
		}
		if bad := capSubset(caps, spec.caps); len(bad) > 0 && c.Class != CapClassDevices {
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
		if c.Class == CapClassDockerAPI {
			if !mountsDockerSocket(f.Services[c.Service]) {
				errs = append(errs, fmt.Errorf("capability %s is declared for service %q but it does not mount /var/run/docker.sock", c.Class, c.Service))
			}
			continue
		}
		if c.Class == CapClassHostNetwork || c.Class == CapClassDevices {
			errs = append(errs, hostGrantUsage(c, f.Services[c.Service])...)
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

func mountsDockerSocket(svc compose.Service) bool {
	for _, v := range svc.Volumes {
		if v.HostPath == "/var/run/docker.sock" && v.ContainerPath == "/var/run/docker.sock" {
			return true
		}
	}
	return false
}

// classCaps normalizes a declaration's params: device paths keep their case, the rest are cap names.
func classCaps(class string, in []string) []string {
	if class != CapClassDevices {
		return normCaps(in)
	}
	out := make([]string, 0, len(in))
	for _, c := range in {
		out = append(out, strings.TrimSpace(c))
	}
	sort.Strings(out)
	return out
}

// hostGrantUsage checks that the compose service really uses a host_network or devices declaration.
func hostGrantUsage(c ManifestCapability, svc compose.Service) []error {
	var errs []error
	if c.Class == CapClassHostNetwork {
		if !strings.EqualFold(strings.TrimSpace(svc.HostAccess.NetworkMode), "host") {
			errs = append(errs, fmt.Errorf("capability %s is declared for service %q but it does not set network_mode: host", c.Class, c.Service))
		}
		return errs
	}
	used := compose.DevicePaths(svc.HostAccess.Devices)
	declared := classCaps(c.Class, c.Caps)
	for _, d := range declared {
		if !slices.Contains(used, d) {
			errs = append(errs, fmt.Errorf("capability %s declares %s for service %q but its devices does not use it", c.Class, d, c.Service))
		}
	}
	for _, d := range used {
		if !slices.Contains(declared, d) {
			errs = append(errs, fmt.Errorf("service %q maps device %s but the manifest does not declare it under capabilities", c.Service, d))
		}
	}
	return errs
}

// grantCaps gives a docker_api, host_network or devices declaration its grant tokens; other classes keep their cap names.
func grantCaps(class string, caps []string) []string {
	switch class {
	case CapClassHostNetwork:
		return []string{dockerguard.HostNetworkGrantCap}
	case CapClassDevices:
		out := make([]string, 0, len(caps))
		for _, c := range caps {
			out = append(out, dockerguard.DeviceGrantCap(c))
		}
		return out
	case CapClassDockerAPI:
	default:
		return caps
	}
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, dockerguard.GrantCapPrefix+c)
	}
	return out
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
		caps := classCaps(c.Class, c.Caps)
		if c.Class == CapClassCapAdd {
			// Validate already checks this; repeating it keeps a bypassed manifest from granting more than Review.
			for _, cp := range caps {
				if !slices.Contains(normCaps(dockerguard.ReviewTierCaps), cp) {
					return nil, fmt.Errorf("capability %s is outside the Review allowlist for %s", cp, c.Class)
				}
			}
		}
		out = append(out, CapabilityGrant{Class: c.Class, Tier: spec.tier, Service: c.Service, Reason: c.Reason, Caps: grantCaps(c.Class, caps)})
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
		out = append(out, CapabilityGrant{Class: c.Class, Tier: capClasses[c.Class].tier, Service: c.Service, Reason: c.Reason, Caps: grantCaps(c.Class, classCaps(c.Class, c.Caps))})
	}
	return out
}
