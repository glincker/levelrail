package apptype

import (
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/policy"
)

// Violation codes. They share the policy.Violation shape so every surface
// that renders deploy policy violations renders these too.
const (
	CodeWorkloadMismatch   = "apptype_workload_mismatch"
	CodeParamInvalid       = "apptype_param_invalid"
	CodeBuildNotAllowed    = "apptype_build_not_allowed"
	CodeAddonNotAllowed    = "apptype_addon_not_allowed"
	CodeFixedValueChanged  = "apptype_fixed_value_changed"
	CodeFixedValueMissing  = "apptype_fixed_value_missing"
	CodeReplicasOutOfRange = "apptype_replicas_out_of_range"
	CodeReadinessRequired  = "apptype_readiness_required"
)

// Spec is the slice of an app the checks look at. Values holds flat dotted
// spec paths (for example labels.team) for fixed-value rules.
type Spec struct {
	Workload  string            `json:"workload,omitempty"`
	EnvKind   string            `json:"env_kind,omitempty"`
	Params    map[string]any    `json:"params,omitempty"`
	Build     string            `json:"build,omitempty"`
	Replicas  *int              `json:"replicas,omitempty"`
	Readiness bool              `json:"readiness,omitempty"`
	Databases []string          `json:"databases,omitempty"`
	Volumes   int               `json:"volumes,omitempty"`
	Domains   []string          `json:"domains,omitempty"`
	Cron      string            `json:"cron,omitempty"`
	Values    map[string]string `json:"values,omitempty"`
}

type collector struct {
	t   AppType
	out []policy.Violation
}

func (c *collector) add(rule, code, msg, next string) {
	if custom := c.t.Messages[rule]; custom != "" {
		msg = custom
	}
	mode := c.t.Enforcement
	if mode == "" {
		mode = policy.ModeWarn
	}
	c.out = append(c.out, policy.Violation{Rule: rule, Code: code, Mode: mode, Message: msg, NextAction: next})
}

// Check evaluates one app spec against one published type version. It
// returns every violation, never stops at the first.
func Check(t AppType, spec Spec) []policy.Violation {
	c := &collector{t: t}
	if spec.Workload != "" && spec.Workload != t.Workload {
		c.add("workload", CodeWorkloadMismatch,
			fmt.Sprintf("This app is a %s workload but type %s is for %s workloads.", spec.Workload, t.Name, t.Workload),
			"Pick a type with the matching workload or change the app.")
	}
	c.params(spec)
	c.build(spec)
	c.addons(spec)
	c.fixed(spec)
	c.environment(spec)
	return c.out
}

func (c *collector) params(spec Spec) {
	schema, err := ParseSchema(c.t.Parameters, LimitsFromEnv())
	if err != nil {
		c.add("parameters", CodeParamInvalid, "The type's parameter schema cannot be read: "+err.Error(), "Ask the type owner to republish it.")
		return
	}
	params := spec.Params
	if params == nil {
		params = map[string]any{}
	}
	for _, msg := range schema.Validate("params", params) {
		c.add("parameters", CodeParamInvalid, msg+".", "Fix the value in the app's type parameters.")
	}
}

func (c *collector) build(spec Spec) {
	allowed := c.t.Build.Allowed
	if spec.Build == "" || len(allowed) == 0 {
		return
	}
	for _, a := range allowed {
		if a == spec.Build {
			return
		}
	}
	c.add("build.allowed", CodeBuildNotAllowed,
		fmt.Sprintf("Build method %s is not allowed by type %s. Allowed: %s.", spec.Build, c.t.Name, strings.Join(allowed, ", ")),
		"Switch the build method to one of the allowed ones.")
}

func (c *collector) addons(spec Spec) {
	a := c.t.Addons
	if d := a.Databases; d != nil {
		for _, e := range spec.Databases {
			if len(d.AllowedEngines) > 0 && !contains(d.AllowedEngines, e) {
				c.add("addons.databases", CodeAddonNotAllowed,
					fmt.Sprintf("Database engine %s is not allowed. Allowed: %s.", e, strings.Join(d.AllowedEngines, ", ")),
					"Use an allowed database engine.")
			}
		}
		if d.Max > 0 && len(spec.Databases) > d.Max {
			c.add("addons.databases", CodeAddonNotAllowed,
				fmt.Sprintf("%d databases requested, this type allows at most %d.", len(spec.Databases), d.Max),
				"Remove databases until the app is within the limit.")
		}
	}
	if a.Volumes != nil && !a.Volumes.Allowed && spec.Volumes > 0 {
		c.add("addons.volumes", CodeAddonNotAllowed, "Volumes are not allowed by this type.", "Remove the volumes or pick a type that allows them.")
	}
	if a.Cron != nil && !a.Cron.Allowed && spec.Cron != "" {
		c.add("addons.cron", CodeAddonNotAllowed, "Cron schedules are not allowed by this type.", "Remove the schedule or pick a cron type.")
	}
	if d := a.Domains; d != nil {
		c.domains(d, spec.Domains)
	}
}

func (c *collector) domains(d *DomainRule, domains []string) {
	for _, dom := range domains {
		if !d.Allowed {
			c.add("addons.domains", CodeAddonNotAllowed, "Custom domains are not allowed by this type.", "Remove the domain.")
			return
		}
		if len(d.Suffixes) == 0 {
			continue
		}
		ok := false
		for _, s := range d.Suffixes {
			ok = ok || strings.HasSuffix(dom, s)
		}
		if !ok {
			c.add("addons.domains", CodeAddonNotAllowed,
				fmt.Sprintf("Domain %s must end with one of: %s.", dom, strings.Join(d.Suffixes, ", ")),
				"Use a domain under an allowed suffix.")
		}
	}
}

func (c *collector) fixed(spec Spec) {
	keys := make([]string, 0, len(c.t.Fixed))
	for k := range c.t.Fixed {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		want, got := c.t.Fixed[k], spec.Values[k]
		switch {
		case want == "required" && got == "":
			c.add("fixed."+k, CodeFixedValueMissing, fmt.Sprintf("%s must be set by this type.", k), fmt.Sprintf("Set %s on the app.", k))
		case want != "required" && got != want:
			c.add("fixed."+k, CodeFixedValueChanged,
				fmt.Sprintf("%s is fixed to %q by type %s, the app has %q.", k, want, c.t.Name, got),
				fmt.Sprintf("Set %s to %q or remove the override.", k, want))
		}
	}
}

func (c *collector) environment(spec Spec) {
	rule, ok := c.t.EnvironmentConfig[spec.EnvKind]
	if !ok {
		return
	}
	if spec.Replicas != nil {
		if rule.ReplicasMin != nil && *spec.Replicas < *rule.ReplicasMin {
			c.add("environment."+spec.EnvKind+".replicas", CodeReplicasOutOfRange,
				fmt.Sprintf("%s needs at least %d replicas, the app has %d.", spec.EnvKind, *rule.ReplicasMin, *spec.Replicas),
				"Raise the replica count.")
		}
		if rule.ReplicasMax != nil && *spec.Replicas > *rule.ReplicasMax {
			c.add("environment."+spec.EnvKind+".replicas", CodeReplicasOutOfRange,
				fmt.Sprintf("%s allows at most %d replicas, the app has %d.", spec.EnvKind, *rule.ReplicasMax, *spec.Replicas),
				"Lower the replica count.")
		}
	}
	if rule.ReadinessRequired && !spec.Readiness {
		c.add("environment."+spec.EnvKind+".readiness", CodeReadinessRequired,
			fmt.Sprintf("%s requires a readiness check.", spec.EnvKind), "Add a readiness check to the app.")
	}
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
