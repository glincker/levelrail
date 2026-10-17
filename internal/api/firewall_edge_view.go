package api

import (
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/edgefw"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// firewallRanges maps the range query to a window; the in-memory counters
// keep 24 hours.
var firewallRanges = map[string]time.Duration{
	"1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour,
}

func firewallWindow(r *http.Request) (time.Time, time.Time, string) {
	key := r.URL.Query().Get("range")
	d, ok := firewallRanges[key]
	if !ok {
		key, d = "1h", time.Hour
	}
	now := time.Now()
	return now.Add(-d), now, key
}

type firewallAttackResource struct {
	Active    bool   `json:"active"`
	Until     string `json:"until,omitempty"`
	Challenge bool   `json:"challenge"`
}

type firewallSuggestion struct {
	Suggested  bool `json:"suggested"`
	PerMinute  int  `json:"per_minute"`
	Threshold  int  `json:"threshold"`
	WindowMins int  `json:"window_minutes"`
}

type firewallStatusResource struct {
	Active       bool                    `json:"active"`
	Attack       firewallAttackResource  `json:"attack_mode"`
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
	Suggestion   firewallSuggestion      `json:"attack_suggestion"`
	Retention    firewallRetention       `json:"retention"`
}

type firewallRetention struct {
	ChartHours   int    `json:"chart_hours"`
	TopMinutes   int    `json:"top_minutes"`
	InMemoryOnly bool   `json:"in_memory_only"`
	TopCapacity  int    `json:"top_capacity"`
	BanDuration  string `json:"ban_duration"`
	BanStrikes   int    `json:"ban_strikes"`
	BanWindow    string `json:"ban_window"`
}

func totalsMap(t [edgefw.OutcomeCount]uint64) map[string]uint64 {
	m := make(map[string]uint64, len(t))
	for i, n := range t {
		m[edgefw.Outcome(i).String()] = n
	}
	return m
}

// attackSuggestion is deterministic: denied, challenged and rate-limited
// requests per minute, averaged over the configured window, at or above the
// configured threshold while attack mode is off.
func (rt *Router) attackSuggestion(now time.Time, st edgefw.Settings) firewallSuggestion {
	l := rt.edgeFW.engine.Limits()
	out := firewallSuggestion{Threshold: l.SuggestRate, WindowMins: l.SuggestMinutes}
	from := now.Add(-time.Duration(l.SuggestMinutes) * time.Minute)
	t := rt.edgeFW.engine.Stats.Totals("", from, now)
	bad := t[edgefw.OutcomeDenied] + t[edgefw.OutcomeChallenged] + t[edgefw.OutcomeRateLimited]
	out.PerMinute = int(bad) / l.SuggestMinutes //nolint:gosec // a per-minute counter sum, far below int range
	out.Suggested = !st.AttackActive(now) && out.PerMinute >= l.SuggestRate
	return out
}

// handleFirewallEdgeStatus handles GET /api/v1/firewall/edge/status.
func (rt *Router) handleFirewallEdgeStatus(w http.ResponseWriter, r *http.Request) {
	from, to, key := firewallWindow(r)
	scope := r.URL.Query().Get("scope")
	d := &rt.edgeFW
	rules, err := d.loadRules(r.Context())
	if err != nil {
		rt.internalError(w, "api: firewall status failed", err)
		return
	}
	enabled := 0
	for _, rule := range rules {
		if rule.Enabled {
			enabled++
		}
	}
	st := d.engine.CurrentSettings()
	res := firewallStatusResource{
		Active: true, RulesTotal: len(rules), RulesEnabled: enabled,
		BotMode: st.BotMode, AIBotMode: st.AIBotMode, BansEnabled: st.BansEnabled,
		Bans: len(d.engine.Bans.List(to)), GeoActive: ingress.DefaultGeoResolver().Status().Active, Range: key,
		Totals: totalsMap(d.engine.Stats.Totals(scope, from, to)), Reasons: d.engine.Stats.Reasons(scope, from, to),
		Suggestion: rt.attackSuggestion(to, st),
		Attack:     firewallAttackResource{Active: st.AttackActive(to), Challenge: st.AttackChallenge},
	}
	if st.AttackUntil != nil && st.AttackActive(to) {
		res.Attack.Until = st.AttackUntil.UTC().Format(time.RFC3339)
	}
	l := d.engine.Limits()
	res.Retention = firewallRetention{ChartHours: 24, TopMinutes: 60, InMemoryOnly: true, TopCapacity: l.TopCapacity,
		BanDuration: l.BanDuration.String(), BanStrikes: l.BanStrikes, BanWindow: l.BanWindow.String()}
	writeJSON(w, http.StatusOK, res)
}

type firewallSeriesPoint struct {
	Time   string            `json:"time"`
	Counts map[string]uint64 `json:"counts"`
}

// handleFirewallEdgeSeries handles GET /api/v1/firewall/edge/series.
func (rt *Router) handleFirewallEdgeSeries(w http.ResponseWriter, r *http.Request) {
	from, to, _ := firewallWindow(r)
	pts := rt.edgeFW.engine.Stats.Series(r.URL.Query().Get("scope"), from, to, 60)
	out := make([]firewallSeriesPoint, 0, len(pts))
	for _, p := range pts {
		out = append(out, firewallSeriesPoint{Time: p.Time.Format(time.RFC3339), Counts: totalsMap(p.Counts)})
	}
	writeJSON(w, http.StatusOK, out)
}

type firewallTopRow struct {
	Key       string   `json:"key"`
	Count     uint64   `json:"count"`
	Sparkline []uint32 `json:"sparkline"`
}

type firewallTrafficResource struct {
	WindowMinutes int                         `json:"window_minutes"`
	Approximate   bool                        `json:"approximate"`
	Tables        map[string][]firewallTopRow `json:"tables"`
}

// handleFirewallEdgeTraffic handles GET /api/v1/firewall/edge/traffic.
func (rt *Router) handleFirewallEdgeTraffic(w http.ResponseWriter, r *http.Request) {
	n := 10
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 50 {
		n = v
	}
	scope, now := r.URL.Query().Get("scope"), time.Now()
	res := firewallTrafficResource{WindowMinutes: 60, Approximate: true, Tables: map[string][]firewallTopRow{}}
	for _, dim := range []edgefw.Dimension{edgefw.DimIP, edgefw.DimDeniedIP, edgefw.DimUA, edgefw.DimPath, edgefw.DimHost} {
		rows := rt.edgeFW.engine.Stats.Top(scope, dim, now, n)
		out := make([]firewallTopRow, 0, len(rows))
		for _, row := range rows {
			out = append(out, firewallTopRow{Key: row.Key, Count: row.Count, Sparkline: row.Spark[:]})
		}
		res.Tables[string(dim)] = out
	}
	writeJSON(w, http.StatusOK, res)
}

type firewallBanResource struct {
	IP      string `json:"ip"`
	Reason  string `json:"reason"`
	Started string `json:"started"`
	Ends    string `json:"ends"`
	Hits    uint64 `json:"hits"`
}

// handleListFirewallEdgeBans handles GET /api/v1/firewall/edge/bans.
func (rt *Router) handleListFirewallEdgeBans(w http.ResponseWriter, _ *http.Request) {
	bans := rt.edgeFW.engine.Bans.List(time.Now())
	out := make([]firewallBanResource, 0, len(bans))
	for _, b := range bans {
		out = append(out, firewallBanResource{IP: b.IP.String(), Reason: b.Reason, Started: b.Started.UTC().Format(time.RFC3339), Ends: b.Ends.UTC().Format(time.RFC3339), Hits: b.Hits})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleLiftFirewallEdgeBan handles DELETE /api/v1/firewall/edge/bans/{ip}.
func (rt *Router) handleLiftFirewallEdgeBan(w http.ResponseWriter, r *http.Request) {
	ip, err := netip.ParseAddr(r.PathValue("ip"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "ip must be an IP address")
		return
	}
	if !rt.edgeFW.engine.Bans.Lift(ip.Unmap()) {
		writeError(w, http.StatusNotFound, "no active ban for that address")
		return
	}
	rt.auditFirewall(r, auditFirewallBanLifted, map[string]string{"ip": ip.String()})
	w.WriteHeader(http.StatusNoContent)
}

// handleFirewallEdgeAudit handles GET /api/v1/firewall/edge/audit: only
// the firewall's own change events, newest first.
func (rt *Router) handleFirewallEdgeAudit(w http.ResponseWriter, r *http.Request) {
	limit, before, _, ok := parseAuditLogQuery(w, r)
	if !ok {
		return
	}
	filter := store.AuditEntryFilter{ActionPrefixes: []string{AuditActionFirewallPrefix}}
	entries, err := rt.auditLog.ListAuditEntries(r.Context(), limit, before, filter)
	if err != nil {
		rt.internalError(w, "api: firewall audit failed", err)
		return
	}
	out := make([]auditLogEntryResource, len(entries))
	for i, e := range entries {
		out[i] = toAuditLogEntryResource(e)
	}
	writeJSON(w, http.StatusOK, out)
}

// firewallAttentionItems raises one item while the deterministic spike
// threshold is crossed and attack mode is off.
func (rt *Router) firewallAttentionItems(abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityDeploy) {
		return nil
	}
	now := time.Now()
	s := rt.attackSuggestion(now, rt.edgeFW.engine.CurrentSettings())
	if !s.Suggested {
		return nil
	}
	detail := strconv.Itoa(s.PerMinute) + " blocked or challenged requests per minute over " + strconv.Itoa(s.WindowMins) + " minutes"
	return []attention.Item{feedItem(attention.Warning, attention.KindFirewallAttackSuggested, "instance", detail,
		map[string]string{"per_minute": strconv.Itoa(s.PerMinute), "threshold": strconv.Itoa(s.Threshold)})}
}
