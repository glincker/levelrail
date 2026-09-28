package iac

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestDocsExamplesValidate(t *testing.T) {
	raw, err := os.ReadFile("../../docs/platform-as-code.md")
	if err != nil {
		t.Fatal(err)
	}
	blocks := regexp.MustCompile("(?s)```yaml\n(version: 1\n.*?)```").FindAllStringSubmatch(string(raw), -1)
	if len(blocks) < 8 {
		t.Fatalf("found %d resource examples, expected the kind references and examples", len(blocks))
	}
	for i, b := range blocks {
		docs, issues := ParseDocuments([]Source{{Name: "docs.md", Data: []byte(b[1])}})
		if len(issues) == 0 {
			_, issues = Build(docs, Options{})
		}
		if len(issues) > 0 {
			t.Errorf("example %d does not validate: %v\n%s", i+1, issues, b[1])
		}
	}
}

const publishedSchemaPath = "../../docs/schemas/resources.schema.json"

func TestPublishedSchemaFileIsCurrent(t *testing.T) {
	want, err := SchemaJSON()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("IAC_UPDATE_SCHEMA") == "1" {
		if err := os.MkdirAll(filepath.Dir(publishedSchemaPath), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(publishedSchemaPath, want, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(publishedSchemaPath)
	if err != nil {
		t.Fatalf("read published schema: %v (run with IAC_UPDATE_SCHEMA=1 to write it)", err)
	}
	if string(got) != string(want) {
		t.Fatal("docs/schemas/resources.schema.json is stale; run: IAC_UPDATE_SCHEMA=1 go test -run TestPublishedSchemaFileIsCurrent ./internal/iac/")
	}
}
