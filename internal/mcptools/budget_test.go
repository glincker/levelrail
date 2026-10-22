package mcptools

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// charsPerToken is the rough 4 chars per token estimate; no tokenizer
// library is in go.mod and the budget only needs a stable relative measure.
const charsPerToken = 4

// modeTokenBudgets are the default ceilings on a mode's tools/list size in
// estimated tokens with every outputSchema advertised (the in-process
// NewServer shape and levelrail-mcp --output-schemas). They cover write tools
// (AllowWrite). Output schemas are about 60 percent of that listing, which is
// why levelrail-mcp omits them by default: see compactTokenBudgets.
// Override one with APP_MCP_TOKEN_BUDGET_<MODE>, where MODE is upper case with
// dashes as underscores (e.g. READ_ONLY).
var modeTokenBudgets = map[Mode]int{
	ModeReadOnly: 41800,
	ModeStandard: 52400,
	ModeFull:     57250,
}

// compactTokenBudgets are the ceilings for the listing levelrail-mcp serves
// by default (OmitOutputSchemas). Override with
// APP_MCP_TOKEN_BUDGET_COMPACT_<MODE>.
var compactTokenBudgets = map[Mode]int{
	ModeReadOnly: 24450,
	ModeStandard: 31150,
	ModeFull:     34000,
}

func budgetEnvKey(mode Mode) string {
	return "APP_MCP_TOKEN_BUDGET_" + strings.ToUpper(strings.ReplaceAll(string(mode), "-", "_"))
}

func compactBudgetEnvKey(mode Mode) string {
	return "APP_MCP_TOKEN_BUDGET_COMPACT_" + strings.ToUpper(strings.ReplaceAll(string(mode), "-", "_"))
}

func estimateTokens(t *testing.T, tools []*mcp.Tool) int {
	t.Helper()
	b, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	return (len(b) + charsPerToken - 1) / charsPerToken
}

func envBudget(t *testing.T, key string, def int) int {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("%s=%q is not a positive integer", key, v)
		}
		return n
	}
	return def
}

func budgetFor(t *testing.T, mode Mode) int {
	t.Helper()
	return envBudget(t, budgetEnvKey(mode), modeTokenBudgets[mode])
}

// perToolReport ranks tools by listing size so a budget failure names the
// heaviest contributors and the heaviest toolsets.
func perToolReport(tools []*mcp.Tool, top int) string {
	type row struct {
		name  string
		total int
	}
	size := func(v any) int {
		b, _ := json.Marshal(v)
		return len(b) / charsPerToken
	}
	rows := make([]row, 0, len(tools))
	groups := map[string]int{}
	for _, tool := range tools {
		n := size(tool)
		rows = append(rows, row{tool.Name, n})
		m, _ := Lookup(tool.Name)
		groups[m.Group] += n
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].total > rows[j].total })
	if len(rows) > top {
		rows = rows[:top]
	}
	var sb strings.Builder
	sb.WriteString("\n  heaviest tools:")
	for _, r := range rows {
		fmt.Fprintf(&sb, "\n    %-32s ~%d", r.name, r.total)
	}
	names := make([]string, 0, len(groups))
	for g := range groups {
		names = append(names, g)
	}
	sort.Slice(names, func(i, j int) bool { return groups[names[i]] > groups[names[j]] })
	sb.WriteString("\n  heaviest toolsets:")
	for i, g := range names {
		if i >= 6 {
			break
		}
		fmt.Fprintf(&sb, "\n    %-32s ~%d", g, groups[g])
	}
	return sb.String()
}

func checkBudget(t *testing.T, label string, tools []*mcp.Tool, budget int, envKey string) {
	t.Helper()
	got := estimateTokens(t, tools)
	t.Logf("%s: %d tools, ~%d tokens, budget %d, headroom %d (%.1f%%)", label, len(tools), got, budget, budget-got, 100*float64(budget-got)/float64(budget))
	if got > budget {
		t.Errorf("%s tools/list is ~%d tokens, over the %d budget by %d: trim descriptions or schemas, or raise %s deliberately%s",
			label, got, budget, got-budget, envKey, perToolReport(tools, 8))
	}
}

func TestToolListTokenBudget(t *testing.T) {
	for mode := range modeTokenBudgets {
		t.Run(string(mode), func(t *testing.T) {
			tools := listAll(t, Options{Mode: mode, AllowWrite: true})
			checkBudget(t, "mode "+string(mode)+" with output schemas", tools, budgetFor(t, mode), budgetEnvKey(mode))
		})
	}
}

func TestCompactToolListTokenBudget(t *testing.T) {
	for mode := range compactTokenBudgets {
		t.Run(string(mode), func(t *testing.T) {
			tools := listAll(t, Options{Mode: mode, AllowWrite: true, OmitOutputSchemas: true})
			budget := envBudget(t, compactBudgetEnvKey(mode), compactTokenBudgets[mode])
			checkBudget(t, "mode "+string(mode)+" compact", tools, budget, compactBudgetEnvKey(mode))
		})
	}
}

// toolsetTokenCeiling caps one toolset's compact listing so enabling it on demand stays cheap.
const toolsetTokenCeiling = 4000

func TestToolsetTokenCeiling(t *testing.T) {
	groups := map[string]int{}
	for _, tool := range listAll(t, Options{Mode: ModeFull, AllowWrite: true, OmitOutputSchemas: true}) {
		m, _ := Lookup(tool.Name)
		b, _ := json.Marshal(tool)
		groups[m.Group] += (len(b) + charsPerToken - 1) / charsPerToken
	}
	for g, n := range groups {
		if n > toolsetTokenCeiling {
			t.Errorf("toolset %q is ~%d tokens, over the %d ceiling: split it or trim its tools", g, n, toolsetTokenCeiling)
		}
	}
}

func TestBudgetFailureNamesHeaviestTools(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeStandard})
	report := perToolReport(tools, 3)
	if !strings.Contains(report, "heaviest tools:") || !strings.Contains(report, "heaviest toolsets:") {
		t.Errorf("report lacks sections: %s", report)
	}
}

// TestToolListReport logs the heaviest tools of the full set; run with -v.
func TestToolListReport(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull, AllowWrite: true})
	t.Logf("%s", perToolReport(tools, 10))
}
