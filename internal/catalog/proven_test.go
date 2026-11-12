package catalog

import "testing"

func TestProven_IdsExistAndCountsAdd(t *testing.T) {
	for id := range provenIDs {
		if _, ok := TemplateByID(id); !ok {
			t.Errorf("proven id %q is not in the catalog", id)
		}
	}
	proven, preview := ProvenCounts()
	if proven+preview != len(Templates) {
		t.Fatalf("proven %d + preview %d != %d templates", proven, preview, len(Templates))
	}
	if proven == 0 {
		t.Fatal("no proven templates recorded")
	}
	for _, tpl := range Templates {
		if IsPreview(tpl.ID) == IsProven(tpl.ID) {
			t.Errorf("%s must be exactly one of proven or preview", tpl.ID)
		}
	}
	t.Logf("proven %d, preview %d", proven, preview)
}
