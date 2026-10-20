package iac

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Field operations shown in a plan.
const (
	OpAdd    = "add"
	OpChange = "change"
	OpRemove = "remove"
	OpKeep   = "keep"
)

const hiddenValue = "(hidden)"

// FieldChange is one leaf difference. Values under env are never shown.
type FieldChange struct {
	Path string `json:"path"`
	Op   string `json:"op"`
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

func flatten(prefix string, v any, additive bool, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flatten(p, child, additive, out)
		}
	case map[string]string:
		flatten(prefix, toMap(t), additive, out)
	case []any:
		if additive {
			for _, e := range t {
				out[fmt.Sprintf("%s[%v]", prefix, e)] = fmt.Sprint(e)
			}
			return
		}
		raw, _ := json.Marshal(t)
		out[prefix] = string(raw)
	case nil:
	default:
		out[prefix] = fmt.Sprint(t)
	}
}

func flattenFields(fields map[string]any, additiveKeys map[string]bool) map[string]string {
	out := map[string]string{}
	for k, v := range fields {
		flatten(k, v, additiveKeys[k], out)
	}
	return out
}

func topKey(path string) string {
	if i := strings.IndexAny(path, ".["); i >= 0 {
		return path[:i]
	}
	return path
}

func shown(path, val string) string {
	if strings.HasPrefix(path, "env.") {
		return hiddenValue
	}
	return val
}

// diffFields compares desired with live. Fields under an additive key only
// gain entries: live extras are reported as kept, or as removals when
// prune is set. Every other live field the desired side omits is removed.
func diffFields(desired, live map[string]any, additiveKeys map[string]bool, prune bool) (changes, kept []FieldChange) {
	d := flattenFields(desired, additiveKeys)
	l := flattenFields(live, additiveKeys)
	for _, p := range sortedKeys(d) {
		old, ok := l[p]
		switch {
		case !ok:
			changes = append(changes, FieldChange{Path: p, Op: OpAdd, New: shown(p, d[p])})
		case old != d[p]:
			changes = append(changes, FieldChange{Path: p, Op: OpChange, Old: shown(p, old), New: shown(p, d[p])})
		}
	}
	for _, p := range sortedKeys(l) {
		if _, ok := d[p]; ok {
			continue
		}
		fc := FieldChange{Path: p, Op: OpRemove, Old: shown(p, l[p])}
		switch {
		case !additiveKeys[topKey(p)]:
			changes = append(changes, fc)
		case prune:
			changes = append(changes, fc)
		default:
			fc.Op = OpKeep
			kept = append(kept, fc)
		}
	}
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes, kept
}

func addAll(fields map[string]any, additiveKeys map[string]bool) []FieldChange {
	flat := flattenFields(fields, additiveKeys)
	out := make([]FieldChange, 0, len(flat))
	for _, p := range sortedKeys(flat) {
		out = append(out, FieldChange{Path: p, Op: OpAdd, New: shown(p, flat[p])})
	}
	return out
}
