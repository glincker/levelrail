package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// unusedTokenResource is the list shape for an idle token: metadata only, never the secret.
type unusedTokenResource struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Abilities  []string   `json:"abilities"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type unusedTokensResponse struct {
	// AlertDays is the tokens.unused_alert_days setting; 0 means the alert is off and Items is empty.
	AlertDays int                   `json:"alert_days"`
	Items     []unusedTokenResource `json:"items"`
}

// sensitiveTokenAbilities are the grants that can change or reveal something.
var sensitiveTokenAbilities = []string{AbilityWrite, AbilityWriteSensitive, AbilityReadSensitive, AbilityDeploy, AbilityRoot}

func holdsSensitiveAbility(abilities []string) bool {
	return slices.ContainsFunc(abilities, func(a string) bool { return slices.Contains(sensitiveTokenAbilities, a) })
}

// tokenLive is true for a token that is neither revoked nor expired.
func tokenLive(t store.APIToken, now time.Time) bool {
	return t.RevokedAt == nil && (t.ExpiresAt == nil || t.ExpiresAt.After(now))
}

// unusedTokens returns live tokens idle for at least days; a never-used token
// counts from its creation. days <= 0 means the alert is off.
func unusedTokens(tokens []store.APIToken, days int, now time.Time) []unusedTokenResource {
	out := []unusedTokenResource{}
	if days <= 0 {
		return out
	}
	cutoff := now.Add(-time.Duration(days) * hygieneDay)
	for _, t := range tokens {
		if !tokenLive(t, now) || tokenLastActive(t).After(cutoff) {
			continue
		}
		out = append(out, unusedTokenResource{ID: t.ID, Name: t.Name, Abilities: nonNilStrings(t.Abilities), CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt})
	}
	return out
}

// liveTokenRecords lists stored tokens; an empty ownerID lists every owner.
func (rt *Router) liveTokenRecords(ctx context.Context, ownerID string) ([]store.APIToken, error) {
	if rt.authLib.tokens == nil {
		return nil, nil
	}
	recs, err := rt.authLib.tokens.ListTokens(ctx, ownerID)
	if err != nil {
		return nil, fmt.Errorf("api: list tokens: %w", err)
	}
	out := make([]store.APIToken, 0, len(recs))
	for _, rec := range recs {
		out = append(out, *libraryTokenToStore(rec))
	}
	return out, nil
}

// handleUnusedTokens handles GET /api/v1/tokens/unused: the caller's own idle
// tokens, or every one for an admin. Read only, nothing is revoked.
func (rt *Router) handleUnusedTokens(w http.ResponseWriter, r *http.Request) {
	owner, _ := rt.currentSessionUserID(r)
	if rt.callerIsAdmin(r) {
		owner = ""
	}
	recs, err := rt.liveTokenRecords(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: unused tokens failed", err)
		return
	}
	days := rt.tokenUnusedAlertDays(r.Context())
	writeJSON(w, http.StatusOK, unusedTokensResponse{AlertDays: days, Items: unusedTokens(recs, days, time.Now())})
}
