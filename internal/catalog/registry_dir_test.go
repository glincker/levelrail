package catalog

import (
	"reflect"
	"testing"
)

// Proves the folder registry holds exactly what the Go catalog serves today.
func TestRegistryDirRoundTrip(t *testing.T) {
	got, err := LoadRegistryDir("../../registry/templates")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(Templates) {
		t.Fatalf("registry has %d templates, catalog has %d", len(got), len(Templates))
	}
	want := map[string]Template{}
	for _, tpl := range Templates {
		want[tpl.ID] = tpl
	}
	for _, g := range got {
		w, ok := want[g.ID]
		if !ok {
			t.Errorf("registry template %s is not in the catalog", g.ID)
			continue
		}
		if !reflect.DeepEqual(w, g) {
			t.Errorf("template %s differs after round trip:\nwant %+v\ngot  %+v", g.ID, w, g)
		}
	}
}
