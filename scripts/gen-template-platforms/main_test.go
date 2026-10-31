package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestStatusAndBlock(t *testing.T) {
	ts := []catalog.Template{
		{ID: "b", Platforms: []string{"linux/amd64", "linux/arm64"}},
		{ID: "a"},
		{ID: "c", Platforms: []string{"linux/amd64"}},
	}
	blk := block(ts, "2026-10-31T00:00:00Z")
	for _, want := range []string{"| `a` | not checked | - |", "| `b` | yes |", "| `c` | amd64 only |", "1 yes, 1 amd64 only, 1 not checked"} {
		if !strings.Contains(blk, want) {
			t.Errorf("missing %q in\n%s", want, blk)
		}
	}
}

func TestGenerateInsertsThenReplaces(t *testing.T) {
	doc := "intro\n\n" + anchorHead + "\nrest\n"
	first, err := generate(doc, beginMarker+"\nOLD\n"+endMarker+"\n")
	if err != nil {
		t.Fatal(err)
	}
	second, err := generate(first, beginMarker+"\nNEW\n"+endMarker+"\n")
	if err != nil || !strings.Contains(second, "NEW") || strings.Contains(second, "OLD") || strings.Count(second, anchorHead) != 1 {
		t.Fatalf("replace failed: %v\n%s", err, second)
	}
}
