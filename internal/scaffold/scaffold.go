// Package scaffold turns a valid app.yaml into a commented starter, taking
// every comment from the embedded app schema so docs and output cannot drift.
package scaffold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// optionalFields are the service fields most users want to find in a starter.
var optionalFields = []string{"port", "env", "health", "domains", "replicas", "resources"}

// optionalSamples are example values for the commented-out fields; the
// descriptions and defaults still come from the schema.
var optionalSamples = map[string]string{
	"port":      "8080",
	"env":       "{ LOG_LEVEL: info }",
	"health":    "{ readiness: { path: /, interval: 5s, timeout: 2s } }",
	"domains":   "[app.example.com]",
	"replicas":  "2",
	"resources": "{ memory: 512Mi, cpu: 0.5 }",
}

type schemaNode map[string]any

// Commented returns body with a one line comment above each field and the
// common optional service fields added as commented-out lines.
func Commented(body string) (string, error) {
	raw, err := spec.SchemaJSON()
	if err != nil {
		return "", err
	}
	return render(body, raw)
}

func render(body string, schemaJSON []byte) (string, error) {
	var root schemaNode
	if err := json.Unmarshal(schemaJSON, &root); err != nil {
		return "", fmt.Errorf("scaffold: parse schema: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil {
		return "", fmt.Errorf("scaffold: parse yaml: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return "", fmt.Errorf("scaffold: empty document")
	}
	walk(doc.Content[0], root, root, 0)

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("scaffold: encode: %w", err)
	}
	return buf.String(), nil
}

// walk annotates mapping keys. depth 2 is a service mapping, where the
// optional fields are appended.
func walk(n *yaml.Node, schema schemaNode, root schemaNode, depth int) {
	schema = resolve(schema, root)
	if n.Kind != yaml.MappingNode {
		return
	}
	// Flow style cannot carry per-key comments, so expand to block style.
	n.Style &^= yaml.FlowStyle
	props, _ := schema["properties"].(map[string]any)
	extra, _ := schema["additionalProperties"].(map[string]any)
	present := map[string]bool{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		present[k.Value] = true
		var child schemaNode
		if p, ok := props[k.Value].(map[string]any); ok {
			child = p
		} else if extra != nil {
			child = extra
		}
		if child == nil {
			continue
		}
		if c := fieldComment(child); c != "" && props[k.Value] != nil {
			k.HeadComment = c
		}
		walk(v, child, root, depth+1)
	}
	if depth == 2 && len(n.Content) > 0 {
		if foot := optionalBlock(props, present); foot != "" {
			n.Content[len(n.Content)-2].FootComment = foot
		}
	}
}

func optionalBlock(props map[string]any, present map[string]bool) string {
	var lines []string
	for _, name := range optionalFields {
		p, ok := props[name].(map[string]any)
		if !ok || present[name] {
			continue
		}
		if c := fieldComment(p); c != "" {
			lines = append(lines, c)
		}
		lines = append(lines, fmt.Sprintf("# %s: %s", name, optionalSamples[name]))
	}
	return strings.Join(lines, "\n")
}

// fieldComment builds the comment text for one schema property: its
// description, then its default when the schema declares one.
func fieldComment(s schemaNode) string {
	desc := clean(fmt.Sprint(s["description"]))
	if s["description"] == nil {
		desc = ""
	}
	var lines []string
	if desc != "" {
		lines = append(lines, "# "+desc)
	}
	if d, ok := s["default"]; ok {
		lines = append(lines, fmt.Sprintf("# default: %v", d))
	}
	return strings.Join(lines, "\n")
}

// clean puts a description on one line and keeps dashes out of the output.
func clean(s string) string {
	s = strings.NewReplacer(string(rune(0x2014)), ",", string(rune(0x2013)), ",").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func resolve(s schemaNode, root schemaNode) schemaNode {
	for i := 0; i < 8; i++ {
		ref, ok := s["$ref"].(string)
		if !ok {
			return s
		}
		name := strings.TrimPrefix(ref, "#/$defs/")
		defs, _ := root["$defs"].(map[string]any)
		next, ok := defs[name].(map[string]any)
		if !ok {
			return s
		}
		s = merge(s, next)
	}
	return s
}

// merge keeps the referring node's description and default over the target's.
func merge(ref, target schemaNode) schemaNode {
	out := schemaNode{}
	for k, v := range target {
		out[k] = v
	}
	for k, v := range ref {
		if k != "$ref" {
			out[k] = v
		}
	}
	return out
}
