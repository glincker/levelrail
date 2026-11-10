package iac

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
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

// defaultPlanValueMax is the seed for APP_IAC_PLAN_VALUE_MAX.
const defaultPlanValueMax = 120

var secretPathWords = []string{"TOKEN", "SECRET", "PASSW", "CREDENTIAL", "PRIVATE", "APIKEY", "API_KEY", "ACCESS_KEY", "_KEY"}

var planValueLive atomic.Pointer[func() int]

// SetPlanValueSource makes PlanValueMax follow a live setting.
func SetPlanValueSource(fn func() int) { planValueLive.Store(&fn) }

// PlanValueMax is the longest before or after value a plan shows.
func PlanValueMax() int {
	if p := planValueLive.Load(); p != nil {
		return (*p)()
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_IAC_PLAN_VALUE_MAX"))); err == nil && n > 0 {
		return n
	}
	return defaultPlanValueMax
}

// hiddenPath reports env vars, anything under secrets and keys that look
// like credentials, so a plan never carries their values.
func hiddenPath(path string) bool {
	if strings.HasPrefix(path, "env.") || strings.HasPrefix(path, "secrets") {
		return true
	}
	// connections, secretRef and secretEnv hold names and references, never values.
	if strings.HasPrefix(path, "connections.") || strings.Contains(path, "secretRef") || strings.HasPrefix(path, "secretEnv[") {
		return false
	}
	up := strings.ToUpper(path)
	for _, w := range secretPathWords {
		if strings.Contains(up, w) {
			return true
		}
	}
	return false
}

func truncateValue(val string, limit int) string {
	r := []rune(val)
	if len(r) <= limit {
		return val
	}
	return string(r[:limit]) + "..."
}

func shown(path, val string) string {
	if hiddenPath(path) {
		return hiddenValue
	}
	return truncateValue(val, PlanValueMax())
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
