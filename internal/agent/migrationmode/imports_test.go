package migrationmode

import (
	"os/exec"
	"strings"
	"testing"
)

func depsOf(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).Output() //nolint:gosec // test-only, package names are constants
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	return strings.Fields(string(out))
}

func TestImportGraphExcludesWritePaths(t *testing.T) {
	const mod = "github.com/GLINCKER/levelrail/"
	forbidden := []string{
		mod + "internal/docker", mod + "internal/dockerguard", mod + "internal/reconcile", mod + "internal/build",
		mod + "internal/agent", mod + "internal/agent/agentpb", mod + "internal/placement", mod + "internal/store",
		"github.com/docker/docker/client", "os/exec",
	}
	cases := []struct {
		pkg   string
		allow map[string]bool
	}{
		{"github.com/GLINCKER/levelrail/internal/agent/migrationmode", nil},
		{"github.com/GLINCKER/levelrail/cmd/levelrail-migrate", map[string]bool{"os/exec": true}},
	}
	for _, tc := range cases {
		deps := depsOf(t, tc.pkg)
		set := map[string]bool{}
		for _, d := range deps {
			set[d] = true
		}
		if !set[tc.pkg] {
			continue
		}
		for _, f := range forbidden {
			if set[f] && !tc.allow[f] {
				t.Errorf("%s must not depend on %s", tc.pkg, f)
			}
		}
	}
}
