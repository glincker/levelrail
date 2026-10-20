package mcptools

import (
	"regexp"
	"strings"
	"testing"
)

func TestExposureToolsAreReadOnly(t *testing.T) {
	changes := regexp.MustCompile(`(?i)restrict|firewall_(apply|rule|allow|deny|enable|disable)`)
	found := false
	for name, meta := range toolTable {
		if strings.Contains(name, "exposure") {
			found = true
			if meta.Class != ClassRead {
				t.Errorf("tool %q must be read-only, got %s", name, meta.Class)
			}
		}
		if changes.MatchString(name) {
			t.Errorf("tool %q looks like it can change firewall rules, which MCP must never do", name)
		}
	}
	if !found {
		t.Fatal("no exposure tool registered")
	}
}
