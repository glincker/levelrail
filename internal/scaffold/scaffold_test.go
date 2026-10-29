package scaffold

import (
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

var update = flag.Bool("update", false, "rewrite the golden file")

const starter = "version: 1\nservices:\n  web:\n    build:\n      type: dockerfile\n    port: 3000\n    health:\n      readiness: { path: /, interval: 5s, timeout: 2s }\n"

func TestCommentedGoldenAndValid(t *testing.T) {
	got, err := Commented(starter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := spec.Parse([]byte(got)); err != nil {
		t.Fatalf("commented output no longer validates: %v\n%s", err, got)
	}
	if strings.ContainsAny(got, string([]rune{0x2014, 0x2013})) {
		t.Fatal("output contains a dash character")
	}
	if !strings.Contains(got, "# replicas: 2") || !strings.Contains(got, "# default: 1") {
		t.Fatalf("missing optional replicas block or default note:\n%s", got)
	}
	const golden = "testdata/commented.golden.yaml"
	if *update {
		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Fatalf("golden mismatch (run with -update):\n%s", got)
	}
}
