package platformimport

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestDokployPreviewNotes(t *testing.T) {
	raw, err := os.ReadFile("testdata/dokploy/app_previews.json")
	if err != nil {
		t.Fatal(err)
	}
	var a dokployApp
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	app, unsup := mapDokployApp("p", a)
	if unsup != nil {
		t.Fatalf("unsupported: %+v", unsup)
	}
	var joined string
	for _, n := range app.Notes {
		joined += n.Reason + "\n"
	}
	for _, want := range []string{"limit 4", "*.pv.example.com", "auto deploy", "clean cache"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes missing %q: %s", want, joined)
		}
	}
}
