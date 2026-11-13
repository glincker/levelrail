package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Fails when skills/levelrail/references/commands is stale; fix with scripts/gen-skill-references.sh.
func TestSkillReferencesMatchCommandTree(t *testing.T) {
	files := docsFiles(collectDocs())
	if _, ok := files["apps.md"]; !ok {
		t.Fatal("apps.md missing from generated docs")
	}
	dir := filepath.Join("..", "..", "skills", "levelrail", "references", "commands")
	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // fixed test path
		if err != nil {
			t.Fatalf("%s: %v (run scripts/gen-skill-references.sh)", name, err)
		}
		if string(got) != want {
			t.Fatalf("%s is stale (run scripts/gen-skill-references.sh)", name)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(files) {
		t.Fatalf("%d committed reference files, %d generated (run scripts/gen-skill-references.sh)", len(entries), len(files))
	}
}
