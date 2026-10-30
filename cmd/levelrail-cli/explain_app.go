package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

const appKind = "app.yaml"

func loadAppDoc() (*schemaDoc, bool) {
	raw, err := spec.SchemaJSON()
	if err != nil {
		return nil, false
	}
	var root schemaNode
	if json.Unmarshal(raw, &root) != nil {
		return nil, false
	}
	return &schemaDoc{root: root}, true
}

// child returns the schema for one path segment. A map keyed by user chosen
// names (services, env) consumes the segment as the key.
func (d *schemaDoc) child(node schemaNode, seg string) (schemaNode, bool) {
	node = d.resolve(node)
	if props, _ := d.properties(node); props != nil {
		if c, ok := props[seg]; ok {
			return c, true
		}
	}
	if ap, ok := node["additionalProperties"].(schemaNode); ok {
		return d.resolve(ap), true
	}
	if items, ok := node["items"].(schemaNode); ok {
		return d.child(items, seg)
	}
	return nil, false
}

func (d *schemaDoc) walkPath(start schemaNode, segs []string) (schemaNode, bool) {
	node := start
	for _, seg := range segs {
		c, ok := d.child(node, strings.TrimSuffix(seg, "[]"))
		if !ok {
			return nil, false
		}
		node = c
	}
	return node, true
}

// explainAppPath resolves an app.yaml path; a bare field name such as
// "replicas" is looked up on a service.
func explainAppPath(path string) (explainResult, bool) {
	d, ok := loadAppDoc()
	if !ok || path == "" {
		return explainResult{}, false
	}
	if strings.EqualFold(path, "appspec") {
		return d.result(path, appKind, d.root), true
	}
	segs := strings.Split(path, ".")
	if node, ok := d.walkPath(d.root, segs); ok {
		return d.result(path, appKind, node), true
	}
	svc := schemaNode{"$ref": "#/$defs/service"}
	if node, ok := d.walkPath(svc, segs); ok {
		return d.result(path, appKind, node), true
	}
	return explainResult{}, false
}

// appPathError suggests the closest known field when the first segment is not
// a resource kind either; it returns nil so the kind error stays for kind-like input.
func appPathError(path string, kinds []string) error {
	first := strings.Split(path, ".")[0]
	for _, k := range kinds {
		if strings.EqualFold(k, first) {
			return nil
		}
	}
	d, ok := loadAppDoc()
	if !ok {
		return nil
	}
	cands := map[string]string{}
	d.collect(d.root, "", 0, cands)
	d.collect(schemaNode{"$ref": "#/$defs/service"}, "services.<name>.", 0, cands)
	last := strings.ToLower(path[strings.LastIndex(path, ".")+1:])
	best, bestDist := "", 4
	keys := make([]string, 0, len(cands))
	for k := range cands {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if dist := editDistance(last, strings.ToLower(k[strings.LastIndex(k, ".")+1:])); dist < bestDist {
			best, bestDist = cands[k], dist
		}
	}
	if best == "" {
		return newValidationError("unknown path %q, try %q for the top level fields or %q for a kind", path, "appspec", "api-resources")
	}
	return newValidationError("unknown path %q, did you mean %q?", path, best)
}

func (d *schemaDoc) collect(node schemaNode, prefix string, depth int, out map[string]string) {
	if depth > 4 {
		return
	}
	props, _ := d.properties(d.resolve(node))
	for name, p := range props {
		full := prefix + name
		out[full] = full
		d.collect(p, full+".", depth+1, out)
		if ap, ok := d.resolve(p)["additionalProperties"].(schemaNode); ok {
			d.collect(ap, full+".<name>.", depth+1, out)
		}
	}
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}
