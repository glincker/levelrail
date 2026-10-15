// Package policy evaluates deploy guardrail rules against what an app is
// about to run. It is pure: callers resolve the layers and the subject.
package policy

import (
	"fmt"
	"path"
	"strings"
)

// Scopes a rule set can be attached to, least to most specific.
const (
	ScopeOrg         = "org"
	ScopeProject     = "project"
	ScopeEnvironment = "environment"
	// OrgScopeID is the single organization-wide scope id.
	OrgScopeID = "org"
)

// Mode is how a rule reacts: off ignores it, warn reports it, enforce blocks.
type Mode string

// Rule modes.
const (
	ModeOff     Mode = "off"
	ModeWarn    Mode = "warn"
	ModeEnforce Mode = "enforce"
)

// Rule ids, the keys of a Set.
const (
	RuleAllowedRegistries = "allowed_registries"
	RuleRequireReadiness  = "require_readiness_probe"
	RuleMaxMemoryMB       = "max_memory_mb"
	RuleMaxCPU            = "max_cpu"
	RuleForbidPrivileged  = "forbid_privileged"
	RuleForbidHostMounts  = "forbid_host_mounts"
	RuleRequiredLabels    = "required_labels"
	RuleMaxReplicas       = "max_replicas"
)

// Stable machine readable violation codes.
const (
	CodeRegistryNotAllowed  = "policy_registry_not_allowed"
	CodeReadinessRequired   = "policy_readiness_probe_required"
	CodeMemoryCeiling       = "policy_memory_ceiling_exceeded"
	CodeCPUCeiling          = "policy_cpu_ceiling_exceeded"
	CodePrivilegedForbidden = "policy_privileged_forbidden"
	CodeHostMountForbidden  = "policy_host_mount_forbidden"
	CodeLabelRequired       = "policy_label_required"
	CodeReplicasExceeded    = "policy_replicas_exceeded"
	// CodeViolation is the response code of a blocked deploy.
	CodeViolation = "policy_violation"
)

const (
	nanoCPUsPerCore    = 1_000_000_000
	bytesPerMB         = 1 << 20
	defaultRegistry    = "docker.io"
	maxRuleValues      = 200
	maxRuleValueLength = 253
)

// Rule is one guardrail. Values carries list rules (registries, labels,
// allowed host path prefixes); Limit carries numeric ceilings.
type Rule struct {
	Mode   Mode     `json:"mode"`
	Values []string `json:"values,omitempty"`
	Limit  float64  `json:"limit,omitempty"`
}

// Set maps a rule id to its rule.
type Set map[string]Rule

// Subject is the deploy being judged. Zero MemoryBytes or NanoCPUs mean no
// limit is set, which a ceiling treats as exceeding it.
type Subject struct {
	Image        string
	MemoryBytes  int64
	NanoCPUs     int64
	HasReadiness bool
	Privileged   bool
	HostMounts   []string
	Labels       map[string]string
	Replicas     int
}

// Violation is one rule a subject breaks, with a stable code and the next
// action for a human.
type Violation struct {
	Rule       string `json:"rule"`
	Code       string `json:"code"`
	Mode       Mode   `json:"mode"`
	Message    string `json:"message"`
	NextAction string `json:"next_action"`
}

// Blocks reports whether this violation stops a deploy.
func (v Violation) Blocks() bool { return v.Mode == ModeEnforce }

// RuleIDs lists every known rule in display order.
func RuleIDs() []string {
	return []string{
		RuleAllowedRegistries, RuleRequireReadiness, RuleMaxMemoryMB, RuleMaxCPU,
		RuleForbidPrivileged, RuleForbidHostMounts, RuleRequiredLabels, RuleMaxReplicas,
	}
}

func numericRule(id string) bool {
	return id == RuleMaxMemoryMB || id == RuleMaxCPU || id == RuleMaxReplicas
}

// Validate rejects unknown rules, modes, and rules missing what they need.
func Validate(s Set) error {
	known := map[string]bool{}
	for _, id := range RuleIDs() {
		known[id] = true
	}
	for id, r := range s {
		if !known[id] {
			return fmt.Errorf("unknown policy rule %q", id)
		}
		switch r.Mode {
		case ModeOff, ModeWarn, ModeEnforce:
		default:
			return fmt.Errorf("rule %s: mode must be off, warn or enforce", id)
		}
		if r.Mode == ModeOff {
			continue
		}
		if r.Limit < 0 {
			return fmt.Errorf("rule %s: limit cannot be negative", id)
		}
		if numericRule(id) && r.Limit == 0 {
			return fmt.Errorf("rule %s: a limit above zero is required", id)
		}
		if len(r.Values) > maxRuleValues {
			return fmt.Errorf("rule %s: at most %d values", id, maxRuleValues)
		}
		for _, v := range r.Values {
			if strings.TrimSpace(v) == "" || len(v) > maxRuleValueLength {
				return fmt.Errorf("rule %s: values must be non-empty and under %d characters", id, maxRuleValueLength)
			}
		}
		if (id == RuleAllowedRegistries || id == RuleRequiredLabels) && len(r.Values) == 0 {
			return fmt.Errorf("rule %s: at least one value is required", id)
		}
	}
	return nil
}

// Merge layers rule sets from least to most specific; a later layer's rule
// replaces an earlier one with the same id, so an environment can relax or
// tighten what the organization set.
func Merge(layers ...Set) Set {
	out := Set{}
	for _, l := range layers {
		for id, r := range l {
			out[id] = r
		}
	}
	return out
}

