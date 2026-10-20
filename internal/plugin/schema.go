package plugin

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

//go:embed manifest.schema.json
var manifestSchemaJSON []byte

// ManifestSchema returns the JSON Schema manifests are validated against.
func ManifestSchema() []byte { return bytes.Clone(manifestSchemaJSON) }

const manifestSchemaID = "urn:plugin:manifest:1"

var (
	schemaOnce sync.Once
	schemaComp *jsonschema.Schema
	schemaErr  error
)

func compiledManifestSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(manifestSchemaJSON))
		if err != nil {
			schemaErr = fmt.Errorf("plugin: parse manifest schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(manifestSchemaID, doc); err != nil {
			schemaErr = fmt.Errorf("plugin: add manifest schema: %w", err)
			return
		}
		schemaComp, schemaErr = c.Compile(manifestSchemaID)
	})
	return schemaComp, schemaErr
}

func validateSchema(raw []byte) error {
	sch, err := compiledManifestSchema()
	if err != nil {
		return err
	}
	var generic any
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return invalid("parse manifest: %v", err)
	}
	asJSON, err := json.Marshal(generic)
	if err != nil {
		return invalid("manifest is not representable as JSON: %v", err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(asJSON))
	if err != nil {
		return invalid("manifest is not representable as JSON: %v", err)
	}
	verr := sch.Validate(inst)
	if verr == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return invalid("manifest schema: %v", verr)
	}
	return invalid("manifest schema: %s", firstSchemaIssue(*ve.BasicOutput()))
}

func firstSchemaIssue(u jsonschema.OutputUnit) string {
	if u.Error != nil && len(u.Errors) == 0 {
		return fmt.Sprintf("%s: %s", u.InstanceLocation, u.Error.String())
	}
	for _, c := range u.Errors {
		if s := firstSchemaIssue(c); s != "" {
			return s
		}
	}
	return ""
}
