package iac

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func validateYAML(t *testing.T, name string, data []byte) {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: name, Data: data}})
	if len(issues) == 0 {
		_, issues = Build(docs, Options{})
	}
	if len(issues) > 0 {
		t.Errorf("%s does not validate: %v\n%s", name, issues, data)
	}
}

// TestIaCExamples validates every resource block in docs/iac/*.md and every
// file under examples/iac with the real parser.
func TestIaCExamples(t *testing.T) {
	re := regexp.MustCompile("(?s)```yaml\n(version: 1\n.*?)```")
	pages, _ := filepath.Glob("../../docs/iac/*.md")
	blocks := 0
	for _, page := range pages {
		raw, err := os.ReadFile(page) //nolint:gosec // globbed docs path
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range re.FindAllStringSubmatch(string(raw), -1) {
			blocks++
			validateYAML(t, fmt.Sprintf("%s#%d", filepath.Base(page), i+1), []byte(b[1]))
		}
	}
	if blocks == 0 {
		t.Fatal("no yaml blocks found in docs/iac")
	}
	files := 0
	err := filepath.WalkDir("../../examples/iac", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		raw, err := os.ReadFile(path) //nolint:gosec // walked examples path
		if err != nil {
			return err
		}
		files++
		validateYAML(t, path, raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 2 {
		t.Fatalf("found %d example files, expected at least 2", files)
	}
}

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
