package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

type shieldLimitsResource struct {
	MaxBotAgents           int `json:"max_bot_agents"`
	MaxRetryAfterSeconds   int `json:"max_retry_after_seconds"`
	DefaultRetryAfter      int `json:"default_retry_after_seconds"`
	TarpitMaxMillis        int `json:"tarpit_max_ms"`
	AttackRatePerMinute    int `json:"attack_rate_per_minute"`
	AttackBurst            int `json:"attack_burst"`
	AttackMaxConcurrent    int `json:"attack_max_concurrent"`
	AttackDefaultMinutes   int `json:"attack_default_minutes"`
	AttackMaxMinutes       int `json:"attack_max_minutes"`
	MaxTrackedIPsPerDomain int `json:"max_tracked_ips"`
}

func newShieldLimits(l trafficpolicy.Limits) shieldLimitsResource {
	return shieldLimitsResource{
		MaxBotAgents: l.ShieldMaxBotAgents, MaxRetryAfterSeconds: l.ShieldMaxRetryAfter, DefaultRetryAfter: l.ShieldRetryAfter,
		TarpitMaxMillis: int(l.ShieldTarpitMax.Milliseconds()), AttackRatePerMinute: l.ShieldAttackRate,
		AttackBurst: l.ShieldAttackBurst, AttackMaxConcurrent: l.ShieldAttackConcurrent,
		AttackDefaultMinutes: int(l.ShieldAttackDefault.Minutes()), AttackMaxMinutes: int(l.ShieldAttackMax.Minutes()),
		MaxTrackedIPsPerDomain: l.ShieldMaxTrackedIPs,
	}
}

type underAttackRequest struct {
	Minutes int `json:"minutes,omitempty"`
}

type shieldBotListResource struct {
	Agents []string `json:"agents"`
	Custom bool     `json:"custom"`
}

// handleShieldBotList handles GET /api/v1/system/shield/bots: the denylist
// every shield-enabled domain starts from.
func (rt *Router) handleShieldBotList(w http.ResponseWriter, _ *http.Request) {
	l := rt.domainPolicy.limits()
	list, err := trafficpolicy.BotAgentsFor(l)
	if err != nil {
		rt.internalError(w, "api: shield bot list failed", err)
		return
	}
	writeJSON(w, http.StatusOK, shieldBotListResource{Agents: list, Custom: l.ShieldBotListFile != ""})
}

func (rt *Router) loadShield(r *http.Request, domain string) (trafficpolicy.Shield, bool, error) {
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield)
	if err != nil || !found {
		return trafficpolicy.Shield{}, false, err
	}
	var s trafficpolicy.Shield
	if err := json.Unmarshal([]byte(row.Spec), &s); err != nil {
		return trafficpolicy.Shield{}, true, err
	}
	return s, true, nil
}

func (rt *Router) storeShield(w http.ResponseWriter, r *http.Request, domain string, s trafficpolicy.Shield) {
	if err := s.Validate(rt.domainPolicy.limits()); err != nil {
		writePolicyError(w, err)
		return
	}
	raw, err := json.Marshal(s)
	if err != nil {
		rt.internalError(w, "api: encode shield failed", err, slog.String("domain", domain))
		return
	}
	if err := rt.domainPolicy.store.SetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield, raw); err != nil {
		rt.internalError(w, "api: set shield failed", err, slog.String("domain", domain))
		return
	}
	rt.nudgeReconciler()
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield)
	if err != nil {
		rt.internalError(w, "api: get shield failed", err, slog.String("domain", domain))
		return
	}
	writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, trafficpolicy.KindShield, row, found))
}

// handleStartUnderAttack handles POST .../shield/under-attack: tightens the
// domain's limits for a bounded time. It works with no shield configured.
func (rt *Router) handleStartUnderAttack(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	var req underAttackRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPolicyBody)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	l := rt.domainPolicy.limits()
	d := l.ShieldAttackDefault
	if req.Minutes < 0 {
		writeError(w, http.StatusBadRequest, "minutes must not be negative")
		return
	}
	if req.Minutes > 0 {
		d = time.Duration(req.Minutes) * time.Minute
	}
	if d > l.ShieldAttackMax {
		writeError(w, http.StatusBadRequest, "minutes must be at most "+strconv.Itoa(int(l.ShieldAttackMax.Minutes())))
		return
	}
	s, _, err := rt.loadShield(r, domain)
	if err != nil {
		rt.internalError(w, "api: load shield failed", err, slog.String("domain", domain))
		return
	}
	until := time.Now().UTC().Add(d)
	s.UnderAttackUntil = &until
	rt.storeShield(w, r, domain, s)
}

// handleStopUnderAttack handles DELETE .../shield/under-attack.
func (rt *Router) handleStopUnderAttack(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	s, found, err := rt.loadShield(r, domain)
	if err != nil {
		rt.internalError(w, "api: load shield failed", err, slog.String("domain", domain))
		return
	}
	if !found {
		writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, trafficpolicy.KindShield, store.DomainTrafficPolicy{}, false))
		return
	}
	s.UnderAttackUntil = nil
	if !s.Enabled {
		if err := rt.domainPolicy.store.DeleteDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield); err != nil {
			rt.internalError(w, "api: delete shield failed", err, slog.String("domain", domain))
			return
		}
		rt.nudgeReconciler()
		writeJSON(w, http.StatusOK, toDomainPolicyResource(domain, trafficpolicy.KindShield, store.DomainTrafficPolicy{}, false))
		return
	}
	rt.storeShield(w, r, domain, s)
}
