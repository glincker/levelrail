package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/edgefw"
)

const firewallEdgeBase = "/api/v1/firewall/edge"

// FirewallEdgeHits are one rule's counters since the process started.
type FirewallEdgeHits struct {
	Total     uint64   `json:"total"`
	LastHitAt string   `json:"last_hit_at,omitempty"`
	Sparkline []uint32 `json:"sparkline"`
}

// FirewallEdgeRule is a rule with its hit counters.
type FirewallEdgeRule struct {
	edgefw.Rule
	Hits FirewallEdgeHits `json:"hits"`
}

// FirewallEdgeAttack is the attack mode state.
type FirewallEdgeAttack struct {
	Active    bool   `json:"active"`
	Until     string `json:"until,omitempty"`
	Challenge bool   `json:"challenge"`
}

// FirewallEdgeStatus is GET .../firewall/edge/status.
type FirewallEdgeStatus struct {
	Active       bool                    `json:"active"`
	Attack       FirewallEdgeAttack      `json:"attack_mode"`
	RulesTotal   int                     `json:"rules_total"`
	RulesEnabled int                     `json:"rules_enabled"`
	BotMode      edgefw.Mode             `json:"bot_mode"`
	AIBotMode    edgefw.Mode             `json:"ai_bot_mode"`
	BansEnabled  bool                    `json:"bans_enabled"`
	Bans         int                     `json:"bans"`
	GeoActive    bool                    `json:"geo_active"`
	Range        string                  `json:"range"`
	Totals       map[string]uint64       `json:"totals"`
	Reasons      []edgefw.ReasonCountRow `json:"reasons"`
	Suggestion   struct {
		Suggested  bool `json:"suggested"`
		PerMinute  int  `json:"per_minute"`
		Threshold  int  `json:"threshold"`
		WindowMins int  `json:"window_minutes"`
	} `json:"attack_suggestion"`
	Retention struct {
		ChartHours   int    `json:"chart_hours"`
		TopMinutes   int    `json:"top_minutes"`
		InMemoryOnly bool   `json:"in_memory_only"`
		TopCapacity  int    `json:"top_capacity"`
		BanDuration  string `json:"ban_duration"`
		BanStrikes   int    `json:"ban_strikes"`
		BanWindow    string `json:"ban_window"`
	} `json:"retention"`
}

// FirewallEdgeTopRow is one approximate top-N entry.
type FirewallEdgeTopRow struct {
	Key       string   `json:"key"`
	Count     uint64   `json:"count"`
	Sparkline []uint32 `json:"sparkline"`
}

// FirewallEdgeTraffic is GET .../firewall/edge/traffic.
type FirewallEdgeTraffic struct {
	WindowMinutes int                             `json:"window_minutes"`
	Approximate   bool                            `json:"approximate"`
	Tables        map[string][]FirewallEdgeTopRow `json:"tables"`
}

// FirewallEdgeBan is one automatic temporary ban.
type FirewallEdgeBan struct {
	IP      string `json:"ip"`
	Reason  string `json:"reason"`
	Started string `json:"started"`
	Ends    string `json:"ends"`
	Hits    uint64 `json:"hits"`
}

// FirewallEdgeSettings is GET/PUT .../firewall/edge/settings.
type FirewallEdgeSettings struct {
	edgefw.Settings
	DefaultAIBotAgents []string `json:"default_ai_bot_agents"`
	MaxAIBotAgents     int      `json:"max_ai_bot_agents"`
}

// FirewallEdgeIPEntry is the body of the block and allow calls.
type FirewallEdgeIPEntry struct {
	IP    string `json:"ip"`
	Scope string `json:"scope,omitempty"`
	Note  string `json:"note,omitempty"`
}

