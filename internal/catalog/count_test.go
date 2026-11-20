package catalog

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestCountMatchesArmCompatPage(t *testing.T) {
	proven, preview := ProvenCounts()
	if Count() != proven+preview {
		t.Fatalf("Count %d != proven %d + preview %d", Count(), proven, preview)
	}
	page, err := os.ReadFile("../../docs/arm-compatibility.md")
	if err != nil {
		t.Skip("docs not available")
	}
	if !strings.Contains(string(page), "of "+strconv.Itoa(Count())+" templates") {
		t.Fatalf("docs/arm-compatibility.md does not state %d templates; run go run ./scripts/gen-arm-compat", Count())
	}
}
