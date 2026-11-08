package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const bulkMuteMaxRules = 200

type bulkMuteRequest struct {
	RuleIDs  []string `json:"rule_ids"`
	Duration string   `json:"duration"`
	Reason   string   `json:"reason"`
}

type bulkMuteItem struct {
	RuleID    string `json:"rule_id"`
	SilenceID string `json:"silence_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// handleBulkMuteAlertRules handles POST /api/v1/alert-rules/bulk-mute: one
// silence per rule so each can be lifted alone, one audit row per rule.
func (rt *Router) handleBulkMuteAlertRules(w http.ResponseWriter, r *http.Request) {
	if rt.alertRules == nil || !rt.alertNoiseReady(w) {
		return
	}
	var req bulkMuteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.RuleIDs) == 0 || len(req.RuleIDs) > bulkMuteMaxRules {
		writeError(w, http.StatusBadRequest, "rule_ids must hold between 1 and 200 ids")
		return
	}
	now := time.Now()
	actor := rt.pipelineActor(r)
	seen := make(map[string]bool, len(req.RuleIDs))
	items := make([]bulkMuteItem, 0, len(req.RuleIDs))
	for _, id := range req.RuleIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		item := bulkMuteItem{RuleID: id}
		rule, err := rt.alertRules.GetRule(r.Context(), id)
		switch {
		case errors.Is(err, alerting.ErrRuleNotFound):
			item.Error = "alert rule not found"
		case err != nil:
			rt.logger.Warn("api: bulk mute: load rule failed", slog.String("error", err.Error()))
			item.Error = "could not load the rule"
		default:
			s, berr := buildSilence(createSilenceRequest{
				Matchers: alerting.SilenceMatcher{RuleIDs: []string{rule.ID}},
				Duration: req.Duration,
				Reason:   req.Reason,
			}, actor, now)
			if berr != nil {
				writeError(w, http.StatusBadRequest, berr.Error())
				return
			}
			if cerr := rt.alertNoise.CreateSilence(r.Context(), s); cerr != nil {
				rt.logger.Warn("api: bulk mute: create silence failed", slog.String("error", cerr.Error()))
				item.Error = "could not mute the rule"
			} else {
				item.SilenceID = s.ID
				rt.auditAlertRuleMute(r.Context(), r, rule.ID)
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (rt *Router) auditAlertRuleMute(ctx context.Context, r *http.Request, ruleID string) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: alert mute audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/alert-rules/" + ruleID + "/mute",
		StatusCode: http.StatusCreated, RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: alert mute audit save failed", slog.String("error", err.Error()), slog.String("rule", ruleID))
	}
}
