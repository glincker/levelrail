package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/firewall"
	"github.com/GLINCKER/levelrail/internal/store"
)

// FirewallRuleStore is the store surface the firewall rule handlers
// need.
type FirewallRuleStore interface {
	SaveFirewallRule(ctx context.Context, r store.FirewallRule) error
	ListFirewallRules(ctx context.Context) ([]store.FirewallRule, error)
	DeleteFirewallRule(ctx context.Context, id string) error
}

// firewallRuleResource is the wire shape for a stored firewall rule.
type firewallRuleResource struct {
	ID         string `json:"id"`
	NodeID     string `json:"node_id"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	SourceCIDR string `json:"source_cidr,omitempty"`
	Action     string `json:"action"`
	Label      string `json:"label,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func toFirewallRuleResource(r store.FirewallRule) firewallRuleResource {
	return firewallRuleResource{
		ID:         r.ID,
		NodeID:     r.NodeID,
		Port:       r.Port,
		Protocol:   r.Protocol,
		SourceCIDR: r.SourceCIDR,
		Action:     string(r.Action),
		Label:      r.Label,
		CreatedAt:  r.CreatedAt,
	}
}

// createFirewallRuleRequest is handleCreateFirewallRule's request body.
type createFirewallRuleRequest struct {
	Port       int    `json:"port"`
	Protocol   string `json:"protocol,omitempty"`
	SourceCIDR string `json:"source_cidr,omitempty"`
	Action     string `json:"action,omitempty"`
	Label      string `json:"label,omitempty"`
}

func validateCreateFirewallRuleRequest(req createFirewallRuleRequest) (store.FirewallRule, error) {
	if req.Port < 1 || req.Port > 65535 {
		return store.FirewallRule{}, errors.New("port must be between 1 and 65535")
	}
	proto := req.Protocol
	if proto == "" {
		proto = "tcp"
	}
	if proto != "tcp" && proto != "udp" {
		return store.FirewallRule{}, errors.New("protocol must be \"tcp\" or \"udp\"")
	}
	action := req.Action
	if action == "" {
		action = string(store.FirewallRuleActionAllow)
	}
	if action != string(store.FirewallRuleActionAllow) && action != string(store.FirewallRuleActionDeny) {
		return store.FirewallRule{}, errors.New("action must be \"allow\" or \"deny\"")
	}
	if req.SourceCIDR != "" {
		if _, _, err := net.ParseCIDR(req.SourceCIDR); err != nil {
			return store.FirewallRule{}, fmt.Errorf("source_cidr %q is not a valid CIDR: %w", req.SourceCIDR, err)
		}
	}
	return store.FirewallRule{
		Port:       req.Port,
		Protocol:   proto,
		SourceCIDR: req.SourceCIDR,
		Action:     store.FirewallRuleAction(action),
		Label:      req.Label,
	}, nil
}

// handleListFirewallRules handles GET /api/v1/firewall-rules.
func (rt *Router) handleListFirewallRules(w http.ResponseWriter, r *http.Request) {
	rules, err := rt.firewallRules.ListFirewallRules(r.Context())
	if err != nil {
		rt.logger.Error("api: list firewall rules failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]firewallRuleResource, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toFirewallRuleResource(rule))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateFirewallRule handles POST /api/v1/firewall-rules.
// AbilityWriteSensitive. Checked against internal/firewall.Validate
// before it ever reaches the store, so an unsafe rule is refused here,
// not just caught later by the reconciler's own identical check.
func (rt *Router) handleCreateFirewallRule(w http.ResponseWriter, r *http.Request) {
	var req createFirewallRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rule, err := validateCreateFirewallRuleRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	safetyCheck := firewall.Rule{Port: rule.Port, Proto: rule.Protocol, SourceCIDR: rule.SourceCIDR, Action: firewall.Action(rule.Action)}
	if err := firewall.Validate(safetyCheck, rt.firewallRequiredPorts); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := randomFirewallRuleID()
	if err != nil {
		rt.logger.Error("api: create firewall rule: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rule.ID = id
	rule.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := rt.firewallRules.SaveFirewallRule(r.Context(), rule); err != nil {
		rt.logger.Error("api: create firewall rule: save failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusCreated, toFirewallRuleResource(rule))
}

// handleDeleteFirewallRule handles DELETE /api/v1/firewall-rules/{id}.
// AbilityWriteSensitive, same tier as create.
func (rt *Router) handleDeleteFirewallRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := rt.firewallRules.DeleteFirewallRule(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrFirewallRuleNotFound) {
			writeError(w, http.StatusNotFound, "firewall rule not found")
			return
		}
		rt.logger.Error("api: delete firewall rule failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.nudgeReconciler()
	w.WriteHeader(http.StatusNoContent)
}

// randomFirewallRuleID mirrors randomRegistryCredentialID's exact shape.
func randomFirewallRuleID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate firewall rule id: %w", err)
	}
	return "fwrule_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
