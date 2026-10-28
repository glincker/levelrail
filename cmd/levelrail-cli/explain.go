package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/iac"
)

type schemaNode = map[string]any

type schemaDoc struct {
	root schemaNode
}

type explainField struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

type explainResult struct {
	Path        string         `json:"path"`
	Kind        string         `json:"kind"`
	Type        string         `json:"type"`
	Description string         `json:"description,omitempty"`
	Enum        []string       `json:"enum,omitempty"`
	Fields      []explainField `json:"fields,omitempty"`
}

func loadSchemaDoc() (*schemaDoc, error) {
	raw, err := iac.SchemaJSON()
	if err != nil {
		return nil, err
	}
	var root schemaNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse resource schema: %w", err)
	}
	return &schemaDoc{root: root}, nil
}

// Follows $ref chains inside this document; sibling keys such as description win over the target.
func (d *schemaDoc) resolve(n schemaNode) schemaNode {
	for i := 0; i < 16; i++ {
		ref, ok := n["$ref"].(string)
		if !ok || !strings.HasPrefix(ref, "#/") {
			return n
		}
		var target any = d.root
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			m, isMap := target.(schemaNode)
			if !isMap {
				return n
			}
			target = m[part]
		}
		tm, isMap := target.(schemaNode)
		if !isMap {
			return n
		}
		merged := schemaNode{}
		for k, v := range tm {
			merged[k] = v
		}
		for k, v := range n {
			if k != "$ref" {
				merged[k] = v
			}
		}
		n = merged
	}
	return n
}

func (d *schemaDoc) subschemas(n schemaNode) []schemaNode {
	out := []schemaNode{n}
	for _, key := range []string{"allOf", "anyOf", "oneOf"} {
		list, _ := n[key].([]any)
		for _, item := range list {
			if m, ok := item.(schemaNode); ok {
				out = append(out, d.resolve(m))
			}
		}
	}
	return out
}

func (d *schemaDoc) properties(n schemaNode) (map[string]schemaNode, map[string]bool) {
	props := map[string]schemaNode{}
	required := map[string]bool{}
	for _, s := range d.subschemas(n) {
		p, _ := s["properties"].(schemaNode)
		for k, v := range p {
			if m, ok := v.(schemaNode); ok {
				props[k] = d.resolve(m)
			}
		}
		req, _ := s["required"].([]any)
		for _, r := range req {
			if name, ok := r.(string); ok {
				required[name] = true
			}
		}
	}
	return props, required
}

func (d *schemaDoc) typeName(n schemaNode) string {
	switch t := n["type"].(type) {
	case string:
		if t == "array" {
			if items, ok := n["items"].(schemaNode); ok {
				return "[]" + d.typeName(d.resolve(items))
			}
			return "array"
		}
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, p := range t {
			parts = append(parts, fmt.Sprint(p))
		}
		return strings.Join(parts, "|")
	}
	if _, ok := n["enum"]; ok {
		return "enum"
	}
	if props, _ := d.properties(n); len(props) > 0 {
		return "object"
	}
	return "any"
}

