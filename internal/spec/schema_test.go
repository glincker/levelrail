package spec

import (
	"testing"
)

func TestCompiledAppSchema(t *testing.T) {
	schema, err := compiledAppSchema()
	if err != nil {
		t.Fatalf("compiledAppSchema() returned unexpected error: %v", err)
	}
	if schema == nil {
		t.Fatal("compiledAppSchema() returned nil schema")
	}
}
