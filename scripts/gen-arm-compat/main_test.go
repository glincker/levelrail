package main

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func TestSmokeResultsStopsAtFirstTable(t *testing.T) {
	doc := "| Template | Result | Seconds | Note |\n| --- | --- | --- | --- |\n| `a` | pass | 3 |  |\n| `b` | fail | 4 | x |\n\n| Template | Cause | Status |\n| `c` | pass | 1 |\n"
	got := smokeResults(doc)
	if got["a"] != "pass" || got["b"] != "fail" || len(got) != 2 {
		t.Fatalf("unexpected %v", got)
	}
}

func TestPageLabelsUnknownHonestly(t *testing.T) {
	ts := []catalog.Template{
		{ID: "x", Platforms: []string{"linux/amd64", "linux/arm64"}},
		{ID: "y", Platforms: []string{"linux/amd64"}},
		{ID: "z"},
	}
	p := page(ts, "2026-01-01T00:00:00Z", map[string]string{"x": "pass"})
	for _, want := range []string{"| `x` | listed | pass |", "| `y` | amd64 only | not tested |", "| `z` | unknown | not tested |", "1 listed, 1 amd64 only, 1 unknown"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q", want)
		}
	}
}
