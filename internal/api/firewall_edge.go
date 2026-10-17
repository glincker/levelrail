package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/edgefw"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EdgeFirewallStore is the store surface for the ingress firewall rules and
// settings.
type EdgeFirewallStore interface {
	ListEdgeFirewallRules(ctx context.Context) ([]store.EdgeFirewallRule, error)
	GetEdgeFirewallRule(ctx context.Context, id string) (store.EdgeFirewallRule, error)
	CountEdgeFirewallRules(ctx context.Context) (int, error)
	CreateEdgeFirewallRule(ctx context.Context, r store.EdgeFirewallRule) (store.EdgeFirewallRule, error)
	UpdateEdgeFirewallRule(ctx context.Context, r store.EdgeFirewallRule) error
	DeleteEdgeFirewallRule(ctx context.Context, id string) error
	ReorderEdgeFirewallRules(ctx context.Context, ids []string) error
	GetEdgeFirewallSettings(ctx context.Context) (string, error)
	SetEdgeFirewallSettings(ctx context.Context, settings string) error
}

// edgeFirewallDeps serializes firewall writes so the stored rules and the
// running engine never diverge.
type edgeFirewallDeps struct {
	store  EdgeFirewallStore
	engine *edgefw.Engine
	mu     sync.Mutex
}

const maxFirewallBody = 64 << 10

// Audit action names for firewall changes; the Firewall audit view lists
// exactly the rows whose action starts with AuditActionFirewallPrefix.
const (
	AuditActionFirewallPrefix   = "firewall."
	auditFirewallRuleCreated    = "firewall.rule_created"
	auditFirewallRuleUpdated    = "firewall.rule_updated"
	auditFirewallRuleDeleted    = "firewall.rule_deleted"
	auditFirewallRulesReordered = "firewall.rules_reordered"
	auditFirewallIPBlocked      = "firewall.ip_blocked"
	auditFirewallIPAllowed      = "firewall.ip_allowed"
	auditFirewallBanLifted      = "firewall.ban_lifted"
	auditFirewallSettings       = "firewall.settings_changed"
	auditFirewallAttackOn       = "firewall.attack_mode_on"
	auditFirewallAttackOff      = "firewall.attack_mode_off"
)

func newFirewallRuleID() (string, error) {
	b := make([]byte, 9)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("api: generate firewall rule id: %w", err)
	}
	return "fwr_" + base64.RawURLEncoding.EncodeToString(b), nil
}