func enumValues(n schemaNode) []string {
	list, _ := n["enum"].([]any)
	out := make([]string, 0, len(list))
	for _, v := range list {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

// The kind-level node is synthetic: the envelope fields plus the spec schema picked by the kind's allOf branch.
func (d *schemaDoc) kindNode(kind string) schemaNode {
	props, _ := d.root["properties"].(schemaNode)
	node := schemaNode{"type": "object", "properties": schemaNode{}}
	nodeProps := node["properties"].(schemaNode)
	for k, v := range props {
		nodeProps[k] = v
	}
	all, _ := d.root["allOf"].([]any)
	for _, entry := range all {
		em, _ := entry.(schemaNode)
		cond, _ := em["if"].(schemaNode)
		cp, _ := cond["properties"].(schemaNode)
		kp, _ := cp["kind"].(schemaNode)
		if kp["const"] != kind {
			continue
		}
		then, _ := em["then"].(schemaNode)
		tp, _ := then["properties"].(schemaNode)
		if spec, ok := tp["spec"].(schemaNode); ok {
			nodeProps["spec"] = spec
		}
		if req, ok := then["required"]; ok {
			node["required"] = req
		}
	}
	node["description"] = "Top level " + kind + " document"
	return node
}

func (d *schemaDoc) kinds() []string {
	props, _ := d.root["properties"].(schemaNode)
	kp, _ := props["kind"].(schemaNode)
	return enumValues(kp)
}

func (d *schemaDoc) explain(path string) (explainResult, error) {
	segs := strings.Split(path, ".")
	var kind string
	for _, k := range d.kinds() {
		if strings.EqualFold(k, segs[0]) {
			kind = k
		}
	}
	if kind == "" {
		return explainResult{}, newValidationError("unknown kind %q, known kinds: %s", segs[0], strings.Join(d.kinds(), ", "))
	}
	node := d.kindNode(kind)
	for i, seg := range segs[1:] {
		seg = strings.TrimSuffix(seg, "[]")
		props, _ := d.properties(d.resolve(node))
		child, ok := props[seg]
		if !ok {
			names := make([]string, 0, len(props))
			for k := range props {
				names = append(names, k)
			}
			sort.Strings(names)
			return explainResult{}, newValidationError("%s has no field %q, fields: %s", strings.Join(segs[:i+1], "."), seg, strings.Join(names, ", "))
		}
		node = child
		if items, ok := node["items"].(schemaNode); ok {
			node = d.resolve(items)
		}
	}
	node = d.resolve(node)
	res := explainResult{Path: path, Kind: kind, Type: d.typeName(d.resolve(node)), Enum: enumValues(node)}
	res.Description, _ = node["description"].(string)
	props, required := d.properties(node)
	names := make([]string, 0, len(props))
	for k := range props {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, name := range names {
		p := props[name]
		desc, _ := p["description"].(string)
		res.Fields = append(res.Fields, explainField{Name: name, Type: d.typeName(p), Required: required[name], Description: desc, Enum: enumValues(p)})
	}
	return res, nil
}

func runExplain(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" explain", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "print the result as JSON to stdout and nothing else")
	outputFlag, queryFlag := bindOutputQueryFlags(fs)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, explainUsage(prog)) }
	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	format, err := resolveOutputFormat(*jsonOut, *outputFlag)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, err)
		return exitValidation
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprint(stderr, explainUsage(prog))
		return exitUsage
	}
	doc, err := loadSchemaDoc()
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	res, err := doc.explain(fs.Arg(0))
	if err != nil {
		return reportError(stdout, stderr, *jsonOut, err)
	}
	if err := renderResult(stdout, format, *queryFlag, res, func() { printExplainHuman(stdout, res) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printExplainHuman(out io.Writer, r explainResult) {
	_, _ = fmt.Fprintf(out, "KIND:   %s\nPATH:   %s\nTYPE:   %s\n", r.Kind, r.Path, r.Type)
	if len(r.Enum) > 0 {
		_, _ = fmt.Fprintf(out, "VALUES: %s\n", strings.Join(r.Enum, ", "))
	}
	if r.Description != "" {
		_, _ = fmt.Fprintf(out, "\nDESCRIPTION:\n  %s\n", r.Description)
	}
	if len(r.Fields) == 0 {
		return
	}
	_, _ = fmt.Fprintln(out, "\nFIELDS:")
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, f := range r.Fields {
		req := ""
		if f.Required {
			req = " (required)"
		}
		_, _ = fmt.Fprintf(tw, "  %s\t<%s>%s\t%s\n", f.Name, f.Type, req, f.Description)
	}
	_ = tw.Flush()
}

func explainUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s explain <kind[.field[.field...]]> [flags]

Documents a resource kind or one of its fields from the embedded resource
schema, for example "%[1]s explain app.spec.service". Works offline.
List the kinds with "%[1]s api-resources".

Flags:
  --json            print the result as JSON, nothing else
  --output string   output format (json, yaml, table, text, jsonpath=..., and more)
  --query string    JMESPath expression to filter the result before printing
  -h, --help        show this help
`, prog)
}
