package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/firewall"
	firewallreconcile "github.com/GLINCKER/levelrail/internal/reconcile/firewall"
)

// firewallRuleResource is one managed port's wire shape, shared by both
// endpoints below: GET reports it via Report (read-only), POST reports
// it via Sync (after applying whatever change was needed).
type firewallRuleResource struct {
	Port  int    `json:"port"`
	Proto string `json:"proto"`
	Owner string `json:"owner"`
	Open  bool   `json:"open"`
}

// firewallStatusResponse is GET /api/v1/system/firewall's response body.
type firewallStatusResponse struct {
	// Installed and Active mirror firewall.Result: an operator not
	// running ufw, or running it but not enabled, both mean no port
	// below can be meaningfully reported as open (see Rules' own Open
	// field doc comment on firewall.RuleStatus).
	Installed bool                   `json:"installed"`
	Active    bool                   `json:"active"`
	Rules     []firewallRuleResource `json:"rules"`
	// Extra is every rule this platform previously opened that no
	// longer corresponds to any exposed app/database port: present on
	// GET so an operator can see drift before the next reconcile (or a
	// manual POST .../sync) cleans it up.
	Extra []firewallRuleResource `json:"extra,omitempty"`
}

// firewallSyncResponse is POST /api/v1/system/firewall/sync's response
// body: firewallStatusResponse plus what the sync itself just did.
type firewallSyncResponse struct {
	firewallStatusResponse
	Applied int      `json:"applied"`
	Removed int      `json:"removed"`
	Errors  []string `json:"errors,omitempty"`
}

func toFirewallRuleResources(statuses []firewall.RuleStatus) []firewallRuleResource {
	out := make([]firewallRuleResource, len(statuses))
	for i, s := range statuses {
		proto := s.Proto
		if proto == "" {
			proto = "tcp"
		}
		out[i] = firewallRuleResource{Port: s.Port, Proto: proto, Owner: s.Owner, Open: s.Open}
	}
	return out
}

// firewallWantedRules lists every desired service and database and
// derives the same want list internal/reconcile/firewall's own
// background controller reconciles toward (firewallreconcile.WantedRules),
// so this endpoint never carries a second, possibly-diverging copy of
// that filtering logic.
func (rt *Router) firewallWantedRules(ctx context.Context) ([]firewall.Rule, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	databases, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	want, _ := firewallreconcile.WantedRules(ctx, rt.logger, services, databases)
	return want, nil
}

// handleGetFirewallStatus handles GET /api/v1/system/firewall: a
// read-only report of every managed port (every app's HostPort, every
// database's public access port, for resources on this control plane's
// own node, see internal/reconcile/firewall's package doc comment for
// the v1 node-scope limit) and whether ufw currently allows it. Never
// mutates anything, the same read-only shape GET /api/v1/system/doctor's
// firewall check already has; this endpoint is specifically about the
// per-port rules this platform itself manages, not ufw's general
// installed/active/default-policy posture (that's still doctor's job).
//
// 501 if no firewall manager is configured (WithFirewallManager),
// matching every other optional-dependency route in this package.
func (rt *Router) handleGetFirewallStatus(w http.ResponseWriter, r *http.Request) {
	if rt.firewallManager == nil {
		writeError(w, http.StatusNotImplemented, "firewall management is not configured on this control plane")
		return
	}

	want, err := rt.firewallWantedRules(r.Context())
	if err != nil {
		rt.logger.Error("api: get firewall status: list desired resources failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result, err := rt.firewallManager.Report(r.Context(), want)
	if err != nil {
		rt.logger.Error("api: get firewall status failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, firewallStatusResponse{
		Installed: result.Installed,
		Active:    result.Active,
		Rules:     toFirewallRuleResources(result.Managed),
		Extra:     toFirewallRuleResources(result.Extra),
	})
}

// handleSyncFirewall handles POST /api/v1/system/firewall/sync: applies
// right now the same convergence internal/reconcile/firewall's own
// background reconcile loop already performs on its own schedule, for
// an operator who wants to confirm a just-exposed port took effect
// immediately rather than waiting for the next tick. AbilityRoot
// (routes.go), the same fleet-wide-blast-radius tier as
// POST /system/prune and POST /system/master-key/rotate: this changes
// the actual host firewall, not one app's own resources.
func (rt *Router) handleSyncFirewall(w http.ResponseWriter, r *http.Request) {
	if rt.firewallManager == nil {
		writeError(w, http.StatusNotImplemented, "firewall management is not configured on this control plane")
		return
	}

	want, err := rt.firewallWantedRules(r.Context())
	if err != nil {
		rt.logger.Error("api: sync firewall: list desired resources failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	result, err := rt.firewallManager.Sync(r.Context(), want)
	if err != nil {
		rt.logger.Error("api: sync firewall failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, firewallSyncResponse{
		firewallStatusResponse: firewallStatusResponse{
			Installed: result.Installed,
			Active:    result.Active,
			Rules:     toFirewallRuleResources(result.Managed),
			Extra:     toFirewallRuleResources(result.Extra),
		},
		Applied: result.Applied,
		Removed: result.Removed,
		Errors:  result.Errors,
	})
}
