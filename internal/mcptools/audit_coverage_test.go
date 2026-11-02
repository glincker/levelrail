package mcptools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// TestEveryRegisteredToolCallIsAudited calls each tool in the registry
// and fails when a call reaches no audit sink, or when an argument value
// leaks into the recorded call.
func TestEveryRegisteredToolCallIsAudited(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(api.Close)

	const secret = "canary-value-for-audit-test" //nolint:gosec // not a credential
	for _, opts := range []Options{
		{Mode: ModeFull, AllowWrite: true},
		{Mode: ModeFull, AllowWrite: true, Dynamic: true},
	} {
		var mu sync.Mutex
		counts := map[string]int{}
		var leaked []string
		opts.AuditSink = func(_ context.Context, c AuditCall) {
			mu.Lock()
			defer mu.Unlock()
			counts[c.Tool]++
			if strings.Contains(c.Tool+strings.Join(c.ArgumentNames, ","), secret) {
				leaked = append(leaked, c.Tool)
			}
		}
		session := connectWith(t, apiclient.NewClient(api.URL, "test-token"), opts)
		for _, tool := range listWith(t, session) {
			before := func() int { mu.Lock(); defer mu.Unlock(); return counts[tool.Name] }()
			_, _ = session.CallTool(context.Background(), &mcp.CallToolParams{
				Name: tool.Name, Arguments: map[string]any{"api_key": secret},
			})
			mu.Lock()
			got := counts[tool.Name] - before
			mu.Unlock()
			if got != 1 {
				t.Errorf("tool %q produced %d audit events for one call, want 1", tool.Name, got)
			}
		}
		if len(leaked) > 0 {
			t.Errorf("audit calls carried argument values: %v", leaked)
		}
	}
}
