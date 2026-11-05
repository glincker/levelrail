package placement

import (
	"fmt"
	"strings"
)

// TaintEffect says what an untolerated taint does to a placement.
type TaintEffect string

// Taint effects. NoExecute filters like NoSchedule here; moving workloads
// already running on the node is left to the drain flow.
const (
	EffectNoSchedule       TaintEffect = "NoSchedule"
	EffectPreferNoSchedule TaintEffect = "PreferNoSchedule"
	EffectNoExecute        TaintEffect = "NoExecute"
)

// Toleration operators.
const (
	TolerationEqual  = "Equal"
	TolerationExists = "Exists"
)

// Taint marks a node so only workloads that tolerate it are placed there.
type Taint struct {
	Key    string      `json:"key" yaml:"key"`
	Value  string      `json:"value,omitempty" yaml:"value,omitempty"`
	Effect TaintEffect `json:"effect" yaml:"effect"`
}

// Toleration lets a workload ignore matching taints. An empty Effect matches
// every effect and an empty Key with the Exists operator matches every taint.
type Toleration struct {
	Key      string      `json:"key,omitempty" yaml:"key,omitempty"`
	Operator string      `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value    string      `json:"value,omitempty" yaml:"value,omitempty"`
	Effect   TaintEffect `json:"effect,omitempty" yaml:"effect,omitempty"`
}

// String renders the taint as key=value:Effect.
func (t Taint) String() string {
	if t.Value == "" {
		return t.Key + ":" + string(t.Effect)
	}
	return t.Key + "=" + t.Value + ":" + string(t.Effect)
}

// Tolerates reports whether the toleration covers the taint.
func (tol Toleration) Tolerates(t Taint) bool {
	if tol.Effect != "" && tol.Effect != t.Effect {
		return false
	}
	switch tol.Operator {
	case TolerationExists:
		return tol.Key == "" || tol.Key == t.Key
	case "", TolerationEqual:
		return tol.Key == t.Key && tol.Value == t.Value
	}
	return false
}

// untolerated splits the taints no toleration covers into those that block
// the placement and the count that only lowers a node's rank.
func untolerated(taints []Taint, tols []Toleration) (hard []Taint, soft int) {
	for _, t := range taints {
		covered := false
		for _, tol := range tols {
			if tol.Tolerates(t) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		if t.Effect == EffectPreferNoSchedule {
			soft++
			continue
		}
		hard = append(hard, t)
	}
	return hard, soft
}

func taintReason(hard []Taint) string {
	parts := make([]string, len(hard))
	for i, t := range hard {
		parts[i] = t.String()
	}
	return "tainted and not tolerated: " + strings.Join(parts, ", ")
}

// ValidateTaints checks keys, values, effects and the set size.
func ValidateTaints(taints []Taint) error {
	if len(taints) > maxLabels {
		return fmt.Errorf("at most %d taints are allowed, got %d", maxLabels, len(taints))
	}
	seen := map[string]bool{}
	for _, t := range taints {
		if !labelKeyRE.MatchString(t.Key) {
			return fmt.Errorf("taint key %q must be lowercase letters, digits, '.', '_', '-' or '/', 1 to 63 characters", t.Key)
		}
		if t.Value != "" && !labelValueRE.MatchString(t.Value) {
			return fmt.Errorf("taint %q value %q must be letters, digits, '.', '_' or '-', 1 to 63 characters", t.Key, t.Value)
		}
		switch t.Effect {
		case EffectNoSchedule, EffectPreferNoSchedule, EffectNoExecute:
		default:
			return fmt.Errorf("taint %q effect %q must be NoSchedule, PreferNoSchedule or NoExecute", t.Key, t.Effect)
		}
		id := t.Key + ":" + string(t.Effect)
		if seen[id] {
			return fmt.Errorf("taint %q is listed twice with effect %s", t.Key, t.Effect)
		}
		seen[id] = true
	}
	return nil
}

// ValidateTolerations checks operators and the set size.
func ValidateTolerations(tols []Toleration) error {
	if len(tols) > maxLabels {
		return fmt.Errorf("at most %d tolerations are allowed, got %d", maxLabels, len(tols))
	}
	for _, tol := range tols {
		switch tol.Operator {
		case "", TolerationEqual:
			if tol.Key == "" {
				return fmt.Errorf("a toleration with operator Equal needs a key")
			}
		case TolerationExists:
			if tol.Value != "" {
				return fmt.Errorf("toleration %q uses Exists, which takes no value", tol.Key)
			}
		default:
			return fmt.Errorf("toleration operator %q must be Equal or Exists", tol.Operator)
		}
		switch tol.Effect {
		case "", EffectNoSchedule, EffectPreferNoSchedule, EffectNoExecute:
		default:
			return fmt.Errorf("toleration effect %q must be NoSchedule, PreferNoSchedule or NoExecute", tol.Effect)
		}
	}
	return nil
}
