package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

func kindFn(kind string) func(string, string) (string, error) {
	return func(string, string) (string, error) { return kind, nil }
}

func TestEvaluateAgentAccess_Matrix(t *testing.T) {
	kinds := []string{"dev", "test"}
	abilities := []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy, AbilityRoot}
	expected := func(mode, ability, envKind string) bool {
		inEnv := envKind == "dev"
		switch mode {
		case "observe":
			return ability == AbilityRead
		case "operate":
			return ability != AbilityRoot && ability != AbilityWriteSensitive && inEnv
		case "admin":
			return inEnv
		}
		return false
	}
	for _, mode := range []string{"off", "observe", "operate", "admin"} {
		for _, ability := range abilities {
			for _, envKind := range []string{"dev", "production"} {
				req := agentRequest{Ability: ability, Method: http.MethodPost, Path: "/api/v1/apps/web/deploy", ResourceKind: "app", ResourceName: "web"}
				got, _, err := evaluateAgentAccess(store.AIControlSettings{Mode: mode, AllowedEnvKinds: kinds}, true, req, kindFn(envKind))
				if err != nil {
					t.Fatal(err)
				}
				if want := expected(mode, ability, envKind); got != want {
					t.Errorf("mode=%s ability=%s env=%s got %v want %v", mode, ability, envKind, got, want)
				}
			}
		}
	}
}

func TestEvaluateAgentAccess_Rules(t *testing.T) {
	s := func(mode string) store.AIControlSettings {
		return store.AIControlSettings{Mode: mode, AllowedEnvKinds: []string{"dev"}}
	}
	tests := []struct {
		name      string
		mode      string
		adminFlag bool
		req       agentRequest
		want      bool
		reason    string
	}{
		{"off blocks reads", "off", true, agentRequest{Ability: AbilityRead, Method: "GET", Path: "/api/v1/apps"}, false, msgAgentDisabled},
		{"observe allows list", "observe", true, agentRequest{Ability: AbilityRead, Method: "GET", Path: "/api/v1/apps"}, true, ""},
		{"observe blocks sensitive reads", "observe", true, agentRequest{Ability: AbilityReadSensitive, Method: "GET", Path: "/api/v1/apps/web/env"}, false, msgAgentModeLimit},
		{"operate allows sensitive reads", "operate", true, agentRequest{Ability: AbilityReadSensitive, Method: "GET", Path: "/api/v1/settings/email"}, true, ""},
		{"operate blocks sensitive writes", "operate", true, agentRequest{Ability: AbilityWriteSensitive, Method: "PUT", Path: "/api/v1/settings/backup"}, false, msgAgentModeLimit},
		{"admin allows sensitive writes", "admin", true, agentRequest{Ability: AbilityWriteSensitive, Method: "PUT", Path: "/api/v1/settings/backup"}, true, ""},
		{"observe blocks write", "observe", true, agentRequest{Ability: AbilityWrite, Method: "POST", Path: "/api/v1/apps"}, false, msgAgentModeLimit},
		{"operate blocks root", "operate", true, agentRequest{Ability: AbilityRoot, Method: "PUT", Path: "/api/v1/settings/email"}, false, msgAgentModeLimit},
		{"operate non resource write ok", "operate", true, agentRequest{Ability: AbilityWrite, Method: "POST", Path: "/api/v1/apps"}, true, ""},
		{"admin allows root off resource", "admin", true, agentRequest{Ability: AbilityRoot, Method: "PUT", Path: "/api/v1/settings/email"}, true, ""},
		{"admin without flag acts as operate", "admin", false, agentRequest{Ability: AbilityRoot, Method: "PUT", Path: "/api/v1/settings/email"}, false, msgAgentModeLimit},
		{"agent cannot approve deploy", "admin", true, agentRequest{Ability: AbilityDeploy, Method: "POST", Path: "/api/v1/deploy-approvals/da_1/approve"}, false, msgAgentNoApprove},
		{"agent cannot approve in operate", "operate", true, agentRequest{Ability: AbilityDeploy, Method: "POST", Path: "/api/v1/deploy-approvals/da_1/approve"}, false, msgAgentNoApprove},
		{"agent cannot decide pipeline approval", "admin", true, agentRequest{Ability: AbilityDeploy, Method: "POST", Path: "/api/v1/apps/web/pipeline-runs/r1/approvals/a1"}, false, msgAgentNoApprove},
		{"agent can reject deploy", "operate", true, agentRequest{Ability: AbilityDeploy, Method: "POST", Path: "/api/v1/deploy-approvals/da_1/reject"}, true, ""},
		{"agent cannot change ai control", "admin", true, agentRequest{Ability: AbilityRoot, Method: "PUT", Path: "/api/v1/settings/ai-control"}, false, msgAgentNoSelfAdmin},
		{"agent cannot revoke tokens via ai control", "admin", true, agentRequest{Ability: AbilityRoot, Method: "POST", Path: "/api/v1/settings/ai-control/revoke-agent-tokens"}, false, msgAgentNoSelfAdmin},
		{"agent can read ai control", "operate", true, agentRequest{Ability: AbilityRead, Method: "GET", Path: "/api/v1/settings/ai-control"}, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, reason, err := evaluateAgentAccess(s(tc.mode), tc.adminFlag, tc.req, kindFn("dev"))
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || reason != tc.reason {
				t.Fatalf("got (%v, %q), want (%v, %q)", got, reason, tc.want, tc.reason)
			}
		})
	}
}

