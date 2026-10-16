package pipeline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixturePipelinesValidate(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join("..", "..", "test", "fixtures", "pipelines", "*.yaml"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixture pipelines found: %v", err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // fixture path from a fixed glob
		if err != nil {
			t.Fatal(err)
		}
		if _, issues := Validate(raw); len(issues) > 0 {
			t.Errorf("%s: %v", f, issues)
		}
	}
}
