package catalog

import (
	"reflect"
	"testing"
)

// Proves the folder registry holds every catalog template unchanged. Extra
// registry-only templates are allowed while the registry becomes the source.
func TestRegistryDirRoundTrip(t *testing.T) {
	got, err := LoadRegistryDir("../../registry/templates")
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]Template{}
	for _, g := range got {
		have[g.ID] = g
	}
	for _, w := range Templates {
		g, ok := have[w.ID]
		if !ok {
			t.Errorf("catalog template %s is missing from the registry", w.ID)
			continue
		}
		if !reflect.DeepEqual(w, g) {
			t.Errorf("template %s differs after round trip:\nwant %+v\ngot  %+v", w.ID, w, g)
		}
	}
}
