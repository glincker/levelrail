package pipeline

import "testing"

func TestCatalogTemplatesValidate(t *testing.T) {
	cat := Catalog()
	if len(cat) < 10 {
		t.Fatalf("catalog has %d templates, want at least 10", len(cat))
	}
	seen := map[string]bool{}
	for _, tpl := range cat {
		if seen[tpl.ID] {
			t.Fatalf("duplicate template id %q", tpl.ID)
		}
		seen[tpl.ID] = true
		t.Run(tpl.ID, func(t *testing.T) {
			if _, issues := Validate([]byte(tpl.YAML)); len(issues) > 0 {
				t.Fatalf("template invalid: %v", issues)
			}
		})
	}
	if _, err := CatalogByID("nope"); err == nil {
		t.Fatal("expected error for unknown template")
	}
}
