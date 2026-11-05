package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/placement"
)

// parseTolerations reads key[=value]:Effect arguments (an Equal match) and
// bare key:Effect exists-style keys from existsKeys; the server validates the rest.
func parseTolerations(args, existsKeys []string) ([]placement.Toleration, error) {
	out := make([]placement.Toleration, 0, len(args)+len(existsKeys))
	for _, a := range args {
		i := strings.LastIndex(a, ":")
		if i <= 0 || i == len(a)-1 {
			return nil, fmt.Errorf("toleration %q must look like key[=value]:Effect", a)
		}
		key, value, hasValue := strings.Cut(a[:i], "=")
		tol := placement.Toleration{Key: key, Value: value, Effect: placement.TaintEffect(a[i+1:])}
		if !hasValue {
			tol.Operator = placement.TolerationExists
		}
		out = append(out, tol)
	}
	for _, k := range existsKeys {
		out = append(out, placement.Toleration{Key: k, Operator: placement.TolerationExists})
	}
	return out, nil
}

// readAffinityFile reads an affinity rule set from a YAML or JSON file.
func readAffinityFile(path string) (*placement.Affinity, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-chosen path
	if err != nil {
		return nil, err
	}
	var a placement.Affinity
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("affinity file %s: %w", path, err)
	}
	return &a, nil
}

// formatToleration renders a toleration the way --toleration takes it.
func formatToleration(t placement.Toleration) string {
	s := t.Key
	if t.Operator != placement.TolerationExists && t.Value != "" {
		s += "=" + t.Value
	}
	if t.Operator == placement.TolerationExists && t.Key == "" {
		s = "(any taint)"
	}
	if t.Effect != "" {
		s += ":" + string(t.Effect)
	}
	return s
}
