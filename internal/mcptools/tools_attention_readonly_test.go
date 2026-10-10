package mcptools

import (
	"regexp"
	"testing"
)

var (
	deviceLoginToolName = regexp.MustCompile(`device|login|cli_access`)
	decisionVerb        = regexp.MustCompile(`approve|deny|reject|decide|confirm|dismiss`)
	attentionToolName   = regexp.MustCompile(`attention|dismiss`)
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

// Attention tools only read: dismissing or deciding an item is a signed-in
// dashboard action, so anything attention-shaped must be a get_ or list_ tool.
func TestAttentionToolsAreReadOnly(t *testing.T) {
	readOnly := regexp.MustCompile(`^(get|list)_`)
	for _, tool := range listAll(t, Options{Mode: ModeFull}) {
		if attentionToolName.MatchString(tool.Name) && !readOnly.MatchString(tool.Name) {
			t.Errorf("tool %q is attention-related but not a get_ or list_ tool", tool.Name)
		}
	}
}
