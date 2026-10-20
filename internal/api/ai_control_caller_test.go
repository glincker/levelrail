package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAIControlGet_ReportsWhetherTheCallerIsAnAgent(t *testing.T) {
	rt, _, tokens := gateFixture(t, "operate", []string{"dev"})
	tests := []struct {
		token string
		want  bool
	}{
		{"agent-write", true},
		{"internal", true},
		{"human-root", false},
	}
	for _, tc := range tests {
		t.Run(tc.token, func(t *testing.T) {
			rec := gateDo(rt, http.MethodGet, "/api/v1/settings/ai-control", tokens[tc.token], "")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET = %d: %s", rec.Code, rec.Body.String())
			}
			var res aiControlResource
			if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
				t.Fatal(err)
			}
			if res.CallerIsAgent != tc.want {
				t.Errorf("caller_is_agent = %v, want %v", res.CallerIsAgent, tc.want)
			}
		})
	}
}

func TestAIControlGet_OffRefusesAgentsButNotOrdinaryTokens(t *testing.T) {
	rt, _, tokens := gateFixture(t, "off", []string{"dev"})
	if rec := gateDo(rt, http.MethodGet, "/api/v1/settings/ai-control", tokens["agent-write"], ""); rec.Code != http.StatusForbidden {
		t.Errorf("agent GET in off mode = %d, want 403 (the MCP server reads this as agent and off)", rec.Code)
	}
	rec := gateDo(rt, http.MethodGet, "/api/v1/settings/ai-control", tokens["human-root"], "")
	var res aiControlResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil || res.CallerIsAgent || res.Mode != "off" {
		t.Errorf("ordinary token in off mode = %d %s, want 200 with mode off and caller_is_agent false", rec.Code, rec.Body.String())
	}
}
