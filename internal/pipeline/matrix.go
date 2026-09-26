package pipeline

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// MaxMatrixCombinations bounds one job's expansion.
const MaxMatrixCombinations = 256

// Combo is one matrix combination.
type Combo map[string]string

// Expand returns the job's combinations. A nil or empty matrix yields one
// empty combination so callers treat plain and matrix jobs alike.
func (m *Matrix) Expand() ([]Combo, error) {
	if m == nil || (len(m.Vars) == 0 && len(m.Include) == 0) {
		return []Combo{{}}, nil
	}
	keys := make([]string, 0, len(m.Vars))
	for k := range m.Vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var combos []Combo
	if len(keys) > 0 {
		combos = []Combo{{}}
		for _, k := range keys {
			var next []Combo
			for _, c := range combos {
				for _, v := range m.Vars[k] {
					n := Combo{}
					for ck, cv := range c {
						n[ck] = cv
					}
					n[k] = v
					next = append(next, n)
					if len(next) > MaxMatrixCombinations {
						return nil, fmt.Errorf("matrix expands to more than %d combinations", MaxMatrixCombinations)
					}
				}
			}
			combos = next
		}
	}
	combos = slices.DeleteFunc(combos, func(c Combo) bool {
		for _, ex := range m.Exclude {
			if comboMatches(c, ex) {
				return true
			}
		}
		return false
	})
	for _, inc := range m.Include {
		combos = append(combos, Combo(inc))
	}
	if len(combos) > MaxMatrixCombinations {
		return nil, fmt.Errorf("matrix expands to more than %d combinations", MaxMatrixCombinations)
	}
	if len(combos) == 0 {
		return nil, fmt.Errorf("matrix excludes every combination")
	}
	return combos, nil
}

func comboMatches(c Combo, filter map[string]string) bool {
	for k, v := range filter {
		if c[k] != v {
			return false
		}
	}
	return len(filter) > 0
}

// ComboKey is the stable job key for a matrix instance, e.g. "test[go=1.22]".
func ComboKey(job string, c Combo) string {
	if len(c) == 0 {
		return job
	}
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + c[k]
	}
	return job + "[" + strings.Join(parts, ",") + "]"
}

// Event is what a trigger is matched against.
type Event struct {
	Kind   string
	Branch string
	Tag    string
	// Fork marks a pull request whose source is another repository.
	Fork bool
	// HeadRepo is that source repository's full name, for the trigger log.
	HeadRepo string
	// Action is a pull request's normalized action (opened, synchronize).
	Action string
	// Changed lists the files the event touched. ChangedFn, when set, is
	// called at most once to fetch the list when Changed is empty. An
	// empty result means the list is unknown, so path filters pass.
	Changed   []string
	ChangedFn func(ctx context.Context) ([]string, error)
}

// Matches reports whether the definition should run for ev.
func (t Triggers) Matches(ev Event) bool {
	switch ev.Kind {
	case TriggerPush:
		return t.Push != nil && globAny(t.Push.Branches, ev.Branch)
	case TriggerPullRequest:
		return t.PullRequest != nil && globAny(t.PullRequest.Branches, ev.Branch) && t.PullRequest.acceptsAction(ev.Action)
	case TriggerTag:
		return t.Tag != nil && globAny(t.Tag.Patterns, ev.Tag)
	case TriggerMergeGroup:
		return t.MergeGroup != nil && globAny(t.MergeGroup.Branches, ev.Branch)
	case TriggerManual:
		return t.Manual != nil
	case TriggerAPI:
		return t.API
	case TriggerSchedule:
		return len(t.Schedule) > 0
	}
	return false
}

func globAny(patterns StringList, name string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if globMatch(p, name) {
			return true
		}
	}
	return false
}

func globMatch(pattern, name string) bool {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString(".*")
				i++
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	sb.WriteString("$")
	re, err := regexp.Compile(sb.String())
	return err == nil && re.MatchString(name)
}
