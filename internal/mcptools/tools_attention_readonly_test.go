package mcptools

import (
	"regexp"
	"testing"
)

var (
	deviceLoginToolName = regexp.MustCompile(`device|login|cli_access`)
	decisionVerb        = regexp.MustCompile(`approve|deny|reject|decide|confirm`)
)

// A person must approve a CLI login in the dashboard: no MCP tool may be
// able to approve or deny one, in any mode.
func TestNoDeviceLoginDecisionTool(t *testing.T) {
	for _, tool := range listAll(t, Options{Mode: ModeFull}) {
		if deviceLoginToolName.MatchString(tool.Name) && decisionVerb.MatchString(tool.Name) {
			t.Errorf("tool %q can decide a device login; approval must stay dashboard-only", tool.Name)
		}
	}
}