func gateFixture(t *testing.T, mode string, kinds []string) (*Router, *store.DB, map[string]string) {
	t.Helper()
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.UpdateAIControlSettings(ctx, mode, kinds, "test"); err != nil {
		t.Fatal(err)
	}
	for name, env := range map[string]string{"devapp": "env_dev", "prodapp": "env_production", "bareapp": ""} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:v1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
		if env != "" {
			if err := db.SetServiceEnvironment(ctx, name, env); err != nil {
				t.Fatal(err)
			}
		}
	}
	tokens := map[string]string{}
	mint := func(label, agent string, abilities []string) {
		plain, _, err := MintAgentAPIToken(ctx, db, label, abilities, nil, agentIdentity{Name: agent}, "")
		if err != nil {
			t.Fatal(err)
		}
		tokens[label] = plain
	}
	mint("agent-write", "claude", []string{AbilityRead, AbilityWrite, AbilityDeploy})
	mint("agent-root", "claude-root", []string{AbilityRoot})
	mint("human-root", "", []string{AbilityRoot})
	tok, err := MintAIAssistantToken(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	tokens["internal"] = tok
	return rt, db, tokens
}

func gateDo(rt *Router, method, target, token, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(method, target, body, token))
	return rec
}

func TestAIControlGate_Integration(t *testing.T) {
	experimental.Set(experimental.AIControl)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	tests := []struct {
		name    string
		mode    string
		token   string
		method  string
		target  string
		want    int
		wantMsg string
	}{
		{"off blocks agent read", "off", "agent-write", "GET", "/api/v1/apps", 403, msgAgentDisabled},
		{"off blocks internal token", "off", "internal", "GET", "/api/v1/apps", 403, msgAgentDisabled},
		{"off never affects human token", "off", "human-root", "GET", "/api/v1/apps", 200, ""},
		{"observe allows agent read", "observe", "agent-write", "GET", "/api/v1/apps", 200, ""},
		{"observe blocks agent write", "observe", "agent-write", "POST", "/api/v1/apps/devapp/restart", 403, msgAgentModeLimit},
		{"observe never affects human write", "observe", "human-root", "GET", "/api/v1/apps/devapp", 200, ""},
		{"operate allows dev app", "operate", "agent-write", "GET", "/api/v1/apps/devapp", 200, ""},
		{"operate blocks prod app", "operate", "agent-write", "GET", "/api/v1/apps/prodapp", 403, msgAgentEnvLimit},
		{"operate untagged app is custom, blocked", "operate", "agent-write", "GET", "/api/v1/apps/bareapp", 403, msgAgentEnvLimit},
		{"operate blocks root route", "operate", "agent-root", "GET", "/api/v1/system/backups", 403, msgAgentModeLimit},
		{"admin passes gate on root route (handler 501)", "admin", "agent-root", "GET", "/api/v1/system/backups", 501, ""},
		{"admin still bound by env kinds", "admin", "agent-root", "GET", "/api/v1/apps/prodapp", 403, msgAgentEnvLimit},
		{"admin agent cannot change ai control", "admin", "agent-root", "PUT", "/api/v1/settings/ai-control", 403, msgAgentNoSelfAdmin},
		{"admin agent cannot approve", "admin", "agent-root", "POST", "/api/v1/deploy-approvals/da_x/approve", 403, msgAgentNoApprove},
		{"human token approves path reaches handler", "admin", "human-root", "POST", "/api/v1/deploy-approvals/da_x/approve", 404, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, _, tokens := gateFixture(t, tc.mode, []string{"dev", "test"})
			rec := gateDo(rt, tc.method, tc.target, tokens[tc.token], "")
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tc.want, rec.Body.String())
			}
			if tc.wantMsg != "" && !strings.Contains(rec.Body.String(), tc.wantMsg) {
				t.Fatalf("body = %s, want %q", rec.Body.String(), tc.wantMsg)
			}
		})
	}
}