// RegistryOf returns the registry host an image reference pulls from.
func RegistryOf(image string) string {
	ref := strings.TrimSpace(image)
	if i := strings.Index(ref, "@"); i >= 0 {
		ref = ref[:i]
	}
	first, _, found := strings.Cut(ref, "/")
	if !found {
		return defaultRegistry
	}
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return strings.ToLower(first)
	}
	return defaultRegistry
}

// Evaluate returns every violation of s by subj, in rule display order.
func Evaluate(s Set, subj Subject) []Violation {
	var out []Violation
	for _, id := range RuleIDs() {
		r, ok := s[id]
		if !ok || r.Mode == ModeOff || r.Mode == "" {
			continue
		}
		for _, v := range evalRule(id, r, subj) {
			v.Rule, v.Mode = id, r.Mode
			out = append(out, v)
		}
	}
	return out
}

// Blocking returns the violations that stop a deploy.
func Blocking(vs []Violation) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Blocks() {
			out = append(out, v)
		}
	}
	return out
}

func evalRule(id string, r Rule, s Subject) []Violation {
	switch id {
	case RuleAllowedRegistries:
		return evalRegistry(r, s)
	case RuleRequireReadiness:
		if s.HasReadiness {
			return nil
		}
		return []Violation{{Code: CodeReadinessRequired,
			Message:    "no readiness probe is configured",
			NextAction: "Add a readiness probe (path and interval) to the app's health settings, then deploy again."}}
	case RuleMaxMemoryMB:
		return evalMemory(r, s)
	case RuleMaxCPU:
		return evalCPU(r, s)
	case RuleForbidPrivileged:
		if !s.Privileged {
			return nil
		}
		return []Violation{{Code: CodePrivilegedForbidden,
			Message:    "the container asks for privileged mode",
			NextAction: "Remove privileged mode from the app, or ask an admin to override this policy with a reason."}}
	case RuleForbidHostMounts:
		return evalHostMounts(r, s)
	case RuleRequiredLabels:
		return evalLabels(r, s)
	case RuleMaxReplicas:
		n := max(s.Replicas, 1)
		if float64(n) <= r.Limit {
			return nil
		}
		return []Violation{{Code: CodeReplicasExceeded,
			Message:    fmt.Sprintf("%d replicas requested, the ceiling is %g", n, r.Limit),
			NextAction: fmt.Sprintf("Lower replicas to %g or fewer, or raise the max_replicas limit in the policy.", r.Limit)}}
	}
	return nil
}

func evalRegistry(r Rule, s Subject) []Violation {
	if s.Image == "" {
		return nil
	}
	reg := RegistryOf(s.Image)
	for _, a := range r.Values {
		if strings.EqualFold(strings.TrimSpace(a), reg) {
			return nil
		}
	}
	return []Violation{{Code: CodeRegistryNotAllowed,
		Message:    fmt.Sprintf("image registry %q is not on the allowed list", reg),
		NextAction: "Push the image to one of: " + strings.Join(r.Values, ", ") + ", or add the registry to the policy."}}
}

func evalMemory(r Rule, s Subject) []Violation {
	limit := int64(r.Limit) * bytesPerMB
	if s.MemoryBytes > 0 && s.MemoryBytes <= limit {
		return nil
	}
	msg := fmt.Sprintf("memory limit %d MB is above the %g MB ceiling", s.MemoryBytes/bytesPerMB, r.Limit)
	if s.MemoryBytes == 0 {
		msg = fmt.Sprintf("no memory limit is set, the policy requires one of at most %g MB", r.Limit)
	}
	return []Violation{{Code: CodeMemoryCeiling, Message: msg,
		NextAction: fmt.Sprintf("Set the app's memory limit to %g MB or less.", r.Limit)}}
}

func evalCPU(r Rule, s Subject) []Violation {
	limit := int64(r.Limit * nanoCPUsPerCore)
	if s.NanoCPUs > 0 && s.NanoCPUs <= limit {
		return nil
	}
	msg := fmt.Sprintf("CPU limit %g cores is above the %g core ceiling", float64(s.NanoCPUs)/nanoCPUsPerCore, r.Limit)
	if s.NanoCPUs == 0 {
		msg = fmt.Sprintf("no CPU limit is set, the policy requires one of at most %g cores", r.Limit)
	}
	return []Violation{{Code: CodeCPUCeiling, Message: msg,
		NextAction: fmt.Sprintf("Set the app's CPU limit to %g cores or less.", r.Limit)}}
}

func evalHostMounts(r Rule, s Subject) []Violation {
	var out []Violation
	for _, m := range s.HostMounts {
		if allowedMount(m, r.Values) {
			continue
		}
		out = append(out, Violation{Code: CodeHostMountForbidden,
			Message:    fmt.Sprintf("host path %q is bind mounted into the container", m),
			NextAction: "Use a managed volume instead of a host bind mount, or add the path prefix to the policy's allowed list."})
	}
	return out
}

func allowedMount(mount string, prefixes []string) bool {
	clean := path.Clean(mount)
	for _, p := range prefixes {
		p = path.Clean(strings.TrimSpace(p))
		if clean == p || strings.HasPrefix(clean, strings.TrimSuffix(p, "/")+"/") {
			return true
		}
	}
	return false
}

func evalLabels(r Rule, s Subject) []Violation {
	var missing []string
	for _, k := range r.Values {
		if strings.TrimSpace(s.Labels[k]) == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return []Violation{{Code: CodeLabelRequired,
		Message:    "required labels are missing: " + strings.Join(missing, ", "),
		NextAction: "Add these labels to the app: " + strings.Join(missing, ", ") + "."}}
}
