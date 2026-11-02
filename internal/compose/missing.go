package compose

import (
	"regexp"
	"sort"
)

// MissingVar is one required ${NAME:?msg} variable with no value.
type MissingVar struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

var requiredVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:?)\?([^}]*)\}`)

// MissingRequired lists every required variable in env values that vars
// does not satisfy, so a caller can report them all at once. Names and the
// author's message only, never values.
func MissingRequired(f *File, vars map[string]string) []MissingVar {
	seen := map[string]MissingVar{}
	for _, svc := range f.Services {
		for _, val := range svc.Environment {
			for _, m := range requiredVarPattern.FindAllStringSubmatch(val, -1) {
				cur, ok := vars[m[1]]
				if ok && (m[2] != ":" || cur != "") {
					continue
				}
				if _, dup := seen[m[1]]; !dup {
					seen[m[1]] = MissingVar{Name: m[1], Message: m[3]}
				}
			}
		}
	}
	out := make([]MissingVar, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
