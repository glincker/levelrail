package iac

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/GLINCKER/levelrail/internal/spec"
)

//go:embed schema/resources.schema.json
var resourcesSchema []byte

var extraPropRe = regexp.MustCompile(`^additional properties '([^']+)'`)

const schemaID = "https://glinr.com/levelrail/schema/resources.json"

var secretRefEnv = map[string]any{
	"type":                 "object",
	"required":             []any{"secretRef"},
	"additionalProperties": false,
	"properties":           map[string]any{"secretRef": map[string]any{"type": "string", "minLength": 1}},
}

var (
	mergedOnce sync.Once
	mergedJSON []byte
	mergedErr  error

	compiledOnce sync.Once
	compiled     *jsonschema.Schema
	compiledErr  error
)

// SchemaJSON returns the published JSON Schema: the resource envelope schema
// with the app.yaml schema definitions merged in unchanged, except that an
// env value may also be a { secretRef } object.
func SchemaJSON() ([]byte, error) {
	mergedOnce.Do(func() { mergedJSON, mergedErr = buildSchema() })
	return bytes.Clone(mergedJSON), mergedErr
}

func buildSchema() ([]byte, error) {
	var own, app map[string]any
	if err := json.Unmarshal(resourcesSchema, &own); err != nil {
		return nil, fmt.Errorf("iac: parse resource schema: %w", err)
	}
	raw, err := spec.SchemaJSON()
	if err != nil {
		return nil, fmt.Errorf("iac: read app schema: %w", err)
	}
	if err := json.Unmarshal(raw, &app); err != nil {
		return nil, fmt.Errorf("iac: parse app schema: %w", err)
	}
	defs, _ := own["$defs"].(map[string]any)
	appDefs, _ := app["$defs"].(map[string]any)
	for name, def := range appDefs {
		if _, clash := defs[name]; clash {
			return nil, fmt.Errorf("iac: schema definition %q clashes with the app schema", name)
		}
		defs[name] = def
	}
	env, _ := defs["envValue"].(map[string]any)
	oneOf, _ := env["oneOf"].([]any)
	env["oneOf"] = append(oneOf, secretRefEnv)
	out, err := json.MarshalIndent(own, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("iac: encode merged schema: %w", err)
	}
	return append(out, '\n'), nil
}

func compiledSchema() (*jsonschema.Schema, error) {
	compiledOnce.Do(func() {
		raw, err := SchemaJSON()
		if err != nil {
			compiledErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			compiledErr = fmt.Errorf("iac: parse schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaID, doc); err != nil {
			compiledErr = fmt.Errorf("iac: add schema: %w", err)
			return
		}
		compiled, compiledErr = c.Compile(schemaID)
	})
	return compiled, compiledErr
}

// schemaIssues validates one document against the schema and maps each
// violation back to a source line.
func schemaIssues(d *Document) ([]Issue, error) {
	sch, err := compiledSchema()
	if err != nil {
		return nil, err
	}
	var generic any
	if err := d.Root.Decode(&generic); err != nil {
		return nil, fmt.Errorf("iac: decode yaml: %w", err)
	}
	raw, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("iac: convert to json: %w", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("iac: re-parse json: %w", err)
	}
	verr := sch.Validate(inst)
	if verr == nil {
		return nil, nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return nil, fmt.Errorf("iac: schema validation: %w", verr)
	}
	var issues []Issue
	seen := map[string]bool{}
	var walk func(u jsonschema.OutputUnit)
	walk = func(u jsonschema.OutputUnit) {
		if u.Error != nil && len(u.Errors) == 0 {
			path := strings.Trim(strings.ReplaceAll(u.InstanceLocation, "/", "."), ".")
			msg := u.Error.String()
			if m := extraPropRe.FindStringSubmatch(msg); m != nil {
				path += "." + m[1]
			}
			if !seen[path+"|"+msg] {
				seen[path+"|"+msg] = true
				issues = append(issues, Issue{File: d.File, Path: path, Line: lineFor(d.Root, strings.Split(path, ".")), Message: msg})
			}
		}
		for _, c := range u.Errors {
			walk(c)
		}
	}
	walk(*ve.DetailedOutput())
	return issues, nil
}
