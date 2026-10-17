package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/edgefw"
)

// registerFirewallEdgeRoutes wires the ingress firewall. Reads that expose
// client addresses need read:sensitive; changes need deploy, the tier the
// per-domain shield routes use.
func (rt *Router) registerFirewallEdgeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/firewall/edge/status", rt.requireAbility(AbilityRead, rt.handleFirewallEdgeStatus))
	mux.HandleFunc("GET /api/v1/firewall/edge/series", rt.requireAbility(AbilityRead, rt.handleFirewallEdgeSeries))
	mux.HandleFunc("GET /api/v1/firewall/edge/traffic", rt.requireAbility(AbilityReadSensitive, rt.handleFirewallEdgeTraffic))
	mux.HandleFunc("GET /api/v1/firewall/edge/rules", rt.requireAbility(AbilityRead, rt.handleListFirewallEdgeRules))
	mux.HandleFunc("POST /api/v1/firewall/edge/rules", rt.requireAbility(AbilityDeploy, rt.handleCreateFirewallEdgeRule))
	mux.HandleFunc("POST /api/v1/firewall/edge/rules/order", rt.requireAbility(AbilityDeploy, rt.handleReorderFirewallEdgeRules))
	mux.HandleFunc("PUT /api/v1/firewall/edge/rules/{id}", rt.requireAbility(AbilityDeploy, rt.handleUpdateFirewallEdgeRule))
	mux.HandleFunc("DELETE /api/v1/firewall/edge/rules/{id}", rt.requireAbility(AbilityDeploy, rt.handleDeleteFirewallEdgeRule))
	mux.HandleFunc("POST /api/v1/firewall/edge/block", rt.requireAbility(AbilityDeploy, rt.handleFirewallIPEntry(edgefw.ActionDeny, auditFirewallIPBlocked)))
	mux.HandleFunc("POST /api/v1/firewall/edge/allow", rt.requireAbility(AbilityDeploy, rt.handleFirewallIPEntry(edgefw.ActionAllow, auditFirewallIPAllowed)))
	mux.HandleFunc("GET /api/v1/firewall/edge/bans", rt.requireAbility(AbilityReadSensitive, rt.handleListFirewallEdgeBans))
	mux.HandleFunc("DELETE /api/v1/firewall/edge/bans/{ip}", rt.requireAbility(AbilityDeploy, rt.handleLiftFirewallEdgeBan))
	mux.HandleFunc("GET /api/v1/firewall/edge/settings", rt.requireAbility(AbilityRead, rt.handleGetFirewallEdgeSettings))
	mux.HandleFunc("PUT /api/v1/firewall/edge/settings", rt.requireAbility(AbilityDeploy, rt.handleSetFirewallEdgeSettings))
	mux.HandleFunc("POST /api/v1/firewall/edge/attack-mode", rt.requireAbility(AbilityDeploy, rt.handleStartFirewallEdgeAttack))
	mux.HandleFunc("DELETE /api/v1/firewall/edge/attack-mode", rt.requireAbility(AbilityDeploy, rt.handleStopFirewallEdgeAttack))
	mux.HandleFunc("GET /api/v1/firewall/edge/audit", rt.requireAbility(AbilityRead, rt.handleFirewallEdgeAudit))
}
