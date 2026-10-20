package api

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Suggestion statuses.
const (
	suggestOK     = "ok"
	suggestNarrow = "narrow"
	suggestIdle   = "idle"
)

const (
	defaultSuggestDays = 30
	maxSuggestDays     = 365
)

// tokenUsageSource is the optional audit capability least-privilege suggestions need.
type tokenUsageSource interface {
	TokenAbilityUsageSince(ctx context.Context, since time.Time) ([]store.TokenAbilityUsage, error)
}

type abilityUse struct {
	Ability    string `json:"ability"`
	Requests   int    `json:"requests"`
	LastUsedAt string `json:"last_used_at"`
}

type tokenSuggestion struct {
	TokenID            string       `json:"token_id"`
	Name               string       `json:"name"`
	AgentName          string       `json:"agent_name,omitempty"`
	Granted            []string     `json:"granted"`
	Used               []abilityUse `json:"used"`
	Unused             []string     `json:"unused"`
	SuggestedAbilities []string     `json:"suggested_abilities"`
	Status             string       `json:"status"`
	Risk               string       `json:"risk"`
	Reason             string       `json:"reason"`
}

type suggestionsResponse struct {
	WindowDays int               `json:"window_days"`
	Tokens     []tokenSuggestion `json:"tokens"`
	Note       string            `json:"note"`
}

const suggestionsNote = "Plain read requests are not audited, so the read ability is never flagged as unused."

// leastPrivilegeSuggestions compares each active token's abilities with what the audit log shows it used.
func leastPrivilegeSuggestions(tokens []store.APIToken, usage []store.TokenAbilityUsage, since, now time.Time) []tokenSuggestion {
	byToken := map[string][]abilityUse{}
	for _, u := range usage {
		byToken[u.TokenID] = append(byToken[u.TokenID], abilityUse{Ability: u.Ability, Requests: u.Requests, LastUsedAt: u.LastAt})
	}
	out := make([]tokenSuggestion, 0, len(tokens))
	for _, t := range tokens {
		if t.RevokedAt != nil || (t.ExpiresAt != nil && !t.ExpiresAt.After(now)) {
			continue
		}
		used := byToken[t.ID]
		slices.SortFunc(used, func(a, b abilityUse) int { return strings.Compare(a.Ability, b.Ability) })
		s := tokenSuggestion{
			TokenID: t.ID, Name: t.Name, AgentName: t.AgentName,
			Granted: nonNilStrings(t.Abilities), Used: nonNilAbilityUse(used),
			Unused: []string{}, Status: suggestOK, Risk: sevInfo,
		}
		usedSet := map[string]bool{}
		for _, u := range used {
			usedSet[u.Ability] = true
		}
		idle := len(used) == 0 && (t.LastUsedAt == nil || t.LastUsedAt.Before(since))
		s.SuggestedAbilities = suggestedAbilities(t.Abilities, usedSet)
		for _, a := range t.Abilities {
			if a != AbilityRead && !usedSet[a] {
				s.Unused = append(s.Unused, a)
			}
		}
		switch {
		case idle:
			s.Status, s.Risk = suggestIdle, idleRisk(t.Abilities)
			s.SuggestedAbilities = []string{}
			s.Reason = "No activity in the window. Revoke it or mint a narrower token when it is needed again."
		case len(s.Unused) > 0:
			s.Status, s.Risk = suggestNarrow, narrowRisk(s.Unused)
			s.Reason = "Granted abilities the audit log never saw this token use."
		default:
			s.Reason = "Every auditable ability granted is in use."
		}
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b tokenSuggestion) int {
		return riskRank(a.Risk) - riskRank(b.Risk)
	})
	return out
}

// suggestedAbilities keeps read and whatever was used; root is replaced by the used set unless root itself was exercised.
func suggestedAbilities(granted []string, used map[string]bool) []string {
	if slices.Contains(granted, AbilityRoot) && used[AbilityRoot] {
		return []string{AbilityRoot}
	}
	keep := map[string]bool{AbilityRead: true}
	for _, a := range granted {
		if a == AbilityRoot {
			continue
		}
		if a == AbilityRead || used[a] {
			keep[a] = true
		}
	}
	if slices.Contains(granted, AbilityRoot) {
		for a := range used {
			if a != AbilityRoot {
				keep[a] = true
			}
		}
	}
	out := make([]string, 0, len(keep))
	for _, a := range validAbilities {
		if keep[a] {
			out = append(out, a)
		}
	}
	return out
}

func idleRisk(granted []string) string {
	if slices.Contains(granted, AbilityRoot) {
		return sevHigh
	}
	return sevMedium
}

func narrowRisk(unused []string) string {
	for _, a := range unused {
		if a == AbilityRoot || a == AbilityWriteSensitive || a == AbilityReadSensitive {
			return sevHigh
		}
	}
	return sevMedium
}

func riskRank(r string) int { return slices.Index(severityOrder, r) }

func nonNilAbilityUse(in []abilityUse) []abilityUse {
	if in == nil {
		return []abilityUse{}
	}
	return in
}

// handleIAMSuggestions handles GET /api/v1/iam/suggestions?days=30. It changes nothing.
func (rt *Router) handleIAMSuggestions(w http.ResponseWriter, r *http.Request) {
	days := defaultSuggestDays
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxSuggestDays {
			writeError(w, http.StatusBadRequest, "days must be between 1 and "+strconv.Itoa(maxSuggestDays))
			return
		}
		days = n
	}
	src, ok := rt.auditLog.(tokenUsageSource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "audit usage is not available on this store")
		return
	}
	ctx := r.Context()
	now := time.Now()
	since := now.AddDate(0, 0, -days)
	tokens, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		rt.internalError(w, "api: iam suggestions: list tokens failed", err)
		return
	}
	usage, err := src.TokenAbilityUsageSince(ctx, since)
	if err != nil {
		rt.internalError(w, "api: iam suggestions: audit usage failed", err, slog.Int("days", days))
		return
	}
	writeJSON(w, http.StatusOK, suggestionsResponse{WindowDays: days, Tokens: leastPrivilegeSuggestions(tokens, usage, since, now), Note: suggestionsNote})
}
