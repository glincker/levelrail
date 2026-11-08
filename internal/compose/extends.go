package compose

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Keys Compose never inherits through extends.
var extendsNotInherited = []string{"depends_on", "links", "volumes_from"}

// Sequence keys Compose concatenates (base first) instead of replacing.
var extendsConcatKeys = map[string]bool{
	"ports": true, "expose": true, "env_file": true, "dns": true,
	"dns_search": true, "tmpfs": true, "external_links": true,
}

// resolveExtends merges every same-file extends: {service: x} into its
// service and returns the rewritten document. An extends with file: is left
// in place so the unsupported-key warning still fires for it.
func resolveExtends(data []byte) ([]byte, error) {
	if !bytes.Contains(data, []byte("extends")) {
		return data, nil
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return data, nil
	}
	services, _ := doc["services"].(map[string]any)
	if services == nil {
		return data, nil
	}
	r := &extendsResolver{services: services, done: map[string]map[string]any{}}
	out := make(map[string]any, len(services))
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		merged, err := r.resolve(name, nil)
		if err != nil {
			return nil, err
		}
		if merged == nil {
			out[name] = services[name]
			continue
		}
		out[name] = merged
	}
	doc["services"] = out
	res, err := yaml.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("compose: extends: %w", err)
	}
	return res, nil
}

type extendsResolver struct {
	services map[string]any
	done     map[string]map[string]any
}

func (r *extendsResolver) resolve(name string, stack []string) (map[string]any, error) {
	if m, ok := r.done[name]; ok {
		return m, nil
	}
	for _, s := range stack {
		if s == name {
			return nil, fmt.Errorf("compose: service %q: extends forms a cycle (%s)", name, strings.Join(append(stack, name), " -> "))
		}
	}
	svc, ok := r.services[name].(map[string]any)
	if !ok {
		return nil, nil
	}
	ext, isMap := svc["extends"].(map[string]any)
	if !isMap || ext["file"] != nil {
		r.done[name] = svc
		return svc, nil
	}
	parentName, _ := ext["service"].(string)
	if parentName == "" {
		return nil, fmt.Errorf("compose: service %q: extends needs a service name", name)
	}
	if _, found := r.services[parentName].(map[string]any); !found {
		return nil, fmt.Errorf("compose: service %q: extends service %q is not defined in this file", name, parentName)
	}
	parent, err := r.resolve(parentName, append(stack, name))
	if err != nil {
		return nil, err
	}
	own := make(map[string]any, len(svc))
	for k, v := range svc {
		if k != "extends" {
			own[k] = v
		}
	}
	base := make(map[string]any, len(parent))
	for k, v := range parent {
		base[k] = v
	}
	for _, k := range extendsNotInherited {
		delete(base, k)
	}
	merged := mergeServiceMaps(base, own)
	r.done[name] = merged
	return merged, nil
}

func mergeServiceMaps(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		prev, has := out[k]
		switch {
		case !has:
			out[k] = v
		case k == "command" || k == "entrypoint" || k == "test":
			out[k] = v
		case k == "environment" || k == "labels":
			out[k] = mergeKeyed(prev, v)
		case extendsConcatKeys[k]:
			out[k] = append(toList(prev), toList(v)...)
		case k == "volumes":
			out[k] = mergeVolumes(toList(prev), toList(v))
		default:
			pm, pok := prev.(map[string]any)
			vm, vok := v.(map[string]any)
			if pok && vok {
				out[k] = mergeServiceMaps(pm, vm)
			} else {
				out[k] = v
			}
		}
	}
	return out
}

func toList(v any) []any {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		return append([]any(nil), t...)
	default:
		return []any{t}
	}
}

// keyedMap reads environment:/labels: in either the map or the list form.
func keyedMap(v any) map[string]any {
	out := map[string]any{}
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			out[k] = val
		}
	case []any:
		for _, item := range t {
			s, _ := item.(string)
			key, val, hasVal := strings.Cut(s, "=")
			if hasVal {
				out[key] = val
			} else {
				out[key] = nil
			}
		}
	}
	return out
}

func mergeKeyed(base, over any) map[string]any {
	out := keyedMap(base)
	for k, v := range keyedMap(over) {
		out[k] = v
	}
	return out
}

func volumeTarget(v any) string {
	switch t := v.(type) {
	case string:
		parts := strings.Split(t, ":")
		if len(parts) >= 2 {
			return parts[1]
		}
		return parts[0]
	case map[string]any:
		s, _ := t["target"].(string)
		return s
	}
	return ""
}

func mergeVolumes(base, over []any) []any {
	out := append([]any(nil), base...)
	for _, v := range over {
		target := volumeTarget(v)
		replaced := false
		for i, existing := range out {
			if target != "" && volumeTarget(existing) == target {
				out[i] = v
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, v)
		}
	}
	return out
}