func TestAIControlGate_RejectionsAreAudited(t *testing.T) {
	rt, db, tokens := gateFixture(t, "off", []string{"dev"})
	if rec := gateDo(rt, "GET", "/api/v1/apps", tokens["agent-write"], ""); rec.Code != 403 {
		t.Fatalf("status = %d", rec.Code)
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{AgentName: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].StatusCode != 403 || entries[0].AgentName != "claude" {
		t.Fatalf("audit entries = %+v", entries)
	}
}

func TestAIControlGate_CacheInvalidatedOnPut(t *testing.T) {
	experimental.Set(experimental.AIControl)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, _, tokens := gateFixture(t, "off", []string{"dev"})
	if rec := gateDo(rt, "GET", "/api/v1/apps", tokens["agent-write"], ""); rec.Code != 403 {
		t.Fatalf("status = %d", rec.Code)
	}
	rec := gateDo(rt, "PUT", "/api/v1/settings/ai-control", tokens["human-root"], `{"mode":"observe","allowed_env_kinds":["dev"]}`)
	if rec.Code != 200 {
		t.Fatalf("put = %d %s", rec.Code, rec.Body.String())
	}
	if rec := gateDo(rt, "GET", "/api/v1/apps", tokens["agent-write"], ""); rec.Code != 200 {
		t.Fatalf("after put status = %d, want 200 (stale cache)", rec.Code)
	}
}

func TestAIControlSettingsRoutes(t *testing.T) {
	t.Run("admin refused without flag", func(t *testing.T) {
		experimental.Set()
		t.Cleanup(func() { experimental.Set(experimental.All()...) })
		rt, _, tokens := gateFixture(t, "off", nil)
		rec := gateDo(rt, "PUT", "/api/v1/settings/ai-control", tokens["human-root"], `{"mode":"admin","allowed_env_kinds":["dev"]}`)
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "ai-control") {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
	})
	t.Run("admin accepted with flag and recorded", func(t *testing.T) {
		experimental.Set(experimental.AIControl)
		t.Cleanup(func() { experimental.Set(experimental.All()...) })
		rt, db, tokens := gateFixture(t, "off", nil)
		rec := gateDo(rt, "PUT", "/api/v1/settings/ai-control", tokens["human-root"], `{"mode":"admin","allowed_env_kinds":["test","dev","dev"]}`)
		if rec.Code != 200 {
			t.Fatalf("got %d %s", rec.Code, rec.Body.String())
		}
		var res aiControlResource
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		if res.Mode != "admin" || strings.Join(res.AllowedEnvKinds, ",") != "dev,test" {
			t.Fatalf("res = %+v", res)
		}
		entries, _ := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
		found := false
		for _, e := range entries {
			if e.Method == "PUT" && e.Path == "/api/v1/settings/ai-control" && e.StatusCode == 200 {
				found = true
			}
		}
		if !found {
			t.Fatalf("no audit entry for PUT: %+v", entries)
		}
	})
	t.Run("invalid input", func(t *testing.T) {
		rt, _, tokens := gateFixture(t, "off", nil)
		for _, body := range []string{`{"mode":"bogus"}`, `{"mode":"observe","allowed_env_kinds":["moon"]}`, `nope`} {
			if rec := gateDo(rt, "PUT", "/api/v1/settings/ai-control", tokens["human-root"], body); rec.Code != 400 {
				t.Errorf("%s: got %d", body, rec.Code)
			}
		}
	})
	t.Run("get is readable by a read agent in observe", func(t *testing.T) {
		rt, _, tokens := gateFixture(t, "observe", []string{"dev"})
		rec := gateDo(rt, "GET", "/api/v1/settings/ai-control", tokens["agent-write"], "")
		if rec.Code != 200 {
			t.Fatalf("got %d", rec.Code)
		}
		var res aiControlResource
		_ = json.Unmarshal(rec.Body.Bytes(), &res)
		if res.AgentTokenCount != 2 {
			t.Fatalf("agent token count = %d, want 2 (internal excluded)", res.AgentTokenCount)
		}
	})
}

func TestRevokeAgentTokens_OnlyAgentTokens(t *testing.T) {
	rt, db, tokens := gateFixture(t, "operate", []string{"dev"})
	rec := gateDo(rt, "POST", "/api/v1/settings/ai-control/revoke-agent-tokens", tokens["human-root"], "")
	if rec.Code != 200 {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	var res revokeAgentTokensResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res.Revoked != 2 {
		t.Fatalf("revoked = %d, want 2", res.Revoked)
	}
	for _, name := range []string{"agent-write", "agent-root"} {
		if rec := gateDo(rt, "GET", "/api/v1/apps", tokens[name], ""); rec.Code != 401 {
			t.Errorf("%s status = %d, want 401", name, rec.Code)
		}
	}
	for _, name := range []string{"human-root", "internal"} {
		if rec := gateDo(rt, "GET", "/api/v1/apps", tokens[name], ""); rec.Code != 200 {
			t.Errorf("%s status = %d, want 200", name, rec.Code)
		}
	}
	if n, _ := db.CountAgentTokens(context.Background(), AIAssistantTokenName, AIAssistantAgentName); n != 0 {
		t.Errorf("count = %d", n)
	}
}

func TestLabelUnlabeledTokensByName(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, _, err := MintAPIToken(ctx, db, AIAssistantTokenName, []string{AbilityRoot}, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := db.LabelUnlabeledTokensByName(ctx, AIAssistantTokenName, AIAssistantAgentName); err != nil {
			t.Fatal(err)
		}
	}
	toks, _ := db.ListAPITokens(ctx)
	if len(toks) != 1 || toks[0].AgentName != AIAssistantAgentName {
		t.Fatalf("tokens = %+v", toks)
	}
}