func ruleFromRow(row store.EdgeFirewallRule) (edgefw.Rule, error) {
	r := edgefw.Rule{
		ID: row.ID, Name: row.Name, Note: row.Note, Scope: row.Scope, Enabled: row.Enabled, Position: row.Position,
		Action: edgefw.Action(row.Action), RateCount: row.RateCount, RateWindowSeconds: row.RateWindowSeconds,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if err := json.Unmarshal([]byte(row.Conditions), &r.Conditions); err != nil {
		return edgefw.Rule{}, fmt.Errorf("api: decode firewall rule %q conditions: %w", row.ID, err)
	}
	return r, nil
}

func rowFromRule(r edgefw.Rule) (store.EdgeFirewallRule, error) {
	raw, err := json.Marshal(r.Conditions)
	if err != nil {
		return store.EdgeFirewallRule{}, fmt.Errorf("api: encode firewall rule conditions: %w", err)
	}
	return store.EdgeFirewallRule{
		ID: r.ID, Name: r.Name, Note: r.Note, Scope: r.Scope, Enabled: r.Enabled, Position: r.Position,
		Action: string(r.Action), Conditions: string(raw), RateCount: r.RateCount, RateWindowSeconds: r.RateWindowSeconds,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}, nil
}

func (d *edgeFirewallDeps) loadRules(ctx context.Context) ([]edgefw.Rule, error) {
	rows, err := d.store.ListEdgeFirewallRules(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: list firewall rules: %w", err)
	}
	out := make([]edgefw.Rule, 0, len(rows))
	for _, row := range rows {
		r, err := ruleFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

func (d *edgeFirewallDeps) loadSettings(ctx context.Context) (edgefw.Settings, error) {
	raw, err := d.store.GetEdgeFirewallSettings(ctx)
	if err != nil {
		return edgefw.Settings{}, fmt.Errorf("api: load firewall settings: %w", err)
	}
	s := edgefw.DefaultSettings()
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return edgefw.Settings{}, fmt.Errorf("api: decode firewall settings: %w", err)
	}
	return s, nil
}

// LoadEdgeFirewall installs the stored rules and settings into the running
// engine; call it once at startup.
func (rt *Router) LoadEdgeFirewall(ctx context.Context) error {
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	return rt.syncEdgeFirewall(ctx)
}

func (rt *Router) syncEdgeFirewall(ctx context.Context) error {
	d := &rt.edgeFW
	rules, err := d.loadRules(ctx)
	if err != nil {
		return err
	}
	settings, err := d.loadSettings(ctx)
	if err != nil {
		return err
	}
	d.engine.SetRules(rules)
	d.engine.SetSettings(settings)
	return nil
}

func (rt *Router) auditFirewall(r *http.Request, action string, detail map[string]string) {
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: firewall audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: r.Method, Path: "/api/v1/firewall/edge", StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: firewall audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

type edgeFirewallRuleHits struct {
	Total      uint64   `json:"total"`
	LastHitAt  string   `json:"last_hit_at,omitempty"`
	Sparkline  []uint32 `json:"sparkline"`
	SinceStart bool     `json:"since_start"`
}

type edgeFirewallRuleResource struct {
	edgefw.Rule
	Hits edgeFirewallRuleHits `json:"hits"`
}

func (rt *Router) ruleResource(r edgefw.Rule, now time.Time) edgeFirewallRuleResource {
	h := rt.edgeFW.engine.Stats.RuleHits(r.ID, now)
	res := edgeFirewallRuleResource{Rule: r, Hits: edgeFirewallRuleHits{Total: h.Total, Sparkline: h.Spark[:], SinceStart: true}}
	if !h.Last.IsZero() {
		res.Hits.LastHitAt = h.Last.UTC().Format(time.RFC3339)
	}
	return res
}

func writeFirewallValidation(w http.ResponseWriter, err error) bool {
	var ve *edgefw.ValidationError
	if !errors.As(err, &ve) {
		return false
	}
	writeJSON(w, http.StatusBadRequest, struct {
		Error  string              `json:"error"`
		Fields []edgefw.FieldError `json:"fields"`
	}{Error: "the rule is not valid", Fields: ve.Fields})
	return true
}

// handleListFirewallEdgeRules handles GET /api/v1/firewall/edge/rules.
func (rt *Router) handleListFirewallEdgeRules(w http.ResponseWriter, r *http.Request) {
	rules, err := rt.edgeFW.loadRules(r.Context())
	if err != nil {
		rt.internalError(w, "api: list firewall rules failed", err)
		return
	}
	now := time.Now()
	out := make([]edgeFirewallRuleResource, 0, len(rules))
	for _, rule := range rules {
		out = append(out, rt.ruleResource(rule, now))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) decodeFirewallRule(w http.ResponseWriter, r *http.Request) (edgefw.Rule, bool) {
	var rule edgefw.Rule
	if err := decodeJSONBodyLimit(r, &rule, maxFirewallBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return rule, false
	}
	edgefw.Normalize(&rule)
	if err := edgefw.ValidateRule(&rule); err != nil {
		writeFirewallValidation(w, err)
		return rule, false
	}
	return rule, true
}

// selfLockout reports an IP-list deny rule whose range holds the caller.
func selfLockout(r *http.Request, rule edgefw.Rule) (string, bool) {
	if a, ok := rule.IPList(); !ok || a != edgefw.ActionDeny {
		return "", false
	}
	me, err := netip.ParseAddr(clientIP(r))
	if err != nil {
		return "", false
	}
	p, err := netip.ParsePrefix(rule.Conditions[0].Value)
	if err != nil {
		a, aerr := netip.ParseAddr(rule.Conditions[0].Value)
		if aerr != nil {
			return "", false
		}
		p = netip.PrefixFrom(a, a.BitLen())
	}
	return me.String(), p.Contains(me.Unmap()) || p.Contains(me)
}

func (rt *Router) lockoutRefused(w http.ResponseWriter, r *http.Request, rule edgefw.Rule) bool {
	if me, hit := selfLockout(r, rule); hit && r.URL.Query().Get("force") != "true" {
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "this rule would block your own address (" + me + "); add an allow rule for it first, or confirm to continue",
			"code":  "self_lockout",
		})
		return true
	}
	return false
}

// insertIndex places allow IP entries first and IP blocks right after them,
// so an allow always wins over a block.
func insertIndex(rules []edgefw.Rule, a edgefw.Action) int {
	if a == edgefw.ActionAllow {
		return 0
	}
	n := 0
	for _, r := range rules {
		if r.Action != edgefw.ActionAllow {
			break
		}
		n++
	}
	return n
}

func (rt *Router) createFirewallRule(r *http.Request, rule edgefw.Rule, front bool) (edgefw.Rule, error) {
	d := &rt.edgeFW
	ctx := r.Context()
	n, err := d.store.CountEdgeFirewallRules(ctx)
	if err != nil {
		return edgefw.Rule{}, fmt.Errorf("api: count firewall rules: %w", err)
	}
	if n >= d.engine.Limits().MaxRules {
		return edgefw.Rule{}, &edgefw.ValidationError{Fields: []edgefw.FieldError{{Field: "rules", Message: fmt.Sprintf("at most %d rules", d.engine.Limits().MaxRules)}}}
	}
	if rule.ID, err = newFirewallRuleID(); err != nil {
		return edgefw.Rule{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	rule.CreatedAt, rule.UpdatedAt, rule.Position = now, now, 0
	row, err := rowFromRule(rule)
	if err != nil {
		return edgefw.Rule{}, err
	}
	if _, err := d.store.CreateEdgeFirewallRule(ctx, row); err != nil {
		return edgefw.Rule{}, fmt.Errorf("api: create firewall rule: %w", err)
	}
	if front {
		existing, err := d.loadRules(ctx)
		if err != nil {
			return edgefw.Rule{}, err
		}
		idx := insertIndex(existing[:len(existing)-1], rule.Action)
		ids := make([]string, 0, len(existing))
		for i, e := range existing[:len(existing)-1] {
			if i == idx {
				ids = append(ids, rule.ID)
			}
			ids = append(ids, e.ID)
		}
		if idx >= len(existing)-1 {
			ids = append(ids, rule.ID)
		}
		if err := d.store.ReorderEdgeFirewallRules(ctx, ids); err != nil {
			return edgefw.Rule{}, fmt.Errorf("api: place firewall rule: %w", err)
		}
	}
	if err := rt.syncEdgeFirewall(ctx); err != nil {
		return edgefw.Rule{}, err
	}
	saved, err := d.store.GetEdgeFirewallRule(ctx, rule.ID)
	if err != nil {
		return edgefw.Rule{}, fmt.Errorf("api: reload firewall rule: %w", err)
	}
	return ruleFromRow(saved)
}

// handleCreateFirewallEdgeRule handles POST /api/v1/firewall/edge/rules.
func (rt *Router) handleCreateFirewallEdgeRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := rt.decodeFirewallRule(w, r)
	if !ok || rt.lockoutRefused(w, r, rule) {
		return
	}
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	saved, err := rt.createFirewallRule(r, rule, false)
	if err != nil {
		if !writeFirewallValidation(w, err) {
			rt.internalError(w, "api: create firewall rule failed", err)
		}
		return
	}
	rt.auditFirewall(r, auditFirewallRuleCreated, map[string]string{"id": saved.ID, "name": saved.Name, "action": string(saved.Action), "scope": saved.Scope})
	writeJSON(w, http.StatusCreated, rt.ruleResource(saved, time.Now()))
}

// handleUpdateFirewallEdgeRule handles PUT /api/v1/firewall/edge/rules/{id}.
func (rt *Router) handleUpdateFirewallEdgeRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := rt.decodeFirewallRule(w, r)
	if !ok || rt.lockoutRefused(w, r, rule) {
		return
	}
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	rule.ID = r.PathValue("id")
	rule.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	row, err := rowFromRule(rule)
	if err != nil {
		rt.internalError(w, "api: encode firewall rule failed", err)
		return
	}
	if err := d.store.UpdateEdgeFirewallRule(r.Context(), row); err != nil {
		if errors.Is(err, store.ErrEdgeFirewallRuleNotFound) {
			writeError(w, http.StatusNotFound, "firewall rule not found")
			return
		}
		rt.internalError(w, "api: update firewall rule failed", err)
		return
	}
	if err := rt.syncEdgeFirewall(r.Context()); err != nil {
		rt.internalError(w, "api: sync firewall failed", err)
		return
	}
	saved, err := d.store.GetEdgeFirewallRule(r.Context(), rule.ID)
	if err != nil {
		rt.internalError(w, "api: reload firewall rule failed", err)
		return
	}
	out, err := ruleFromRow(saved)
	if err != nil {
		rt.internalError(w, "api: decode firewall rule failed", err)
		return
	}
	rt.auditFirewall(r, auditFirewallRuleUpdated, map[string]string{"id": out.ID, "name": out.Name, "action": string(out.Action), "enabled": fmt.Sprint(out.Enabled)})
	writeJSON(w, http.StatusOK, rt.ruleResource(out, time.Now()))
}

// handleDeleteFirewallEdgeRule handles DELETE /api/v1/firewall/edge/rules/{id}.
func (rt *Router) handleDeleteFirewallEdgeRule(w http.ResponseWriter, r *http.Request) {
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	id := r.PathValue("id")
	old, err := d.store.GetEdgeFirewallRule(r.Context(), id)
	if errors.Is(err, store.ErrEdgeFirewallRuleNotFound) {
		writeError(w, http.StatusNotFound, "firewall rule not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get firewall rule failed", err)
		return
	}
	if err := d.store.DeleteEdgeFirewallRule(r.Context(), id); err != nil {
		rt.internalError(w, "api: delete firewall rule failed", err)
		return
	}
	if err := rt.syncEdgeFirewall(r.Context()); err != nil {
		rt.internalError(w, "api: sync firewall failed", err)
		return
	}
	rt.auditFirewall(r, auditFirewallRuleDeleted, map[string]string{"id": id, "name": old.Name})
	w.WriteHeader(http.StatusNoContent)
}

type firewallOrderRequest struct {
	IDs []string `json:"ids"`
}

// handleReorderFirewallEdgeRules handles POST /api/v1/firewall/edge/rules/order.
func (rt *Router) handleReorderFirewallEdgeRules(w http.ResponseWriter, r *http.Request) {
	var req firewallOrderRequest
	if err := decodeJSONBodyLimit(r, &req, maxFirewallBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	seen := map[string]bool{}
	for _, id := range req.IDs {
		if seen[id] {
			writeError(w, http.StatusBadRequest, "ids must name each rule once")
			return
		}
		seen[id] = true
	}
	if err := d.store.ReorderEdgeFirewallRules(r.Context(), req.IDs); err != nil {
		if errors.Is(err, store.ErrEdgeFirewallRuleNotFound) {
			writeError(w, http.StatusNotFound, "firewall rule not found")
			return
		}
		writeError(w, http.StatusBadRequest, "ids must name every rule exactly once")
		return
	}
	if err := rt.syncEdgeFirewall(r.Context()); err != nil {
		rt.internalError(w, "api: sync firewall failed", err)
		return
	}
	rt.auditFirewall(r, auditFirewallRulesReordered, map[string]string{"count": fmt.Sprint(len(req.IDs))})
	rt.handleListFirewallEdgeRules(w, r)
}

type firewallIPRequest struct {
	IP    string `json:"ip"`
	Scope string `json:"scope,omitempty"`
	Note  string `json:"note,omitempty"`
}

func (rt *Router) handleFirewallIPEntry(action edgefw.Action, audit string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req firewallIPRequest
		if err := decodeJSONBodyLimit(r, &req, maxFirewallBody); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		rule, err := edgefw.IPRule(action, req.IP, req.Scope, req.Note)
		if err != nil {
			if !writeFirewallValidation(w, err) {
				writeError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		if rt.lockoutRefused(w, r, rule) {
			return
		}
		d := &rt.edgeFW
		d.mu.Lock()
		defer d.mu.Unlock()
		saved, err := rt.createFirewallRule(r, rule, true)
		if err != nil {
			if !writeFirewallValidation(w, err) {
				rt.internalError(w, "api: create firewall ip rule failed", err)
			}
			return
		}
		rt.auditFirewall(r, audit, map[string]string{"id": saved.ID, "ip": saved.Conditions[0].Value, "scope": saved.Scope})
		writeJSON(w, http.StatusCreated, rt.ruleResource(saved, time.Now()))
	}
}
