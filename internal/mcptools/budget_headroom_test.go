package mcptools

import (
	"os"
	"strconv"
	"testing"
)

func TestCompactBudgetKeepsHeadroom(t *testing.T) {
	minHeadroom := 150
	if v := os.Getenv("APP_MCP_TOKEN_MIN_HEADROOM"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			t.Fatalf("APP_MCP_TOKEN_MIN_HEADROOM=%q is not a non-negative integer", v)
		}
		minHeadroom = n
	}
	for mode := range compactTokenBudgets {
		tools := listAll(t, Options{Mode: mode, AllowWrite: true, OmitOutputSchemas: true})
		budget := envBudget(t, compactBudgetEnvKey(mode), compactTokenBudgets[mode])
		if room := budget - estimateTokens(t, tools); room < minHeadroom {
			t.Errorf("mode %s compact headroom %d is under %d", mode, room, minHeadroom)
		}
	}
}

func TestSlimInputSchemaKeepsParamsAndRequired(t *testing.T) {
	in := map[string]any{
		"type": "object", "additionalProperties": false, "required": []any{"name"},
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "d"},
			"age":  map[string]any{"type": []any{"null", "integer"}},
			"cfg":  map[string]any{"type": "object", "additionalProperties": false},
		},
	}
	out, ok := slimInputSchema(in).(map[string]any)
	if !ok {
		t.Fatal("not a map")
	}
	props, _ := out["properties"].(map[string]any)
	if _, has := out["additionalProperties"]; has || len(props) != 3 || out["required"] == nil {
		t.Errorf("got %v", out)
	}
	if props["age"].(map[string]any)["type"] != "integer" {
		t.Errorf("nullable not collapsed: %v", props["age"])
	}
	if _, has := props["cfg"].(map[string]any)["additionalProperties"]; has {
		t.Errorf("nested additionalProperties kept")
	}
}
