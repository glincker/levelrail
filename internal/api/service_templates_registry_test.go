package api

import (
	"context"
	"io/fs"
	"testing"

	"github.com/glincker/levelrail/registry"
)

func TestResolveTemplate_ServesRegistryOnlyIDs(t *testing.T) {
	rt, _ := newTestRouter(t)
	entries, err := fs.ReadDir(registry.Templates, "templates")
	if err != nil {
		t.Fatal(err)
	}
	var served int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		tpl, ok, err := rt.resolveTemplate(context.Background(), e.Name())
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if !ok || tpl.Compose == "" {
			t.Errorf("%s does not resolve through the API lookup", e.Name())
			continue
		}
		served++
	}
	if served < 590 {
		t.Fatalf("served %d registry ids, want the whole registry", served)
	}
	for _, id := range []string{"dagu", "kener", "novu", "argilla", "automatisch", "zitadel"} {
		tpl, ok, _ := rt.resolveTemplate(context.Background(), id)
		if !ok {
			t.Fatalf("%s not served", id)
		}
		// The one-click deploy handler rejects exactly this case with a 409.
		if tpl.RequiresConfiguration {
			t.Errorf("%s would be refused by one-click deploy", id)
		}
	}
}
