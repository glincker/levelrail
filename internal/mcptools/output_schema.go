package mcptools

import (
	_ "embed"
	"encoding/json"
	"slices"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
)

// outputFieldsJSON freezes the top-level properties each tool advertises in
// its outputSchema. Regenerate with: UPDATE_OUTPUT_FIELDS=1 go test ./internal/mcptools -run TestOutputFieldsSnapshot
//
//go:embed output_fields.json
var outputFieldsJSON []byte

var (
	outputFieldsOnce sync.Once
	outputFields     map[string][]string
)

// captureFullOutputFields is set by tests to observe each tool's unfiltered
// top-level properties while registering.
var captureFullOutputFields func(tool string, props []string)

func frozenOutputFields(tool string) []string {
	outputFieldsOnce.Do(func() {
		if err := json.Unmarshal(outputFieldsJSON, &outputFields); err != nil {
			outputFields = map[string][]string{}
		}
	})
	return outputFields[tool]
}

// slimOutputSchema builds the advertised outputSchema for a tool. It lists
// only the frozen top-level properties, collapses nested objects, and drops
// required/additionalProperties, so adding an API field never grows tools/list
// and extra fields still validate. Results themselves are unchanged.
func slimOutputSchema[Out any](tool string) *jsonschema.Schema {
	full, err := jsonschema.For[Out](nil)
	if err != nil || full == nil {
		return nil
	}
	root := rootObject(full)
	if root == nil {
		return nil
	}
	if captureFullOutputFields != nil {
		names := make([]string, 0, len(root.Properties))
		for k := range root.Properties {
			names = append(names, k)
		}
		slices.Sort(names)
		captureFullOutputFields(tool, names)
	}
	if allow := frozenOutputFields(tool); allow != nil {
		for k := range root.Properties {
			if !slices.Contains(allow, k) {
				delete(root.Properties, k)
			}
		}
	}
	for k, p := range root.Properties {
		root.Properties[k] = collapseNested(p)
	}
	root.Required = nil
	root.AdditionalProperties = nil
	return full
}

// rootObject returns the object schema that carries the tool's fields: the
// schema itself, or the element schema of a top-level array. Anything else
// (maps, scalars) is left to the SDK default.
func rootObject(s *jsonschema.Schema) *jsonschema.Schema {
	if len(s.Properties) > 0 {
		return s
	}
	if s.Items != nil && len(s.Items.Properties) > 0 {
		return s.Items
	}
	return nil
}

func collapseNested(p *jsonschema.Schema) *jsonschema.Schema {
	if p == nil {
		return p
	}
	if len(p.Properties) > 0 {
		return &jsonschema.Schema{Type: p.Type, Types: p.Types}
	}
	if p.Items != nil && len(p.Items.Properties) > 0 {
		c := *p
		c.Items = &jsonschema.Schema{Type: "object"}
		return &c
	}
	return p
}
