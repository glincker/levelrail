package mcptools

import (
	"os"
	"strconv"
	"testing"
)

// agentCoreListingBudget is the default ceiling on the agent-core profile's
// whole tools/list response (output schemas and annotations included), in
// estimated tokens. Override with APP_MCP_TOKEN_BUDGET_AGENT_CORE.
const agentCoreListingBudget = 3500

func TestAgentCoreProfileListingBudget(t *testing.T) {
	budget := agentCoreListingBudget
	if v := os.Getenv("APP_MCP_TOKEN_BUDGET_AGENT_CORE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			t.Fatalf("APP_MCP_TOKEN_BUDGET_AGENT_CORE=%q is not a positive integer", v)
		}
		budget = n
	}
	tools := listAll(t, Options{Mode: ModeFull, Profile: ProfileAgentCore})
	got := estimateTokens(t, tools)
	t.Logf("agent-core profile: %d tools, ~%d tokens in the full tools/list (budget %d)", len(tools), got, budget)
	if got > budget {
		t.Errorf("agent-core tools/list is ~%d tokens, over the %d budget: return compact results or trim schemas", got, budget)
	}
}
