package dockerssh

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestPackageHasNoCommandExecution keeps every host process or remote command
// surface out of this package: SSH here is a socket transport only.
func TestPackageHasNoCommandExecution(t *testing.T) {
	banned := regexp.MustCompile(`\b(NewSession|Exec|Shell|StartProcess)\b|"os/exec"|\.Start\(|\.Run\(|\.Output\(|RequestPty`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(f) //nolint:gosec // package-local source files
		if err != nil {
			t.Fatal(err)
		}
		if loc := banned.FindIndex(raw); loc != nil {
			t.Errorf("%s references %q, command execution is not allowed in this package", f, raw[loc[0]:loc[1]])
		}
	}
}
