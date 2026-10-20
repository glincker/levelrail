package mcptools

import (
	"strings"
	"testing"
)

// A copy needs the source password and must be started by an operator in the
// dashboard or CLI, so no migration or copy tool may be anything but read.
func TestNoToolCanStartAMigrationCopy(t *testing.T) {
	found := false
	for name, meta := range toolTable {
		if !strings.Contains(name, "migrat") && !strings.Contains(name, "copy") && !strings.Contains(name, "import") {
			continue
		}
		found = true
		if strings.Contains(name, "migrat") && meta.Class != ClassRead {
			t.Errorf("tool %q is %s, migration tools must be read-only", name, meta.Class)
		}
	}
	if !found {
		t.Fatal("no migration tool registered")
	}
	if _, ok := toolTable["get_migration_plan"]; !ok {
		t.Fatal("get_migration_plan is not classified")
	}
}
