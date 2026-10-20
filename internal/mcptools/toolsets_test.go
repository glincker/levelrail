package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dynamicListingBudget is the ratchet on the default progressive session's whole
// tools/list in estimated tokens (output schemas omitted). Lower it, never raise it.
const dynamicListingBudget = 3150

func dynamicSession(t *testing.T, opts Options) (*mcp.ClientSession, chan struct{}) {
	t.Helper()
	opts.Dynamic = true
	opts.OmitOutputSchemas = true
	server, _ := NewServerWithOptions(apiclient.NewClient("http://127.0.0.1:1", "t"), opts)
	st, ct := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), st) }()
	changed := make(chan struct{}, 8)
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) { changed <- struct{}{} },
	})
	session, err := client.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, changed
}

func sessionTools(t *testing.T, s *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	var tools []*mcp.Tool
	for tool, err := range s.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func toolNames(tools []*mcp.Tool) map[string]bool {
	out := map[string]bool{}
	for _, tool := range tools {
		out[tool.Name] = true
	}
	return out
}

func waitChanged(t *testing.T, ch chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("no tools/list_changed notification")
	}
}

func TestDynamicDefaultListingBudget(t *testing.T) {
	s, _ := dynamicSession(t, Options{Mode: ModeFull, AllowWrite: true})
	tools := sessionTools(t, s)
	got := estimateTokens(t, tools)
	t.Logf("progressive default: %d tools, ~%d tokens (budget %d)", len(tools), got, dynamicListingBudget)
	if got > dynamicListingBudget {
		t.Errorf("default tools/list is ~%d tokens, over the %d ratchet%s", got, dynamicListingBudget, perToolReport(tools, 8))
	}
	names := toolNames(tools)
	for _, n := range []string{toolListToolsets, toolEnableToolset, toolDisableToolset, "list_apps", "deploy_app", "whoami", "search"} {
		if !names[n] {
			t.Errorf("default session is missing %q", n)
		}
	}
	if names["list_alert_rules"] || names["list_nodes"] {
		t.Error("default session lists tools outside the core")
	}
}

func TestEnableToolsetAddsToolsAndNotifies(t *testing.T) {
	s, changed := dynamicSession(t, Options{Mode: ModeStandard})
	before := toolNames(sessionTools(t, s))
	if before["list_nodes"] {
		t.Fatal("list_nodes should start hidden")
	}
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: toolEnableToolset, Arguments: map[string]any{"toolsets": []string{"nodes"}}})
	if err != nil || res.IsError {
		t.Fatalf("enable_toolset: %v %+v", err, res)
	}
	waitChanged(t, changed)
	after := toolNames(sessionTools(t, s))
	if !after["list_nodes"] {
		t.Error("enable_toolset did not add the nodes tools")
	}

	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: toolDisableToolset, Arguments: map[string]any{"toolsets": []string{"nodes"}}}); err != nil {
		t.Fatalf("disable_toolset: %v", err)
	}
	waitChanged(t, changed)
	if toolNames(sessionTools(t, s))["list_nodes"] {
		t.Error("disable_toolset left the nodes tools listed")
	}
}

func TestDisabledToolsetCallRefused(t *testing.T) {
	s, _ := dynamicSession(t, Options{Mode: ModeStandard})
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "list_nodes", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError || !strings.Contains(textOf(res), "enable_toolset") {
		t.Errorf("want a refusal naming enable_toolset, got %+v", res)
	}
}

func TestDynamicModeStillBoundsTools(t *testing.T) {
	s, _ := dynamicSession(t, Options{Mode: ModeReadOnly})
	if _, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: toolEnableToolset, Arguments: map[string]any{"toolsets": []string{"deploys"}}}); err != nil {
		t.Fatalf("enable_toolset: %v", err)
	}
	for _, tool := range sessionTools(t, s) {
		if isToolsetMetaTool(tool.Name) {
			continue
		}
		if m, _ := Lookup(tool.Name); m.Class != ClassRead {
			t.Errorf("read-only progressive session lists %s (%s)", tool.Name, m.Class)
		}
	}
	res, _ := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "cancel_deploy", Arguments: map[string]any{}})
	if res != nil && !res.IsError {
		t.Error("cancel_deploy ran in a read-only progressive session")
	}
}

func TestListToolsetsReportsCost(t *testing.T) {
	s, _ := dynamicSession(t, Options{Mode: ModeStandard})
	res, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: toolListToolsets, Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_toolsets: %v", err)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out listToolsetsOutput
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Toolsets) == 0 {
		t.Fatal("no toolsets listed")
	}
	for _, ts := range out.Toolsets {
		if ts.Tools == 0 || ts.EstTokens == 0 || ts.Description == "" {
			t.Errorf("toolset %+v is missing tools, tokens or description", ts)
		}
	}
}

func TestEveryToolsetHasDescription(t *testing.T) {
	for _, g := range Groups() {
		if toolsetDescriptions[g] == "" {
			t.Errorf("toolset %q has no entry in toolsetDescriptions", g)
		}
	}
}

func TestDynamicCoreToolsAreRegistered(t *testing.T) {
	registered := toolNames(listAll(t, Options{Mode: ModeFull, AllowWrite: true}))
	for _, n := range DynamicCoreTools() {
		if !registered[n] {
			t.Errorf("core tool %q is not registered", n)
		}
	}
}

func textOf(res *mcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
