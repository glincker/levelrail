package iac

import "testing"

func schemaCodeIssues(t *testing.T, doc string) []Issue {
	t.Helper()
	docs, issues := ParseDocuments([]Source{{Name: "a.yaml", Data: []byte(doc)}})
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	_, issues = Build(docs, Options{})
	return issues
}

func TestSchemaCodes(t *testing.T) {
	unknown := schemaCodeIssues(t, "version: 1\nkind: Project\nmetadata:\n  name: p\nspec:\n  bogus: 1\n")
	if len(unknown) != 1 || unknown[0].Code != CodeSchemaUnknownField || unknown[0].Path != "spec.bogus" || unknown[0].Line != 6 {
		t.Fatalf("unknown field issue wrong: %+v", unknown)
	}
	ver := schemaCodeIssues(t, "version: 2\nkind: Project\nmetadata:\n  name: p\n")
	if len(ver) != 1 || ver[0].Code != CodeSchemaVersionUnsupported || ver[0].Path != "version" {
		t.Fatalf("version issue wrong: %+v", ver)
	}
	for _, i := range append(unknown, ver...) {
		if i.Message == "" {
			t.Fatal("empty message")
		}
	}
}
