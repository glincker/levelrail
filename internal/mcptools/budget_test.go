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
// estimated tokens. Raised once for the access features (roles, global
// environments, policy templates, AI control add six tools, about 1.9k tokens)
// and again for the migration, external database, exposure, upgrade history and
// attention read-only tools (about 1.2k tokens), and for the database access,
// IAM simulation and app import read-only tools (seven tools, about 2k tokens).
// Trimming tool descriptions is the next lever before raising these again.
// Override one with APP_MCP_TOKEN_BUDGET_<MODE>, where MODE is upper case with
// dashes as underscores (e.g. READ_ONLY).
var modeTokenBudgets = map[Mode]int{
	ModeReadOnly: 52500,
	ModeStandard: 68000,
	ModeFull:     73500,
}

func budgetEnvKey(mode Mode) string {
	return "APP_MCP_TOKEN_BUDGET_" + strings.ToUpper(strings.ReplaceAll(string(mode), "-", "_"))
}

func estimateTokens(t *testing.T, tools []*mcp.Tool) int {
	t.Helper()
	b, err := json.Marshal(tools)
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}
	return (len(b) + charsPerToken - 1) / charsPerToken
}

func budgetFor(t *testing.T, mode Mode) int {
	t.Helper()
	key := budgetEnvKey(mode)
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("%s=%q is not a positive integer", key, v)
		}
		return n
	}
	return modeTokenBudgets[mode]
}

func TestToolListTokenBudget(t *testing.T) {
	for mode := range modeTokenBudgets {
		t.Run(string(mode), func(t *testing.T) {
			tools := listAll(t, Options{Mode: mode})
			got, budget := estimateTokens(t, tools), budgetFor(t, mode)
			t.Logf("mode %s: %d tools, ~%d tokens (budget %d)", mode, len(tools), got, budget)
			if got > budget {
				t.Errorf("mode %s tools/list is ~%d tokens, over the %d budget: trim descriptions or output schemas, or raise %s deliberately",
					mode, got, budget, budgetEnvKey(mode))
			}
		})
	}
}

// TestToolListReport logs the heaviest tools of the full set; run with -v.
func TestToolListReport(t *testing.T) {
	tools := listAll(t, Options{Mode: ModeFull})
	type row struct {
		name         string
		total, in, o int
	}
	size := func(v any) int {
		b, _ := json.Marshal(v)
		return len(b) / charsPerToken
	}
	rows := make([]row, 0, len(tools))
	for _, tool := range tools {
		rows = append(rows, row{tool.Name, size(tool), size(tool.InputSchema), size(tool.OutputSchema)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].total > rows[j].total })
	var sb strings.Builder
	for _, r := range rows[:10] {
		fmt.Fprintf(&sb, "\n  %-32s total ~%d  input ~%d  output ~%d", r.name, r.total, r.in, r.o)
	}
	t.Logf("heaviest tools:%s", sb.String())
}
