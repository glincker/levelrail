package repolayout

import (
	"sort"

	"github.com/GLINCKER/levelrail/kit/pathfilter"
)

// Affected returns the directories of the packages whose watch paths match any
// changed file. Callers pass both sides of a rename. An empty diff affects
// nothing, so a no-op push never rebuilds every service.
func Affected(changed []string, pkgs []Package) []string {
	out := []string{}
	if len(changed) == 0 {
		return out
	}
	for _, p := range pkgs {
		f := pathfilter.Filter{Paths: p.WatchPaths}
		for _, c := range changed {
			if f.Apply([]string{c}).Run {
				out = append(out, p.Dir)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}