func firewallQuery(scope, rng string) string {
	q := url.Values{}
	if scope != "" {
		q.Set("scope", scope)
	}
	if rng != "" {
		q.Set("range", rng)
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// FirewallEdgeStatus calls GET .../firewall/edge/status.
func (c *Client) FirewallEdgeStatus(ctx context.Context, scope, rng string) (FirewallEdgeStatus, error) {
	var out FirewallEdgeStatus
	err := c.do(ctx, http.MethodGet, firewallEdgeBase+"/status"+firewallQuery(scope, rng), nil, &out)
	return out, err
}

// FirewallEdgeTraffic calls GET .../firewall/edge/traffic.
func (c *Client) FirewallEdgeTraffic(ctx context.Context, scope string) (FirewallEdgeTraffic, error) {
	var out FirewallEdgeTraffic
	err := c.do(ctx, http.MethodGet, firewallEdgeBase+"/traffic"+firewallQuery(scope, ""), nil, &out)
	return out, err
}

// ListFirewallEdgeRules calls GET .../firewall/edge/rules.
func (c *Client) ListFirewallEdgeRules(ctx context.Context) ([]FirewallEdgeRule, error) {
	var out []FirewallEdgeRule
	err := c.do(ctx, http.MethodGet, firewallEdgeBase+"/rules", nil, &out)
	return out, err
}

// CreateFirewallEdgeRule calls POST .../firewall/edge/rules.
func (c *Client) CreateFirewallEdgeRule(ctx context.Context, r edgefw.Rule, force bool) (FirewallEdgeRule, error) {
	var out FirewallEdgeRule
	err := c.do(ctx, http.MethodPost, firewallEdgeBase+"/rules"+forceQuery(force), r, &out)
	return out, err
}

// UpdateFirewallEdgeRule calls PUT .../firewall/edge/rules/{id}.
func (c *Client) UpdateFirewallEdgeRule(ctx context.Context, id string, r edgefw.Rule, force bool) (FirewallEdgeRule, error) {
	var out FirewallEdgeRule
	err := c.do(ctx, http.MethodPut, firewallEdgeBase+"/rules/"+url.PathEscape(id)+forceQuery(force), r, &out)
	return out, err
}

// DeleteFirewallEdgeRule calls DELETE .../firewall/edge/rules/{id}.
func (c *Client) DeleteFirewallEdgeRule(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, firewallEdgeBase+"/rules/"+url.PathEscape(id), nil, nil)
}

// ReorderFirewallEdgeRules calls POST .../firewall/edge/rules/order.
func (c *Client) ReorderFirewallEdgeRules(ctx context.Context, ids []string) ([]FirewallEdgeRule, error) {
	var out []FirewallEdgeRule
	err := c.do(ctx, http.MethodPost, firewallEdgeBase+"/rules/order", map[string][]string{"ids": ids}, &out)
	return out, err
}

// BlockFirewallEdgeIP calls POST .../firewall/edge/block.
func (c *Client) BlockFirewallEdgeIP(ctx context.Context, e FirewallEdgeIPEntry, force bool) (FirewallEdgeRule, error) {
	var out FirewallEdgeRule
	err := c.do(ctx, http.MethodPost, firewallEdgeBase+"/block"+forceQuery(force), e, &out)
	return out, err
}

// AllowFirewallEdgeIP calls POST .../firewall/edge/allow.
func (c *Client) AllowFirewallEdgeIP(ctx context.Context, e FirewallEdgeIPEntry) (FirewallEdgeRule, error) {
	var out FirewallEdgeRule
	err := c.do(ctx, http.MethodPost, firewallEdgeBase+"/allow", e, &out)
	return out, err
}

// ListFirewallEdgeBans calls GET .../firewall/edge/bans.
func (c *Client) ListFirewallEdgeBans(ctx context.Context) ([]FirewallEdgeBan, error) {
	var out []FirewallEdgeBan
	err := c.do(ctx, http.MethodGet, firewallEdgeBase+"/bans", nil, &out)
	return out, err
}

// LiftFirewallEdgeBan calls DELETE .../firewall/edge/bans/{ip}.
func (c *Client) LiftFirewallEdgeBan(ctx context.Context, ip string) error {
	return c.do(ctx, http.MethodDelete, firewallEdgeBase+"/bans/"+url.PathEscape(ip), nil, nil)
}

// FirewallEdgeSettings calls GET .../firewall/edge/settings.
func (c *Client) FirewallEdgeSettings(ctx context.Context) (FirewallEdgeSettings, error) {
	var out FirewallEdgeSettings
	err := c.do(ctx, http.MethodGet, firewallEdgeBase+"/settings", nil, &out)
	return out, err
}

// SetFirewallEdgeSettings calls PUT .../firewall/edge/settings.
func (c *Client) SetFirewallEdgeSettings(ctx context.Context, s edgefw.Settings) (FirewallEdgeSettings, error) {
	var out FirewallEdgeSettings
	err := c.do(ctx, http.MethodPut, firewallEdgeBase+"/settings", s, &out)
	return out, err
}

// StartFirewallEdgeAttack calls POST .../firewall/edge/attack-mode.
func (c *Client) StartFirewallEdgeAttack(ctx context.Context, minutes int, challenge bool) (FirewallEdgeAttack, error) {
	var out FirewallEdgeAttack
	body := struct {
		Minutes   int  `json:"minutes,omitempty"`
		Challenge bool `json:"challenge"`
	}{minutes, challenge}
	err := c.do(ctx, http.MethodPost, firewallEdgeBase+"/attack-mode", body, &out)
	return out, err
}

// StopFirewallEdgeAttack calls DELETE .../firewall/edge/attack-mode.
func (c *Client) StopFirewallEdgeAttack(ctx context.Context) (FirewallEdgeAttack, error) {
	var out FirewallEdgeAttack
	err := c.do(ctx, http.MethodDelete, firewallEdgeBase+"/attack-mode", nil, &out)
	return out, err
}

// FirewallEdgeAuditEntry is one firewall change event.
type FirewallEdgeAuditEntry struct {
	ID        string          `json:"id"`
	ActorName string          `json:"actor_name"`
	ActorType string          `json:"actor_type"`
	Action    string          `json:"action"`
	CreatedAt string          `json:"created_at"`
	Detail    json.RawMessage `json:"detail,omitempty"`
}

// FirewallEdgeAudit calls GET .../firewall/edge/audit.
func (c *Client) FirewallEdgeAudit(ctx context.Context, limit int) ([]FirewallEdgeAuditEntry, error) {
	var out []FirewallEdgeAuditEntry
	path := firewallEdgeBase + "/audit"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func forceQuery(force bool) string {
	if force {
		return "?force=true"
	}
	return ""
}
