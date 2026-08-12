package spec

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed schema/app.schema.json
var schemaFS embed.FS

const schemaID = "https://glinr.com/levelrail/schema/app.json"

var (
	compiledSchema     *jsonschema.Schema
	compiledSchemaOnce sync.Once
	compiledSchemaErr  error
)

// compiledAppSchema compiles the embedded schema once, on first use, and
// caches it. The schema itself never changes at runtime, so there's no
// reason to recompile it per call.
func compiledAppSchema() (*jsonschema.Schema, error) {
	compiledSchemaOnce.Do(func() {
		raw, err := schemaFS.ReadFile("schema/app.schema.json")
		if err != nil {
			compiledSchemaErr = fmt.Errorf("spec: read embedded schema: %w", err)
			return
		}

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			compiledSchemaErr = fmt.Errorf("spec: parse embedded schema: %w", err)
			return
		}

		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(schemaID, doc); err != nil {
			compiledSchemaErr = fmt.Errorf("spec: add schema resource: %w", err)
			return
		}

		compiledSchema, compiledSchemaErr = compiler.Compile(schemaID)
	})
	return compiledSchema, compiledSchemaErr
}

// validateAgainstSchema checks yamlData (raw app.yaml bytes) against the
// embedded JSON Schema. YAML is decoded generically first, then round-
// tripped through encoding/json so the validator sees the same generic
// number/string/bool/null shapes it would from real JSON, rather than
// YAML-specific Go types the schema library was never designed against.
func validateAgainstSchema(yamlData []byte) error {
	schema, err := compiledAppSchema()
	if err != nil {
		return err
	}

	var generic any
	if err := yaml.Unmarshal(yamlData, &generic); err != nil {
		return fmt.Errorf("spec: parse yaml: %w", err)
	}

	asJSON, err := json.Marshal(generic)
	if err != nil {
		return fmt.Errorf("spec: convert to json for schema validation: %w", err)
	}

	jsonCompatible, err := jsonschema.UnmarshalJSON(bytes.NewReader(asJSON))
	if err != nil {
		return fmt.Errorf("spec: re-parse for schema validation: %w", err)
	}

	if err := schema.Validate(jsonCompatible); err != nil {
		return fmt.Errorf("spec: schema validation: %w", err)
	}

	return nil
}
