package mcptools

import (
	"strings"
	"testing"
)

func TestExternalDatabaseToolsAreReadOnly(t *testing.T) {
	found := false
	for name, meta := range toolTable {
		if !strings.Contains(name, "external_database") && !strings.Contains(name, "adopt") {
			continue
		}
		found = true
		if meta.Class != clsR {
			t.Errorf("tool %q is class %s, external database tools must be read only", name, meta.Class)
		}
		for _, verb := range []string{"create", "connect", "adopt", "update", "delete", "set_", "probe", "reveal"} {
			if strings.Contains(name, verb) {
				t.Errorf("tool %q looks like it changes an external database", name)
			}
		}
	}
	if !found {
		t.Fatal("list_external_databases is not registered")
	}
}
