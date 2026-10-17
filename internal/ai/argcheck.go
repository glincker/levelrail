package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// ValidateToolArgs checks model-supplied tool arguments against a JSON Schema subset before the tool runs.
func ValidateToolArgs(schema, args json.RawMessage) error {
	if len(bytes.TrimSpace(args)) == 0 {
		args = json.RawMessage(`{}`)
	}
	var in map[string]json.RawMessage
	if err := json.Unmarshal(args, &in); err != nil || in == nil {
		return fmt.Errorf("arguments must be a JSON object")
	}
	if len(schema) == 0 {
		return nil
	}
	var sc struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &sc); err != nil {
		return nil
	}
	for _, name := range sc.Required {
		if _, ok := in[name]; !ok {
			return fmt.Errorf("missing required argument %q", name)
		}
	}
	for name, raw := range in {
		propRaw, known := sc.Properties[name]
		if !known {
			continue
		}
		var prop struct {
			Type any               `json:"type"`
			Enum []json.RawMessage `json:"enum"`
		}
		if json.Unmarshal(propRaw, &prop) != nil {
			continue
		}
		if err := checkType(name, prop.Type, raw); err != nil {
			return err
		}
		if len(prop.Enum) > 0 && !slices.ContainsFunc(prop.Enum, func(e json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(e), bytes.TrimSpace(raw)) }) {
			return fmt.Errorf("argument %q is not one of the allowed values", name)
		}
	}
	return nil
}

func checkType(name string, typ any, raw json.RawMessage) error {
	var types []string
	switch t := typ.(type) {
	case string:
		types = []string{t}
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok {
				types = append(types, s)
			}
		}
	}
	if len(types) == 0 {
		return nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("argument %q is not valid JSON", name)
	}
	for _, t := range types {
		if matchesJSONType(t, v) {
			return nil
		}
	}
	return fmt.Errorf("argument %q must be of type %v", name, types)
}

func matchesJSONType(t string, v any) bool {
	switch t {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == float64(int64(f))
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "null":
		return v == nil
	}
	return true
}
