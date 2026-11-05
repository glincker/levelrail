package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func healthyAgentPosture() agentPostureInput {
	return agentPostureInput{
		AIKnown: true, AIMode: store.AIControlModeOperate, AIEnvKinds: []string{store.EnvironmentKindDev},
		ProdKnown: true, GuardKnown: true, SinksKnown: true, AuditSinks: 1,
	}
}

func TestAgentPostureRules(t *testing.T) {
	prodAgents := func(in *postureInput) {
		in.Agent.ProdApps = 2
		in.Agent.AIEnvKinds = []string{store.EnvironmentKindProduction}
		in.Tokens = append(in.Tokens, store.APIToken{ID: "a1", Name: "bot", AgentName: "claude", Abilities: []string{AbilityDeploy}})
	}
	cases := []struct {
		id     string
		want   string
		mutate func(*postureInput)
	}{
		{"agent_token_unnamed", postureStatusFail, func(in *postureInput) {
			in.Tokens = append(in.Tokens, store.APIToken{ID: "w", Name: "writer", Abilities: []string{AbilityWrite}})
		}},
		{"agent_token_unnamed", postureStatusPass, func(in *postureInput) {
			in.Tokens = append(in.Tokens, store.APIToken{ID: "w", Name: "bot", AgentName: "x", Abilities: []string{AbilityWrite}})
		}},
		{"ai_control_admin", postureStatusFail, func(in *postureInput) { in.Agent.AIMode = store.AIControlModeAdmin }},
		{"ai_control_admin", postureStatusUnknown, func(in *postureInput) { in.Agent.AIKnown = false }},
		{"agent_tokens_production", postureStatusFail, prodAgents},
		{"agent_tokens_production", postureStatusPass, func(in *postureInput) {
			prodAgents(in)
			in.Agent.AIMode = store.AIControlModeObserve
		}},
		{"prod_env_unprotected", postureStatusFail, func(in *postureInput) { in.Agent.ProdEnvOpen = []string{"production"} }},
		{"prod_env_unprotected", postureStatusUnknown, func(in *postureInput) { in.Agent.ProdKnown = false }},
		{"prod_apps_unguarded", postureStatusFail, func(in *postureInput) { in.Agent.ProdAppsNoGuard = []string{"web"} }},
		{"prod_apps_unguarded", postureStatusUnknown, func(in *postureInput) { in.Agent.GuardKnown = false }},
		{"audit_sink_none", postureStatusFail, func(in *postureInput) { in.Agent.AuditSinks = 0 }},
		{"audit_sink_none", postureStatusUnknown, func(in *postureInput) { in.Agent.SinksKnown = false }},
	}
	for _, c := range cases {
		in := healthyPosture()
		c.mutate(&in)
		got := postureByID(evaluatePosture(in))[c.id]
		if got.Status != c.want {
			t.Errorf("%s: status %q, want %q", c.id, got.Status, c.want)
		}
	}
}

func TestAgentPostureRecommendedUntilLive(t *testing.T) {
	in := healthyPosture()
	in.Agent.AuditSinks = 0
	if it := postureByID(evaluatePosture(in))["audit_sink_none"]; !it.Recommended {
		t.Fatal("fresh server: want recommended")
	}
	in.AppsLive = true
	if it := postureByID(evaluatePosture(in))["audit_sink_none"]; it.Recommended {
		t.Fatal("live server: want scoring")
	}
}
