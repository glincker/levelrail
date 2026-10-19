package functions

import (
	"strings"
	"testing"
)

func TestRuntimes(t *testing.T) {
	for _, id := range []string{"node", "python", "go"} {
		r, ok := Lookup(id)
		if !ok || len(r.Files) < 2 {
			t.Fatalf("runtime %q missing or incomplete", id)
		}
		for _, f := range r.Files {
			if f.Path == "" || strings.HasPrefix(f.Path, "/") || strings.Contains(f.Path, "..") {
				t.Errorf("%s: unsafe path %q", id, f.Path)
			}
			if strings.ContainsRune(f.Content, rune(0x2014)) || strings.ContainsRune(f.Content, rune(0x2013)) {
				t.Errorf("%s/%s contains a dash character", id, f.Path)
			}
		}
	}
	if _, ok := Lookup("ruby"); ok {
		t.Fatal("unexpected runtime")
	}
	if got := Runtimes(); len(got) != 3 || got[0].ID != "go" {
		t.Fatalf("Runtimes() = %v", got)
	}
}
