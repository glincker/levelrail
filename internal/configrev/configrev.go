// Package configrev versions the revertible part of an app definition (env,
// scale, resources, domains, health) and plans a revert to an earlier version.
// Secret values never enter a snapshot: they appear as references only.
package configrev

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"

	"github.com/glincker/levelrail/internal/envredact"
	"github.com/glincker/levelrail/internal/store"
)

// Change kinds, one per revertible area.
const (
	KindEnv       = "env"
	KindSecret    = "secret"
	KindScale     = "scale"
	KindResources = "resources"
	KindDomains   = "domains"
	KindHealth    = "health"
)

// Snapshot is the stored, secret-free form of the revertible fields.
type Snapshot struct {
	// Env holds plain values only. A key the classifier calls sensitive is listed
	// in EnvMasked and its value is dropped.
	Env       map[string]string       `json:"env,omitempty"`
	EnvMasked []string                `json:"env_masked,omitempty"`
	SecretEnv []store.SecretEnvRef    `json:"secret_env,omitempty"`
	Replicas  int                     `json:"replicas"`
	Strategy  string                  `json:"strategy"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
	Health    *store.ServiceHealth    `json:"health,omitempty"`
	Domains   []string                `json:"domains,omitempty"`
}

// FieldChange is one line of a diff. From and To are never secret values: a
// masked env key shows MaskedValue, a secret reference shows "secret:NAME".
type FieldChange struct {
	Kind  string `json:"kind"`
	Field string `json:"field"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
}

// MaskedValue stands in for a withheld env value.
const MaskedValue = "[masked]"

// Capture builds the snapshot of a service.
func Capture(svc store.DesiredService) Snapshot {
	s := Snapshot{
		Replicas:  max(svc.Replicas, store.DefaultReplicas),
		Strategy:  svc.Strategy,
		Resources: svc.Resources,
		Health:    svc.Health,
		Domains:   slices.Clone(svc.Domains),
		SecretEnv: slices.Clone(svc.SecretEnv),
	}
	if s.Strategy == "" {
		s.Strategy = store.DefaultDeployStrategy
	}
	slices.Sort(s.Domains)
	slices.SortFunc(s.SecretEnv, func(a, b store.SecretEnvRef) int { return compare(a.Name, b.Name) })
	for k, v := range svc.Env {
		if sensitive, _ := envredact.Classify(k, v, nil); sensitive {
			s.EnvMasked = append(s.EnvMasked, k)
			continue
		}
		if s.Env == nil {
			s.Env = map[string]string{}
		}
		s.Env[k] = v
	}
	sort.Strings(s.EnvMasked)
	return s
}

func compare(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Equal reports whether two snapshots describe the same definition.
func Equal(a, b Snapshot) bool { return reflect.DeepEqual(norm(a), norm(b)) }

func norm(s Snapshot) Snapshot {
	if len(s.Env) == 0 {
		s.Env = nil
	}
	if len(s.Domains) == 0 {
		s.Domains = nil
	}
	if len(s.EnvMasked) == 0 {
		s.EnvMasked = nil
	}
	if len(s.SecretEnv) == 0 {
		s.SecretEnv = nil
	}
	return s
}

// Marshal and Unmarshal keep the JSON form in one place.
func Marshal(s Snapshot) (string, error) {
	b, err := json.Marshal(norm(s))
	return string(b), err
}

// Unmarshal decodes a stored snapshot.
func Unmarshal(raw string) (Snapshot, error) {
	var s Snapshot
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return s, fmt.Errorf("configrev: decode snapshot: %w", err)
	}
	return s, nil
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Diff lists what differs going from old to next.
func Diff(old, next Snapshot) []FieldChange {
	var out []FieldChange
	keys := map[string]bool{}
	for k := range old.Env {
		keys[k] = true
	}
	for k := range next.Env {
		keys[k] = true
	}
	for _, k := range sortedKeys(keys) {
		o, oOK := old.Env[k]
		n, nOK := next.Env[k]
		if oOK != nOK || o != n {
			out = append(out, FieldChange{Kind: KindEnv, Field: k, From: pick(oOK, o), To: pick(nOK, n)})
		}
	}
	for _, k := range symmetric(old.EnvMasked, next.EnvMasked) {
		out = append(out, FieldChange{Kind: KindEnv, Field: k,
			From: pick(slices.Contains(old.EnvMasked, k), MaskedValue), To: pick(slices.Contains(next.EnvMasked, k), MaskedValue)})
	}
	oldRefs, newRefs := refNames(old.SecretEnv), refNames(next.SecretEnv)
	for _, k := range symmetric(oldRefs, newRefs) {
		out = append(out, FieldChange{Kind: KindSecret, Field: k,
			From: pick(slices.Contains(oldRefs, k), "secret:"+k), To: pick(slices.Contains(newRefs, k), "secret:"+k)})
	}
	if old.Replicas != next.Replicas {
		out = append(out, FieldChange{Kind: KindScale, Field: "replicas", From: fmt.Sprint(old.Replicas), To: fmt.Sprint(next.Replicas)})
	}
	if old.Strategy != next.Strategy {
		out = append(out, FieldChange{Kind: KindScale, Field: "strategy", From: old.Strategy, To: next.Strategy})
	}
	if !reflect.DeepEqual(old.Resources, next.Resources) {
		out = append(out, FieldChange{Kind: KindResources, Field: "resources", From: jsonOf(old.Resources), To: jsonOf(next.Resources)})
	}
	for _, d := range symmetric(old.Domains, next.Domains) {
		out = append(out, FieldChange{Kind: KindDomains, Field: d,
			From: pick(slices.Contains(old.Domains, d), d), To: pick(slices.Contains(next.Domains, d), d)})
	}
	if !reflect.DeepEqual(old.Health, next.Health) {
		out = append(out, FieldChange{Kind: KindHealth, Field: "health", From: jsonOf(old.Health), To: jsonOf(next.Health)})
	}
	return out
}

func pick(ok bool, v string) string {
	if !ok {
		return ""
	}
	return v
}

func refNames(refs []store.SecretEnvRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// symmetric returns the sorted elements present in exactly one list.
func symmetric(a, b []string) []string {
	var out []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			out = append(out, x)
		}
	}
	for _, x := range b {
		if !slices.Contains(a, x) {
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// Kinds lists the distinct kinds in a diff.
func Kinds(changes []FieldChange) []string {
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Kind] = true
	}
	return sortedKeys(seen)
}

// Plan is the result of applying a target snapshot onto the live service.
type Plan struct {
	Service store.DesiredService
	// Skipped names env keys whose masked value cannot be restored.
	Skipped []string
}

// Apply returns the live service with the revertible fields set to the
// target. A masked env key keeps its current value, and is reported in Skipped
// when the target had it but the live service no longer does. Nothing outside
// the revertible fields (databases, volumes, image) is touched.
func Apply(live store.DesiredService, target Snapshot) Plan {
	svc := live
	env := map[string]string{}
	for k, v := range live.Env {
		if slices.Contains(target.EnvMasked, k) {
			env[k] = v
		}
	}
	maps.Copy(env, target.Env)
	svc.Env = env
	var skipped []string
	for _, k := range target.EnvMasked {
		if _, ok := live.Env[k]; !ok {
			skipped = append(skipped, k)
		}
	}
	svc.SecretEnv = slices.Clone(target.SecretEnv)
	svc.Replicas = target.Replicas
	svc.Strategy = target.Strategy
	svc.Resources = target.Resources
	svc.Health = target.Health
	svc.Domains = slices.Clone(target.Domains)
	return Plan{Service: svc, Skipped: skipped}
}
