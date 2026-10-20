package spec

import "bytes"

// SchemaJSON returns the embedded app.yaml JSON Schema, for tooling that
// composes it into larger documents.
func SchemaJSON() ([]byte, error) {
	raw, err := schemaFS.ReadFile("schema/app.schema.json")
	if err != nil {
		return nil, err
	}
	return bytes.Clone(raw), nil
}
