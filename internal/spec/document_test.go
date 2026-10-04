package spec

import (
	"testing"
)

func TestValidateDocument_ValidFull(t *testing.T) {
	s, issues := ValidateDocument(readTestdata(t, "valid_full.yaml"))
	if len(issues) != 0 {
		t.Fatalf("issues = %v, want none", issues)
	}
	if s == nil || len(s.Services) == 0 {
		t.Fatalf("Spec = %+v, want a parsed spec", s)
	}
}

func TestValidateDocument_InvalidYAML(t *testing.T) {
	_, issues := ValidateDocument([]byte("services:\n  web: [this is not a map"))
	if len(issues) != 1 || issues[0].Path != "" {
		t.Fatalf("issues = %+v, want exactly one path-less issue", issues)
	}
}

func TestValidateDocument_SchemaViolations_ReportsEveryOne(t *testing.T) {
	data := []byte(`version: 1
services:
  web:
    build:
      type: bogus-type
    port: "not-a-number"
`)
	s, issues := ValidateDocument(data)
	if s != nil {
		t.Fatalf("Spec = %+v, want nil on schema failure", s)
	}
	if len(issues) < 2 {
		t.Fatalf("issues = %+v, want at least 2 (build.type enum, port type)", issues)
	}
	for _, issue := range issues {
		if issue.Line == 0 {
			t.Errorf("issue %+v: want a resolved source line", issue)
		}
	}
}

func TestValidateDocument_SemanticError_HasServicePath(t *testing.T) {
	data := []byte(`version: 1
services:
  web:
    build:
      type: image
      image: nginx:latest
    port: 80
    resources:
      swapMemory: 1Gi
`)
	s, issues := ValidateDocument(data)
	if s != nil {
		t.Fatalf("Spec = %+v, want nil on semantic failure", s)
	}
	if len(issues) != 1 {
		t.Fatalf("issues = %+v, want exactly one", issues)
	}
	if issues[0].Path != "services.web" {
		t.Errorf("issues[0].Path = %q, want %q", issues[0].Path, "services.web")
	}
}

func TestValidateDocument_SemanticError_DatabasePath(t *testing.T) {
	// Engine is a schema enum (app.schema.json), so only a check the
	// schema can't express (the map-key pattern spec.go's own nameLike
	// comment explains) reaches Validate itself here.
	data := []byte(`version: 1
services:
  web:
    build:
      type: image
      image: nginx:latest
    port: 80
databases:
  Main:
    engine: postgres
`)
	_, issues := ValidateDocument(data)
	if len(issues) != 1 || issues[0].Path != "databases.Main" {
		t.Fatalf("issues = %+v, want exactly one with path %q", issues, "databases.Main")
	}
}

func TestValidateDocument_SemanticError_NoNamedResource(t *testing.T) {
	data := []byte(`version: 1
services:
  a:
    build: { type: image, image: nginx:latest }
    port: 80
    dependsOn: [b]
  b:
    build: { type: image, image: nginx:latest }
    port: 80
    dependsOn: [a]
`)
	_, issues := ValidateDocument(data)
	if len(issues) != 1 || issues[0].Path != "" {
		t.Fatalf("issues = %+v, want exactly one path-less cycle issue", issues)
	}
}
