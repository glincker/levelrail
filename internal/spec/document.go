package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// Issue is one problem found in an app.yaml document, located by dotted
// path and source line when known. Mirrors internal/pipeline.Issue's
// shape, the live-validation convention this codebase already follows
// for its own declarative YAML format.
type Issue struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ValidateDocument runs the same two layers Parse does (schema, then
// Validate's semantic rules) but never stops at the first problem: every
// schema violation comes back at once, located by path and line, for a
// live editor to annotate while the operator is still typing. The
// returned Spec is non-nil only when there are no issues. Deploy time
// still calls Parse directly; this exists only to give that same check
// friendlier, earlier feedback, not a second source of truth.
func ValidateDocument(data []byte) (*Spec, []Issue) {
	var root yaml.Node
	if err := yaml.NewDecoder(bytes.NewReader(data)).Decode(&root); err != nil {
		return nil, []Issue{{Message: "invalid yaml: " + strings.TrimPrefix(err.Error(), "yaml: ")}}
	}

	issues, err := documentSchemaIssues(&root)
	if err != nil {
		return nil, []Issue{{Message: err.Error()}}
	}
	if len(issues) > 0 {
		return nil, issues
	}

	var s Spec
	if err := yamlUnmarshalStrict(data, &s); err != nil {
		return nil, []Issue{{Message: strings.TrimPrefix(err.Error(), "spec: parse: ")}}
	}
	if err := s.Validate(); err != nil {
		return nil, []Issue{semanticIssue(err)}
	}
	return &s, nil
}

// semanticIssue extracts a best-effort dotted path from a Validate
// error, whose messages consistently start "spec: service \"name\": ..."
// or "spec: database \"name\": ...": enough for a live editor to point
// at the right block even though Validate stops at its first error
// rather than collecting a list the way the schema check above does.
func semanticIssue(err error) Issue {
	msg := strings.TrimPrefix(err.Error(), "spec: ")
	for _, kind := range []string{"service", "database"} {
		withQuote := kind + ` "`
		if !strings.HasPrefix(msg, withQuote) {
			continue
		}
		rest := strings.TrimPrefix(msg, withQuote)
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		name := rest[:end]
		detail := strings.TrimPrefix(rest[end+1:], ": ")
		return Issue{Path: kind + "s." + name, Message: detail}
	}
	return Issue{Message: msg}
}

// documentSchemaIssues mirrors internal/pipeline's own schemaIssues: walk
// the compiled schema's basic output so every violation is reported, not
// just the first.
func documentSchemaIssues(root *yaml.Node) ([]Issue, error) {
	schema, err := compiledAppSchema()
	if err != nil {
		return nil, err
	}
	var generic any
	if err := root.Decode(&generic); err != nil {
		return nil, fmt.Errorf("spec: decode yaml: %w", err)
	}
	raw, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("spec: convert to json: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("spec: re-parse json: %w", err)
	}
	verr := schema.Validate(inst)
	if verr == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return nil, fmt.Errorf("spec: schema validation: %w", verr)
	}
	var issues []Issue
	seen := map[string]bool{}
	var walk func(u jsonschema.OutputUnit)
	walk = func(u jsonschema.OutputUnit) {
		if u.Error != nil && len(u.Errors) == 0 {
			path := strings.Trim(strings.ReplaceAll(u.InstanceLocation, "/", "."), ".")
			key := path + "|" + u.Error.String()
			if !seen[key] {
				seen[key] = true
				issues = append(issues, Issue{Path: path, Line: lineFor(root, strings.Split(path, ".")), Message: u.Error.String()})
			}
		}
		for _, c := range u.Errors {
			walk(c)
		}
	}
	walk(*ve.BasicOutput())
	return issues, nil
}

// lineFor resolves a dotted path (numeric segments index sequences) to a
// source line, falling back to the deepest ancestor that exists.
func lineFor(root *yaml.Node, segs []string) int {
	n := root
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	line := n.Line
	for _, s := range segs {
		if s == "" {
			continue
		}
		var next *yaml.Node
		switch n.Kind {
		case yaml.MappingNode:
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == s {
					next = n.Content[i+1]
					line = n.Content[i].Line
					break
				}
			}
		case yaml.SequenceNode:
			var idx int
			if _, err := fmt.Sscanf(s, "%d", &idx); err == nil && idx >= 0 && idx < len(n.Content) {
				next = n.Content[idx]
				line = next.Line
			}
		}
		if next == nil {
			return line
		}
		n = next
	}
	return line
}
