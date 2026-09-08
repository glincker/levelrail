package main

import (
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// appSpecDiff is diffAppSpecServices' result: Differences are real,
// exit-code-triggering drift; NotComparable are env vars where the
// deployed side can never carry enough information to judge (the two
// fidelity losses specServiceFromDesired's own doc comment names: a
// secret's required flag, and a { from: ... } cross-resource
// reference), reported so a caller sees them rather than getting either
// a silent false pass or a false "drifted" failure caused purely by a
// known API gap.
type appSpecDiff struct {
	Differences   []string
	NotComparable []string
}

// diffAppSpecServices compares local (parsed from a caller's app.yaml)
// against deployed (GET /api/v1/apps/{name}/spec, reconstructed by
// specServiceFromDesired): only the fields that reconstruction actually
// populates. build/hostPort/replicas/strategy/volumes/hooks are handled
// by appSpecNotTrackedFields instead, one level up: comparing them here
// would report a manufactured difference for every real app.yaml.
func diffAppSpecServices(local, deployed spec.Service) appSpecDiff {
	var d appSpecDiff

	if local.Port != deployed.Port {
		d.Differences = append(d.Differences, fmt.Sprintf("port: deployed=%d local=%d", deployed.Port, local.Port))
	}
	d.Differences = append(d.Differences, diffDomains(local.Domains, deployed.Domains)...)
	d.Differences = append(d.Differences, diffLabels(local.Labels, deployed.Labels)...)
	d.Differences = append(d.Differences, diffResources(local.Resources, deployed.Resources)...)
	d.Differences = append(d.Differences, diffHealth(local.Health, deployed.Health)...)

	envDiffs, envNotComparable := diffEnv(local.Env, deployed.Env)
	d.Differences = append(d.Differences, envDiffs...)
	d.NotComparable = append(d.NotComparable, envNotComparable...)

	return d
}

func diffDomains(local, deployed []string) []string {
	var diffs []string
	localSet, deployedSet := toStringSet(local), toStringSet(deployed)
	for _, dom := range sortedSetKeys(localSet) {
		if !deployedSet[dom] {
			diffs = append(diffs, "domain added: "+dom)
		}
	}
	for _, dom := range sortedSetKeys(deployedSet) {
		if !localSet[dom] {
			diffs = append(diffs, "domain removed: "+dom)
		}
	}
	return diffs
}

func diffLabels(local, deployed map[string]string) []string {
	var diffs []string
	for _, k := range sortedStringMapKeys(local, deployed) {
		lv, lok := local[k]
		dv, dok := deployed[k]
		switch {
		case lok && !dok:
			diffs = append(diffs, fmt.Sprintf("label %s: added (%s)", k, lv))
		case !lok && dok:
			diffs = append(diffs, fmt.Sprintf("label %s: removed (%s)", k, dv))
		case lv != dv:
			diffs = append(diffs, fmt.Sprintf("label %s: deployed=%s local=%s", k, dv, lv))
		}
	}
	return diffs
}

// diffEnv reports added/removed vars and changed literal values as real
// differences. A var that is secret on both sides is treated as equal
// regardless of Required (that flag is never preserved on the deployed
// side, see appSpecDiff's own doc comment), and a var whose local side
// is a { from: ... } reference is reported as not comparable rather
// than diffed, since the deployed side only ever holds the
// already-resolved literal value with no trace of the reference left.
func diffEnv(local, deployed map[string]spec.EnvVar) (diffs, notComparable []string) {
	for _, k := range sortedEnvMapKeys(local, deployed) {
		lv, lok := local[k]
		dv, dok := deployed[k]
		switch {
		case lok && !dok:
			diffs = append(diffs, fmt.Sprintf("env %s: added (%s)", k, envVarSummary(lv)))
		case !lok && dok:
			diffs = append(diffs, fmt.Sprintf("env %s: removed (%s)", k, envVarSummary(dv)))
		case lv.From != "":
			notComparable = append(notComparable, k)
		case lv.Secret && dv.Secret:
			// Equal regardless of Required or of dv.Value (always empty):
			// the known, documented fidelity loss, not real drift.
		case lv != dv:
			diffs = append(diffs, fmt.Sprintf("env %s: deployed=%s local=%s", k, envVarSummary(dv), envVarSummary(lv)))
		}
	}
	return diffs, notComparable
}

func envVarSummary(v spec.EnvVar) string {
	switch {
	case v.Secret:
		return "(secret)"
	case v.From != "":
		return "{ from: " + v.From + " }"
	default:
		return v.Value
	}
}

func diffResources(local, deployed *spec.Resources) []string {
	var diffs []string
	var l, d spec.Resources
	if local != nil {
		l = *local
	}
	if deployed != nil {
		d = *deployed
	}
	if l.Memory != d.Memory {
		diffs = append(diffs, fmt.Sprintf("resources.memory: deployed=%s local=%s", valueOrNone(d.Memory), valueOrNone(l.Memory)))
	}
	if l.CPU != d.CPU {
		diffs = append(diffs, fmt.Sprintf("resources.cpu: deployed=%v local=%v", d.CPU, l.CPU))
	}
	if l.SwapMemory != d.SwapMemory {
		diffs = append(diffs, fmt.Sprintf("resources.swapMemory: deployed=%s local=%s", valueOrNone(d.SwapMemory), valueOrNone(l.SwapMemory)))
	}
	if l.CPUSet != d.CPUSet {
		diffs = append(diffs, fmt.Sprintf("resources.cpuSet: deployed=%s local=%s", valueOrNone(d.CPUSet), valueOrNone(l.CPUSet)))
	}
	return diffs
}

func diffHealth(local, deployed *spec.Health) []string {
	var diffs []string
	var l, d spec.Health
	if local != nil {
		l = *local
	}
	if deployed != nil {
		d = *deployed
	}
	diffs = append(diffs, diffProbe("readiness", l.Readiness, d.Readiness)...)
	diffs = append(diffs, diffProbe("liveness", l.Liveness, d.Liveness)...)
	return diffs
}

func diffProbe(label string, local, deployed *spec.Probe) []string {
	var diffs []string
	var l, d spec.Probe
	if local != nil {
		l = *local
	}
	if deployed != nil {
		d = *deployed
	}
	if l != d {
		diffs = append(diffs, fmt.Sprintf("health.%s: deployed=%+v local=%+v", label, d, l))
	}
	return diffs
}

func valueOrNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func toStringSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	return set
}

func sortedSetKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedStringMapKeys(a, b map[string]string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	return sortedSetKeys(seen)
}

func sortedEnvMapKeys(a, b map[string]spec.EnvVar) []string {
	seen := make(map[string]bool, len(a)+len(b))
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	return sortedSetKeys(seen)
}
