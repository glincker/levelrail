package main

import (
	"bytes"
	"encoding/json"
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

func TestDocsJSONFlagMatchesFormatJSON(t *testing.T) {
	var a, b, errb bytes.Buffer
	if runDocs("levelrail-cli", []string{"--json"}, &a, &errb, nil) != exitOK || runDocs("levelrail-cli", []string{"--format", "json"}, &b, &errb, nil) != exitOK {
		t.Fatal(errb.String())
	}
	var nodes []docsNode
	if err := json.Unmarshal(a.Bytes(), &nodes); err != nil || len(nodes) == 0 || a.String() != b.String() {
		t.Fatalf("--json differs from --format json or is invalid: %v", err)
	}
}

func TestDocsJSONHasFlagsAndManPages(t *testing.T) {
	var out, errb bytes.Buffer
	if runDocs("levelrail-cli", []string{"--json"}, &out, &errb, nil) != exitOK {
		t.Fatal(errb.String())
	}
	var nodes []docsNode
	if err := json.Unmarshal(out.Bytes(), &nodes); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range nodes {
		if len(n.Path) == 2 && n.Path[0] == "apps" && n.Path[1] == "list" {
			for _, f := range n.Flags {
				if f.Name == "limit" && f.Type == "int" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("apps list --limit int missing from docs --json")
	}
	if fl := parseHelpFlags("Flags:\n  --json   print json (default false)\n"); len(fl) != 1 || fl[0].Default != "false" {
		t.Fatalf("hand-written flag layout: %+v", fl)
	}
	dir := t.TempDir()
	if runDocs("levelrail-cli", []string{"--format", "man", "--out-dir", dir}, &out, &errb, nil) != exitOK {
		t.Fatal(errb.String())
	}
	page, err := os.ReadFile(filepath.Join(dir, "levelrail-cli-apps.1")) //nolint:gosec // temp dir
	if err != nil || !bytes.HasPrefix(page, []byte(".TH ")) {
		t.Fatalf("man page missing or malformed: %v", err)
	}
}
