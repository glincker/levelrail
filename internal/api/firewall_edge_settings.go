package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/edgefw"
)

type firewallSettingsResource struct {
	edgefw.Settings
	DefaultAIBotAgents []string `json:"default_ai_bot_agents"`
	MaxAIBotAgents     int      `json:"max_ai_bot_agents"`
}

func (rt *Router) settingsResource() firewallSettingsResource {
	return firewallSettingsResource{
		Settings: rt.edgeFW.engine.CurrentSettings(), DefaultAIBotAgents: edgefw.DefaultAIBotAgents(),
		MaxAIBotAgents: rt.edgeFW.engine.Limits().MaxBotAgents,
	}
}

// handleGetFirewallEdgeSettings handles GET /api/v1/firewall/edge/settings.
func (rt *Router) handleGetFirewallEdgeSettings(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, rt.settingsResource())
}

type firewallSettingsRequest struct {
	BotMode     edgefw.Mode `json:"bot_mode"`
	AIBotMode   edgefw.Mode `json:"ai_bot_mode"`
	AIBotAgents []string    `json:"ai_bot_agents"`
	BansEnabled bool        `json:"bans_enabled"`
}

func (rt *Router) saveSettings(w http.ResponseWriter, r *http.Request, s edgefw.Settings) bool {
	l := rt.domainPolicy.limits()
	if err := s.Validate(rt.edgeFW.engine.Limits(), l.ShieldAttackMax, time.Now()); err != nil {
		writeFirewallValidation(w, err)
		return false
	}
	raw, err := json.Marshal(s)
	if err != nil {
		rt.internalError(w, "api: encode firewall settings failed", err)
		return false
	}
	if err := rt.edgeFW.store.SetEdgeFirewallSettings(r.Context(), string(raw)); err != nil {
		rt.internalError(w, "api: save firewall settings failed", err)
		return false
	}
	rt.edgeFW.engine.SetSettings(s)
	return true
}

// handleSetFirewallEdgeSettings handles PUT /api/v1/firewall/edge/settings.
func (rt *Router) handleSetFirewallEdgeSettings(w http.ResponseWriter, r *http.Request) {
	var req firewallSettingsRequest
	if err := decodeJSONBodyLimit(r, &req, maxFirewallBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.engine.CurrentSettings()
	s.BotMode, s.AIBotMode, s.AIBotAgents, s.BansEnabled = req.BotMode, req.AIBotMode, req.AIBotAgents, req.BansEnabled
	if !rt.saveSettings(w, r, s) {
		return
	}
	rt.auditFirewall(r, auditFirewallSettings, map[string]string{"bot_mode": string(s.BotMode), "ai_bot_mode": string(s.AIBotMode), "bans_enabled": strconv.FormatBool(s.BansEnabled)})
	writeJSON(w, http.StatusOK, rt.settingsResource())
}

type firewallAttackRequest struct {
	Minutes   int  `json:"minutes,omitempty"`
	Challenge bool `json:"challenge"`
}

// handleStartFirewallEdgeAttack handles POST /api/v1/firewall/edge/attack-mode.
func (rt *Router) handleStartFirewallEdgeAttack(w http.ResponseWriter, r *http.Request) {
	var req firewallAttackRequest
	if err := decodeJSONBodyLimit(r, &req, maxFirewallBody); err != nil && !errors.Is(err, io.EOF) {
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
	rt.setAttack(w, r, ptrTime(time.Now().UTC().Add(d)), req.Challenge, auditFirewallAttackOn)
}

// handleStopFirewallEdgeAttack handles DELETE /api/v1/firewall/edge/attack-mode.
func (rt *Router) handleStopFirewallEdgeAttack(w http.ResponseWriter, r *http.Request) {
	rt.setAttack(w, r, nil, false, auditFirewallAttackOff)
}

func ptrTime(t time.Time) *time.Time { return &t }

func (rt *Router) setAttack(w http.ResponseWriter, r *http.Request, until *time.Time, challenge bool, audit string) {
	d := &rt.edgeFW
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.engine.CurrentSettings()
	s.AttackUntil, s.AttackChallenge = until, challenge
	if !rt.saveSettings(w, r, s) {
		return
	}
	detail := map[string]string{"challenge": strconv.FormatBool(challenge)}
	if until != nil {
		detail["until"] = until.Format(time.RFC3339)
	}
	rt.auditFirewall(r, audit, detail)
	writeJSON(w, http.StatusOK, firewallAttackResource{Active: s.AttackActive(time.Now()), Challenge: s.AttackChallenge, Until: untilString(s.AttackUntil)})
}

func untilString(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
