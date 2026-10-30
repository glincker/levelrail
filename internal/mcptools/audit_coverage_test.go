package mcptools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unauditedRegistrations lists files that may call mcp.AddTool directly.
// Those tools never reach the API (session-local toolset switches), so
// there is nothing to audit. A new entry needs the same justification.
var unauditedRegistrations = map[string]bool{
	"classes.go":  true, // addTool itself, the audited path
	"toolsets.go": true, // list/enable/disable_toolset, no API call
}

// TestEveryAPIToolRegistersThroughAuditedPath fails when a tool is
// registered around addTool, which would skip withAgentClient and so
// drop the MCP client from the API audit row.
func TestEveryAPIToolRegistersThroughAuditedPath(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || unauditedRegistrations[f] {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // f comes from a package-local glob
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "mcp.AddTool(") {
			t.Errorf("%s registers a tool with mcp.AddTool; use addTool so API calls carry the agent client for audit", f)
		}
	}
}

// TestMutatingToolsAreAuditClassified ties the tool class to the API
// audit rule (every non-read ability is logged): a registered tool must
// have a class, and no non-read tool may be marked read-only.
func TestMutatingToolsAreAuditClassified(t *testing.T) {
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true}) {
		m, ok := Lookup(tool.Name)
		if !ok {
			if tool.Name == toolListToolsets || tool.Name == toolEnableToolset || tool.Name == toolDisableToolset {
				continue
			}
			t.Errorf("tool %q has no class, so its audit coverage is unknown", tool.Name)
			continue
		}
		if m.Class == ClassUnset {
			t.Errorf("tool %q has an unset class", tool.Name)
		}
	}
}
