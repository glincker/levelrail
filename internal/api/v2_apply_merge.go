package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ssaNull marks a field the applier sent as null: remove it from the object.
type ssaNull struct{}

// ssaConflict is one field another manager owns with a different value.
type ssaConflict struct {
	Path    string `json:"path"`
	Manager string `json:"manager"`
}

// ssaMerge is one server-side apply: current and sent are flattened specs.
type ssaMerge struct {
	current map[string]any
	sent    map[string]any
	owners  []store.ManagedField
	manager string
	force   bool
}

type ssaOutcome struct {
	merged    map[string]any
	owners    []store.ManagedField
	conflicts []ssaConflict
}

func ssaEscape(seg string) string {
	return strings.ReplaceAll(strings.ReplaceAll(seg, "~", "~0"), "/", "~1")
}

func ssaUnescape(seg string) string {
	return strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
}

// flattenSpec turns a spec into JSON-pointer leaves under /spec. Lists are atomic
// leaves and an empty map is kept as a leaf so "pooler: {}" still means something.
func flattenSpec(spec map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if err := flattenInto("/spec", spec, out, true); err != nil {
		return nil, err
	}
	return out, nil
}

func flattenInto(path string, v any, out map[string]any, root bool) error {
	switch t := v.(type) {
	case map[string]any:
		if len(t) == 0 && !root {
			out[path] = map[string]any{}
			return nil
		}
		for k, x := range t {
			if err := flattenInto(path+"/"+ssaEscape(k), x, out, false); err != nil {
				return err
			}
		}
	case map[any]any:
		return fmt.Errorf("%s: map keys must be strings", path)
	case nil:
		out[path] = ssaNull{}
	default:
		out[path] = v
	}
	return nil
}

// unflattenSpec rebuilds the spec map from leaves written by flattenSpec.
func unflattenSpec(flat map[string]any) map[string]any {
	paths := make([]string, 0, len(flat))
	for p := range flat {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	spec := map[string]any{}
	for _, p := range paths {
		if _, null := flat[p].(ssaNull); null {
			continue
		}
		segs := strings.Split(strings.TrimPrefix(p, "/spec/"), "/")
		cur := spec
		for i, s := range segs {
			key := ssaUnescape(s)
			if i == len(segs)-1 {
				cur[key] = flat[p]
				break
			}
			next, ok := cur[key].(map[string]any)
			if !ok {
				next = map[string]any{}
				cur[key] = next
			}
			cur = next
		}
	}
	return spec
}

// ssaOverlaps reports whether a and b name the same field or one contains the other.
func ssaOverlaps(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func ssaSet(flat map[string]any, path string, v any) {
	for k := range flat {
		if ssaOverlaps(k, path) {
			delete(flat, k)
		}
	}
	if _, null := v.(ssaNull); !null {
		flat[path] = v
	}
}

// ssaEqual compares through a JSON round trip so YAML ints and JSON floats match.
func ssaEqual(a, b any) bool {
	na, errA := ssaNormalize(a)
	nb, errB := ssaNormalize(b)
	return errA == nil && errB == nil && reflect.DeepEqual(na, nb)
}

func ssaNormalize(v any) (any, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (m ssaMerge) run() ssaOutcome {
	var conflicts []ssaConflict
	for p, v := range m.sent {
		_, null := v.(ssaNull)
		for _, o := range m.owners {
			if o.Manager == m.manager || !ssaOverlaps(o.Path, p) {
				continue
			}
			if !null && o.Path == p && ssaEqual(m.current[p], v) {
				continue
			}
			conflicts = append(conflicts, ssaConflict{Path: o.Path, Manager: o.Manager})
		}
	}
	if len(conflicts) > 0 && !m.force {
		sort.Slice(conflicts, func(i, j int) bool {
			if conflicts[i].Path != conflicts[j].Path {
				return conflicts[i].Path < conflicts[j].Path
			}
			return conflicts[i].Manager < conflicts[j].Manager
		})
		return ssaOutcome{conflicts: conflicts}
	}

	merged := make(map[string]any, len(m.current))
	for p, v := range m.current {
		merged[p] = v
	}
	// A field this manager dropped from its apply goes away unless someone else still owns it.
	for _, o := range m.owners {
		if o.Manager != m.manager {
			continue
		}
		if _, still := m.sent[o.Path]; still || m.ownedByOther(o.Path) {
			continue
		}
		ssaSet(merged, o.Path, ssaNull{})
	}
	for p, v := range m.sent {
		ssaSet(merged, p, v)
	}

	var owners []store.ManagedField
	for _, o := range m.owners {
		if o.Manager == m.manager || !m.keepOther(o, merged) {
			continue
		}
		owners = append(owners, o)
	}
	for p, v := range m.sent {
		if _, null := v.(ssaNull); !null {
			owners = append(owners, store.ManagedField{Path: p, Manager: m.manager})
		}
	}
	return ssaOutcome{merged: merged, owners: owners}
}

func (m ssaMerge) ownedByOther(path string) bool {
	for _, o := range m.owners {
		if o.Manager != m.manager && o.Path == path {
			return true
		}
	}
	return false
}

// keepOther keeps another manager's claim only while its field survives with the value it had.
func (m ssaMerge) keepOther(o store.ManagedField, merged map[string]any) bool {
	v, ok := merged[o.Path]
	if !ok {
		return false
	}
	for p := range m.sent {
		if ssaOverlaps(p, o.Path) && (p != o.Path || !ssaEqual(m.current[o.Path], v)) {
			return false
		}
	}
	return true
}
