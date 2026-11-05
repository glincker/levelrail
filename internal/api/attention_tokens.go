package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	defaultAttentionTokenWindow = 7 * 24 * time.Hour
	maxTokenNamesListed         = 5
)

// attentionTokenWindow is the live warning window, falling back to the default without settings.
func (rt *Router) attentionTokenWindow(r *http.Request) time.Duration {
	if rt.runtimeSettings == nil {
		return defaultAttentionTokenWindow
	}
	if days := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyTokensExpiryWarningDays); days > 0 {
		return time.Duration(days) * 24 * time.Hour
	}
	return defaultAttentionTokenWindow
}

// tokenAttentionItems folds token expiry into at most three rows: tokens that
// expire soon, expired tokens still in use, and the quiet expired rest.
// Platform-owned tokens rotate themselves and never appear.
func (rt *Router) tokenAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	admin := hasAbility(abilities, AbilityRoot)
	callerID, isSession := rt.currentSessionUserID(r)
	if !isSession && !admin {
		return nil
	}
	tokens, err := rt.libraryListTokens(r.Context(), callerID, admin)
	if err != nil {
		rt.logger.Warn("api: attention feed: list tokens failed", slog.String("error", err.Error()))
		return nil
	}
	window := rt.attentionTokenWindow(r)
	var expiring, inUse, idle []tokenResource
	for _, t := range tokens {
		if t.RevokedAt != nil || t.ExpiresAt == nil || (t.Scope != nil && t.Scope.TeamID == systemTokenTeamID) {
			continue
		}
		left := t.ExpiresAt.Sub(now)
		switch {
		case left > 0 && left <= window:
			expiring = append(expiring, t)
		case left <= 0 && -left <= window:
			if t.LastUsedAt != nil && now.Sub(*t.LastUsedAt) <= window {
				inUse = append(inUse, t)
			} else {
				idle = append(idle, t)
			}
		}
	}
	var items []attention.Item
	if len(expiring) > 0 {
		items = append(items, groupedTokenItem(attention.Warning, attention.KindTokenExpiring, expiring,
			"expire soon", "expires soon", "anything using them will stop working; rotate them from the token list"))
	}
	if len(inUse) > 0 {
		items = append(items, groupedTokenItem(attention.Warning, attention.KindTokenExpiredInUse, inUse,
			"expired but still in use", "expired but is still in use", "something used them just before they expired; replace them and update that caller"))
	}
	if len(idle) > 0 {
		items = append(items, groupedTokenItem(attention.Info, attention.KindTokenExpired, idle,
			"expired", "expired", "requests with them are already rejected; remove them from the token list"))
	}
	return items
}

// groupedTokenItem builds the one row for a set of tokens: names and counts only, never a token value.
func groupedTokenItem(severity, kind string, toks []tokenResource, plural, singular, detail string) attention.Item {
	names := make([]string, 0, len(toks))
	for _, t := range toks {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	shown := names
	if len(shown) > maxTokenNamesListed {
		shown = shown[:maxTokenNamesListed]
	}
	list := strings.Join(shown, ", ")
	if len(names) > len(shown) {
		list += fmt.Sprintf(" and %d more", len(names)-len(shown))
	}
	it := feedItem(severity, kind, "tokens", list+": "+detail, map[string]string{"count": fmt.Sprint(len(toks)), "names": strings.Join(names, ",")})
	if len(toks) == 1 {
		it.Title = fmt.Sprintf("API token %s %s", names[0], singular)
	} else {
		it.Title = fmt.Sprintf("%d API tokens %s", len(toks), plural)
	}
	return it
}
