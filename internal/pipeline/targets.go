package pipeline

import (
	"slices"
	"sort"
)

// TargetApps returns the literal app or service names the definition's
// deploy, promote, rollback, and notify steps act on, excluding the
// pipeline's own app. Validation guarantees these are literals.
func TargetApps(def *Definition, ownApp string) []string {
	set := map[string]bool{}
	for _, job := range def.Jobs {
		for _, s := range job.Steps {
			if !slices.Contains([]string{KindDeploy, KindPromote, KindRollback, KindNotify}, s.Kind()) {
				continue
			}
			for _, k := range []string{"service", "from", "to", "app"} {
				if v := s.With[k]; v != "" && v != ownApp {
					set[v] = true
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
