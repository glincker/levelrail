package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/edgefw"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const fwBase = "/api/v1/firewall/edge"

func fwRouter(t *testing.T) (*Router, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.edgeFW.engine = edgefw.New(edgefw.DefaultLimits(), trafficpolicy.DefaultLimits())
	if err := rt.LoadEdgeFirewall(t.Context()); err != nil {
		t.Fatal(err)
	}
	return rt, loginTestSession(t, rt, db)
}

func fwCall(t *testing.T, rt *Router, c *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doPolicy(t, rt, c, method, fwBase+path, body)
}

func fwCreate(t *testing.T, rt *Router, c *http.Cookie) edgeFirewallRuleResource {
	t.Helper()
	rec := fwCall(t, rt, c, http.MethodPost, "/rules", denyAdmin)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	var out edgeFirewallRuleResource
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

const denyAdmin = `{"name":"Block admin","enabled":true,"action":"deny","conditions":[{"field":"path","op":"starts_with","value":"/admin"}]}`

func TestFirewallEdge_RuleCRUDAndEngineSync(t *testing.T) {
	rt, c := fwRouter(t)
	r := fwCreate(t, rt, c)
	if r.ID == "" || r.Position != 1 || !r.Enabled {
		t.Fatalf("created = %+v", r)
	}
	req := &edgefw.Request{Method: "GET", Host: "a.example.com", Path: "/admin/x"}
	if v := rt.edgeFW.engine.Evaluate(req, time.Now()); v.Action != edgefw.ActionDeny {
		t.Fatalf("engine did not pick up the rule: %+v", v)
	}
	off := strings.Replace(denyAdmin, `"enabled":true`, `"enabled":false`, 1)
	if rec := fwCall(t, rt, c, http.MethodPut, "/rules/"+r.ID, off); rec.Code != 200 {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	if v := rt.edgeFW.engine.Evaluate(req, time.Now()); v.Action != "" {
		t.Fatalf("disabled rule still enforced: %+v", v)
	}
	if rec := fwCall(t, rt, c, http.MethodPut, "/rules/fwr_missing", denyAdmin); rec.Code != 404 {
		t.Fatalf("update missing = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/rules/"+r.ID, ""); rec.Code != 204 {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/rules/"+r.ID, ""); rec.Code != 404 {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func TestFirewallEdge_ValidationErrorsNameFields(t *testing.T) {
	rt, c := fwRouter(t)
	tests := []struct{ name, body, field string }{
		{"no name", `{"name":"","enabled":true,"action":"deny","conditions":[{"field":"path","op":"equals","value":"/x"}]}`, "name"},
		{"bad action", `{"name":"n","action":"explode","conditions":[{"field":"path","op":"equals","value":"/x"}]}`, "action"},
		{"bad op", `{"name":"n","action":"deny","conditions":[{"field":"ip","op":"contains","value":"1.2.3.4"}]}`, "conditions[0].op"},
		{"rate missing", `{"name":"n","action":"rate_limit","conditions":[{"field":"path","op":"equals","value":"/x"}]}`, "rate_count"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := fwCall(t, rt, c, http.MethodPost, "/rules", tt.body)
			var out struct {
				Fields []edgefw.FieldError `json:"fields"`
			}
			if rec.Code != 400 || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Fields) == 0 || out.Fields[0].Field != tt.field {
				t.Fatalf("%d %s", rec.Code, rec.Body.String())
			}
		})
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/rules", `{not json`); rec.Code != 400 {
		t.Fatalf("bad json = %d", rec.Code)
	}
}

func TestFirewallEdge_OrderBlockAllowAndLockout(t *testing.T) {
	rt, c := fwRouter(t)
	a := fwCreate(t, rt, c)
	if rec := fwCall(t, rt, c, http.MethodPost, "/block", `{"ip":"203.0.113.0/24","note":"scanner"}`); rec.Code != 201 {
		t.Fatalf("block = %d %s", rec.Code, rec.Body.String())
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/allow", `{"ip":"198.51.100.9"}`); rec.Code != 201 {
		t.Fatalf("allow = %d", rec.Code)
	}
	var list []edgeFirewallRuleResource
	rec := fwCall(t, rt, c, http.MethodGet, "/rules", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 3 {
		t.Fatalf("list = %v %s", err, rec.Body.String())
	}
	if list[0].Action != edgefw.ActionAllow || list[1].Name != "Block 203.0.113.0/24" || list[2].ID != a.ID {
		t.Fatalf("placement wrong: %s %s %s", list[0].Name, list[1].Name, list[2].Name)
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/rules/order", `{"ids":["`+list[2].ID+`","`+list[0].ID+`","`+list[1].ID+`"]}`); rec.Code != 200 {
		t.Fatalf("reorder = %d %s", rec.Code, rec.Body.String())
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/rules/order", `{"ids":["`+list[0].ID+`"]}`); rec.Code != 400 {
		t.Fatalf("partial reorder = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/rules/order", `{"ids":["`+list[0].ID+`","`+list[0].ID+`","`+list[1].ID+`"]}`); rec.Code != 400 {
		t.Fatalf("duplicate reorder = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/block", `{"ip":"nope"}`); rec.Code != 400 {
		t.Fatalf("bad ip = %d", rec.Code)
	}
	self := "192.0.2.1"
	req := authedRequest(t, c, http.MethodPost, fwBase+"/block", `{"ip":"`+self+`"}`)
	req.RemoteAddr = self + ":5555"
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "self_lockout") {
		t.Fatalf("self block = %d %s", rec.Code, rec.Body.String())
	}
	req = authedRequest(t, c, http.MethodPost, fwBase+"/block?force=true", `{"ip":"`+self+`"}`)
	req.RemoteAddr = self + ":5555"
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("forced self block = %d", rec.Code)
	}
}

func TestFirewallEdge_RuleCap(t *testing.T) {
	rt, c := fwRouter(t)
	l := edgefw.DefaultLimits()
	l.MaxRules = 2
	rt.edgeFW.engine = edgefw.New(l, trafficpolicy.DefaultLimits())
	fwCreate(t, rt, c)
	fwCreate(t, rt, c)
	if rec := fwCall(t, rt, c, http.MethodPost, "/rules", denyAdmin); rec.Code != 400 {
		t.Fatalf("over cap = %d", rec.Code)
	}
}

func TestFirewallEdge_SettingsAttackBansAndAudit(t *testing.T) {
	rt, c := fwRouter(t)
	if rec := fwCall(t, rt, c, http.MethodPut, "/settings", `{"bot_mode":"loud","ai_bot_mode":"off"}`); rec.Code != 400 {
		t.Fatalf("bad mode = %d", rec.Code)
	}
	rec := fwCall(t, rt, c, http.MethodPut, "/settings", `{"bot_mode":"log","ai_bot_mode":"block","ai_bot_agents":["mycrawler"],"bans_enabled":false}`)
	if rec.Code != 200 {
		t.Fatalf("settings = %d %s", rec.Code, rec.Body.String())
	}
	if s := rt.edgeFW.engine.CurrentSettings(); s.AIBotMode != edgefw.ModeBlock || s.BansEnabled || s.AIBotAgents[0] != "mycrawler" {
		t.Fatalf("engine settings = %+v", s)
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/attack-mode", `{"minutes":99999}`); rec.Code != 400 {
		t.Fatalf("oversized attack = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodPost, "/attack-mode", `{"minutes":5,"challenge":true}`); rec.Code != 200 {
		t.Fatalf("attack on = %d %s", rec.Code, rec.Body.String())
	}
	if !rt.edgeFW.engine.CurrentSettings().AttackActive(time.Now()) {
		t.Fatal("attack mode not active in engine")
	}
	var st firewallStatusResource
	rec = fwCall(t, rt, c, http.MethodGet, "/status", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || !st.Attack.Active || !st.Attack.Challenge || st.Retention.ChartHours != 24 {
		t.Fatalf("status = %v %s", err, rec.Body.String())
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/attack-mode", ""); rec.Code != 200 {
		t.Fatalf("attack off = %d", rec.Code)
	}
	if rt.edgeFW.engine.CurrentSettings().AttackActive(time.Now()) {
		t.Fatal("attack mode still active")
	}

	rt.edgeFW.engine.Bans.Add(mustAddr("198.51.100.4"), "rule", time.Hour, time.Now())
	var bans []firewallBanResource
	rec = fwCall(t, rt, c, http.MethodGet, "/bans", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &bans); err != nil || len(bans) != 1 {
		t.Fatalf("bans = %v %s", err, rec.Body.String())
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/bans/198.51.100.4", ""); rec.Code != 204 {
		t.Fatalf("lift = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/bans/198.51.100.4", ""); rec.Code != 404 {
		t.Fatalf("lift again = %d", rec.Code)
	}
	if rec := fwCall(t, rt, c, http.MethodDelete, "/bans/not-an-ip", ""); rec.Code != 400 {
		t.Fatalf("lift junk = %d", rec.Code)
	}

	var audit []auditLogEntryResource
	rec = fwCall(t, rt, c, http.MethodGet, "/audit", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &audit); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range audit {
		if !strings.HasPrefix(e.Action, AuditActionFirewallPrefix) {
			t.Fatalf("non firewall row in the firewall audit view: %+v", e)
		}
		seen[e.Action] = true
	}
	for _, want := range []string{auditFirewallSettings, auditFirewallAttackOn, auditFirewallAttackOff, auditFirewallBanLifted} {
		if !seen[want] {
			t.Errorf("audit is missing %s (have %v)", want, seen)
		}
	}
}

func TestFirewallEdge_VisibilityEndpoints(t *testing.T) {
	rt, c := fwRouter(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		rt.edgeFW.engine.Stats.Observe(edgefw.Observation{At: now, Outcome: edgefw.OutcomeDenied, Reason: edgefw.ReasonRule, IP: mustAddr("198.51.100.4"), UA: "curl/8", Path: "/admin", Host: "a.example.com"})
	}
	var tr firewallTrafficResource
	rec := fwCall(t, rt, c, http.MethodGet, "/traffic", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &tr); err != nil || tr.Tables["ip"][0].Key != "198.51.100.4" || tr.Tables["ip"][0].Count != 5 || !tr.Approximate {
		t.Fatalf("traffic = %v %s", err, rec.Body.String())
	}
	var pts []firewallSeriesPoint
	rec = fwCall(t, rt, c, http.MethodGet, "/series?range=1h", "")
	var denied uint64
	if err := json.Unmarshal(rec.Body.Bytes(), &pts); err != nil {
		t.Fatal(err)
	}
	for _, p := range pts {
		denied += p.Counts["denied"]
	}
	if denied != 5 {
		t.Fatalf("series denied = %d", denied)
	}
	rec = fwCall(t, rt, c, http.MethodGet, "/status?scope=a.example.com", "")
	var st firewallStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.Totals["denied"] != 5 || len(st.Reasons) != 1 {
		t.Fatalf("status = %v %s", err, rec.Body.String())
	}
}

func TestFirewallEdge_AttackSuggestionIsDeterministic(t *testing.T) {
	rt, _ := fwRouter(t)
	l := edgefw.DefaultLimits()
	l.SuggestRate, l.SuggestMinutes = 10, 2
	rt.edgeFW.engine = edgefw.New(l, trafficpolicy.DefaultLimits())
	now := time.Now()
	if s := rt.attackSuggestion(now, rt.edgeFW.engine.CurrentSettings()); s.Suggested {
		t.Fatal("suggested with no traffic")
	}
	for i := 0; i < 20; i++ {
		rt.edgeFW.engine.Stats.Observe(edgefw.Observation{At: now, Outcome: edgefw.OutcomeChallenged})
	}
	st := rt.edgeFW.engine.CurrentSettings()
	if s := rt.attackSuggestion(now, st); !s.Suggested || s.PerMinute != 10 {
		t.Fatalf("suggestion = %+v", s)
	}
	if items := rt.firewallAttentionItems([]string{AbilityDeploy}); len(items) != 1 {
		t.Fatalf("attention items = %d", len(items))
	}
	if items := rt.firewallAttentionItems([]string{AbilityRead}); len(items) != 0 {
		t.Fatal("read-only callers should not get the item")
	}
	until := now.Add(time.Hour)
	st.AttackUntil = &until
	if s := rt.attackSuggestion(now, st); s.Suggested {
		t.Fatal("suggested while attack mode is on")
	}
}

func TestFirewallEdge_ReadOnlyTokenCannotChange(t *testing.T) {
	rt, _ := fwRouter(t)
	for _, tt := range []struct{ method, path string }{
		{http.MethodPost, "/rules"}, {http.MethodPut, "/rules/x"}, {http.MethodDelete, "/rules/x"}, {http.MethodPost, "/rules/order"},
		{http.MethodPost, "/block"}, {http.MethodPost, "/allow"}, {http.MethodDelete, "/bans/10.0.0.1"},
		{http.MethodPut, "/settings"}, {http.MethodPost, "/attack-mode"}, {http.MethodDelete, "/attack-mode"},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(tt.method, fwBase+tt.path, strings.NewReader("{}")))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without credentials = %d", tt.method, tt.path, rec.Code)
		}
	}
}

func mustAddr(s string) netip.Addr { return netip.MustParseAddr(s) }
