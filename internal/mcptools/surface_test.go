package mcptools

import (
	"encoding/json"
	goflag "flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var updateSurface = goflag.Bool("update-surface", false, "rewrite docs/mcp-tool-surface.md")

const (
	surfaceDoc          = "../../docs/mcp-tool-surface.md"
	agentCoreTokenCap   = 4000
	agentCoreMaxTools   = 15
	agentCoreMaxDescLen = 60
)

func toolTokens(t *testing.T, tool *mcp.Tool) int {
	t.Helper()
	schema, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema for %s: %v", tool.Name, err)
	}
	return (len(tool.Name) + len(tool.Description) + len(schema)) / 4
}

type groupStat struct {
	tools, tokens int
}

func renderSurface(t *testing.T) string {
	t.Helper()
	all := listAll(t, Options{Mode: ModeFull})
	core := listAll(t, Options{Mode: ModeFull, Profile: ProfileAgentCore})
	stats := map[string]*groupStat{}
	total := groupStat{}
	for _, tool := range all {
		m, _ := Lookup(tool.Name)
		g := stats[m.Group]
		if g == nil {
			g = &groupStat{}
			stats[m.Group] = g
		}
		n := toolTokens(t, tool)
		g.tools++
		g.tokens += n
		total.tools++
		total.tokens += n
	}
	groups := make([]string, 0, len(stats))
	for g := range stats {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	var b strings.Builder
	b.WriteString("---\ndescription: \"Estimated model context cost of the MCP tool list, by toolset.\"\n---\n\n# MCP tool surface\n\n")
	b.WriteString("Estimated model context cost of the MCP tool list. Tokens are estimated as characters divided by 4 over each tool's name, description and input schema JSON.\n\n")
	b.WriteString("Regenerate with `go test ./internal/mcptools -run TestToolSurfaceDoc -update-surface`.\n\n")
	b.WriteString("## By toolset\n\n| Toolset | Tools | Est. tokens |\n| --- | ---: | ---: |\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "| %s | %d | %d |\n", g, stats[g].tools, stats[g].tokens)
	}
	fmt.Fprintf(&b, "| **total** | **%d** | **%d** |\n\n", total.tools, total.tokens)

	coreTokens := 0
	names := make([]string, 0, len(core))
	for _, tool := range core {
		coreTokens += toolTokens(t, tool)
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "## agent-core profile\n\nSelect with `--tool-profile agent-core` or `%s=agent-core`. It is an allowlist independent of the read-only, standard and full modes; the API token's abilities still apply.\n\n", EnvToolProfile)
	fmt.Fprintf(&b, "%d tools, %d estimated tokens (cap %d).\n\n", len(core), coreTokens, agentCoreTokenCap)
	for _, n := range names {
		fmt.Fprintf(&b, "- `%s`\n", n)
	}
	return b.String()
}

func TestToolSurfaceDoc(t *testing.T) {
	if !*updateSurface {
		t.Skip("pass -update-surface to regenerate docs/mcp-tool-surface.md")
	}
	if err := os.WriteFile(surfaceDoc, []byte(renderSurface(t)), 0o600); err != nil {
		t.Fatalf("write %s: %v", surfaceDoc, err)
	}
}

func TestAgentCoreProfile(t *testing.T) {
	if n := len(AgentCoreTools()); n > agentCoreMaxTools {
		t.Fatalf("agent-core allowlist has %d tools, max %d", n, agentCoreMaxTools)
	}
	tools := listAll(t, Options{Mode: ModeStandard, Profile: ProfileAgentCore})
	got := map[string]bool{}
	tokens := 0
	for _, tool := range tools {
		got[tool.Name] = true
		tokens += toolTokens(t, tool)
		if _, ok := agentCoreTools[tool.Name]; !ok {
			t.Errorf("unexpected tool %q in agent-core", tool.Name)
		}
		if w := len(strings.Fields(tool.Description)); w > agentCoreMaxDescLen {
			t.Errorf("%s description has %d words, max %d", tool.Name, w, agentCoreMaxDescLen)
		}
	}
	registered := map[string]bool{}
	for _, tool := range listAll(t, Options{Mode: ModeFull}) {
		registered[tool.Name] = true
	}
	for _, n := range AgentCoreTools() {
		if registered[n] != got[n] {
			t.Errorf("tool %q registered=%v but in profile=%v", n, registered[n], got[n])
		}
	}
	if tokens > agentCoreTokenCap {
		t.Errorf("agent-core is %d estimated tokens, cap %d", tokens, agentCoreTokenCap)
	}
	for _, n := range []string{"set_app_env", "unset_app_env"} {
		if !registered[n] {
			t.Skipf("%s not registered yet, skipping final-set assertion", n)
		}
	}
	if len(got) != len(agentCoreTools) {
		t.Errorf("agent-core exposes %d tools, want %d", len(got), len(agentCoreTools))
	}
}

func TestAgentCoreReadOnlyMode(t *testing.T) {
	for _, tool := range listAll(t, Options{Mode: ModeReadOnly, Profile: ProfileAgentCore}) {
		if m, _ := Lookup(tool.Name); m.Class != ClassRead {
			t.Errorf("read-only agent-core exposes %s (%s)", tool.Name, m.Class)
		}
	}
}
