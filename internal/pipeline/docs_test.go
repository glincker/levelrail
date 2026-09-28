package pipeline

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestDocExamplesValidate keeps docs/pipelines.md honest: every YAML mapping
// in it, completed with a version and a job where it is only a fragment,
// must pass the same validation a saved pipeline goes through.
func TestDocExamplesValidate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "pipelines.md"))
	if err != nil {
		t.Fatalf("read docs: %v", err)
	}
	blocks := regexp.MustCompile("(?s)```yaml\n(.*?)```").FindAllStringSubmatch(string(raw), -1)
	if len(blocks) < 6 {
		t.Fatalf("found %d yaml blocks, expected the guide's examples", len(blocks))
	}
	checked := 0
	for i, m := range blocks {
		src := m[1]
		if strings.HasPrefix(strings.TrimSpace(src), "-") {
			continue
		}
		if !strings.Contains(src, "version:") {
			src = "version: 1\n" + src
		}
		if !strings.Contains(src, "jobs:") {
			src += "\njobs:\n  x:\n    steps:\n      - uses: notify\n        with: { message: m }\n"
		}
		if _, issues := Validate([]byte(src)); len(issues) > 0 {
			t.Errorf("docs block %d is invalid: %v\n%s", i, issues, src)
		}
		checked++
	}
	if checked < 6 {
		t.Errorf("only %d blocks were validated", checked)
	}
}
