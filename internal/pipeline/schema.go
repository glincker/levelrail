package pipeline

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed schema/pipeline.schema.json
var schemaJSON []byte

// SchemaJSON returns the JSON Schema pipeline files are validated against.
func SchemaJSON() []byte { return bytes.Clone(schemaJSON) }

const schemaID = "urn:pipeline:schema:1"

var (
	compiled     *jsonschema.Schema
	compiledOnce sync.Once
	compiledErr  error
)

func compiledSchema() (*jsonschema.Schema, error) {
	compiledOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
		if err != nil {
			compiledErr = fmt.Errorf("pipeline: parse embedded schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaID, doc); err != nil {
			compiledErr = fmt.Errorf("pipeline: add schema resource: %w", err)
			return
		}
		compiled, compiledErr = c.Compile(schemaID)
	})
	return compiled, compiledErr
}

// schemaIssues validates the decoded YAML against the JSON Schema and maps
// each violation back to a line in the source.
func schemaIssues(root *yaml.Node) ([]Issue, error) {
	sch, err := compiledSchema()
	if err != nil {
		return nil, err
	}
	var generic any
	if err := root.Decode(&generic); err != nil {
		return nil, fmt.Errorf("pipeline: decode yaml: %w", err)
	}
	raw, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("pipeline: convert to json: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("pipeline: re-parse json: %w", err)
	}
	verr := sch.Validate(inst)
	if verr == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return nil, fmt.Errorf("pipeline: schema validation: %w", verr)
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
