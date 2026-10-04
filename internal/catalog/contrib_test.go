package catalog

import (
	"testing"
	"testing/fstest"
)

const validContribYAML = `
id: example-contrib
name: Example Contrib
slogan: A test fixture template, not a real catalog entry.
category: Developer Tools
documentation_url: https://example.com/docs
recommended_memory_bytes: 268435456
compose: |
  services:
    example:
      image: example/example:1.0.0
`

func TestLoadContribTemplates_Valid(t *testing.T) {
	fsys := fstest.MapFS{
		"contrib/example.yaml": &fstest.MapFile{Data: []byte(validContribYAML)},
	}

	tpls, err := loadContribTemplates(fsys)
	if err != nil {
		t.Fatalf("loadContribTemplates() error = %v", err)
	}
	if len(tpls) != 1 {
		t.Fatalf("got %d templates, want 1", len(tpls))
	}

	got := tpls[0]
	if got.ID != "example-contrib" {
		t.Errorf("ID = %q, want %q", got.ID, "example-contrib")
	}
	if got.Name != "Example Contrib" {
		t.Errorf("Name = %q, want %q", got.Name, "Example Contrib")
	}
	if got.RecommendedMemoryBytes != 268435456 {
		t.Errorf("RecommendedMemoryBytes = %d, want 268435456", got.RecommendedMemoryBytes)
	}
	if got.Compose == "" {
		t.Error("Compose is empty, want the YAML block scalar body")
	}
}

func TestLoadContribTemplates_MalformedYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"contrib/broken.yaml": &fstest.MapFile{Data: []byte("id: [this is not a valid\n  name: broken")},
	}

	if _, err := loadContribTemplates(fsys); err == nil {
		t.Fatal("loadContribTemplates() error = nil, want an error for malformed YAML")
	}
}

func TestLoadContribTemplates_MissingRequiredField(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"missing id", `
name: Missing ID
slogan: This entry has no id field.
category: Developer Tools
documentation_url: https://example.com
recommended_memory_bytes: 268435456
compose: |
  services:
    foo:
      image: foo:1.0.0
`},
		{"missing compose", `
id: no-compose
name: No Compose
slogan: This entry has no compose field.
category: Developer Tools
documentation_url: https://example.com
recommended_memory_bytes: 268435456
`},
		{"zero memory", `
id: zero-memory
name: Zero Memory
slogan: This entry has no recommended_memory_bytes.
category: Developer Tools
documentation_url: https://example.com
compose: |
  services:
    foo:
      image: foo:1.0.0
`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := fstest.MapFS{
				"contrib/entry.yaml": &fstest.MapFile{Data: []byte(tt.yaml)},
			}
			if _, err := loadContribTemplates(fsys); err == nil {
				t.Fatalf("loadContribTemplates() error = nil, want an error for %s", tt.name)
			}
		})
	}
}

// TestLoadContribTemplates_DuplicateID guards the common contributor
// mistake of copy-pasting one YAML file into another without changing
// id. A duplicate against the Go-source catalog is a separate case,
// caught once everything is merged by TestTemplates_ComposeParsesAndValidates.
func TestLoadContribTemplates_DuplicateID(t *testing.T) {
	fsys := fstest.MapFS{
		"contrib/a.yaml": &fstest.MapFile{Data: []byte(validContribYAML)},
		"contrib/b.yaml": &fstest.MapFile{Data: []byte(validContribYAML)},
	}

	if _, err := loadContribTemplates(fsys); err == nil {
		t.Fatal("loadContribTemplates() error = nil, want an error for a duplicate id within contrib/")
	}
}

// TestContribTemplates_MergedIntoCatalog confirms the real embedded
// contrib/*.yaml templates (including the bytestash.yaml example) decode
// without error and end up in the package-level Templates slice that
// every consumer (API, CLI, dashboard) reads.
func TestContribTemplates_MergedIntoCatalog(t *testing.T) {
	if len(contribTemplates) == 0 {
		t.Fatal("contribTemplates is empty, want at least the bytestash.yaml example")
	}

	for _, want := range contribTemplates {
		found := false
		for _, tpl := range Templates {
			if tpl.ID == want.ID {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("contrib template %q not present in Templates", want.ID)
		}
	}
}
