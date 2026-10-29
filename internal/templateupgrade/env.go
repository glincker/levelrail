package templateupgrade

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Env actions. Values are never reported, only names.
const (
	EnvAdd      = "add"
	EnvRemove   = "remove"
	EnvUpdate   = "update"
	EnvKeepUser = "keep_user"
)

// EnvChange is one variable's planned fate. Generated marks a value the
// platform creates (a magic variable), Required one the template leaves
// empty for the operator.
type EnvChange struct {
	Name      string `json:"name"`
	Action    string `json:"action"`
	Generated bool   `json:"generated,omitempty"`
	Required  bool   `json:"required,omitempty"`
}

func planEnv(inst, tpl compose.Service, base *compose.Service, hasBaseline bool, sig *signals) []EnvChange {
	names := map[string]struct{}{}
	for n := range inst.Environment {
		names[n] = struct{}{}
	}
	for n := range tpl.Environment {
		names[n] = struct{}{}
	}
	sorted := make([]string, 0, len(names))
	for n := range names {
		sorted = append(sorted, n)
	}
	sort.Strings(sorted)

	var out []EnvChange
	for _, n := range sorted {
		iv, inInst := inst.Environment[n]
		tv, inTpl := tpl.Environment[n]
		var bv string
		var inBase bool
		if base != nil {
			bv, inBase = base.Environment[n]
		}
		generated := len(compose.FindMagicVars(tv)) > 0
		switch {
		case inTpl && !inInst:
			out = append(out, EnvChange{Name: n, Action: EnvAdd, Generated: generated, Required: tv == ""})
			sig.otherChange = true
		case !inTpl && inInst:
			if hasBaseline && !inBase {
				continue // the user's own variable
			}
			out = append(out, EnvChange{Name: n, Action: EnvRemove})
			sig.removedEnv = true
		case generated || iv == tv:
			continue
		case hasBaseline && inBase && iv == bv:
			out = append(out, EnvChange{Name: n, Action: EnvUpdate})
			sig.otherChange = true
		default:
			out = append(out, EnvChange{Name: n, Action: EnvKeepUser})
		}
	}
	return out
}
