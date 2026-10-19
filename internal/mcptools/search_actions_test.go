package mcptools

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/search"
)

func TestSearchActionsNameRealTools(t *testing.T) {
	for _, a := range search.Actions {
		if a.MCPTool == "" {
			continue
		}
		if _, ok := toolTable[a.MCPTool]; !ok {
			t.Errorf("action %s points at unregistered MCP tool %q", a.ID, a.MCPTool)
		}
	}
}
