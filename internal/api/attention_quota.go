package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

// quotaAttentionItems raises one item per team or project dimension at or
// above the warning percentage. Only scopes with an explicit row or the
// default team are swept, so an unlimited instance costs two queries.
func (rt *Router) quotaAttentionItems(r *http.Request, abilities []string) []attention.Item {
	db := rt.quotaDB()
	if db == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	ctx := r.Context()
	scopes := map[string][]string{quota.ScopeTeam: {store.DefaultTeamID}}
	for _, scope := range []string{quota.ScopeTeam, quota.ScopeProject} {
		ids, err := db.QuotaScopeIDs(ctx, scope)
		if err != nil {
			rt.logger.Warn("api: attention feed: list quota scopes failed", slog.String("error", err.Error()))
			return nil
		}
		scopes[scope] = append(scopes[scope], ids...)
	}
	warn := quotaWarnPercent()
	var items []attention.Item
	seen := map[string]bool{}
	for scope, ids := range scopes {
		for _, id := range ids {
			if seen[scope+id] {
				continue
			}
			seen[scope+id] = true
			limits, err := effectiveQuota(ctx, db, scope, id)
			if err != nil || limits.Unlimited() {
				continue
			}
			used, err := db.QuotaUsage(ctx, scope, id)
			if err != nil {
				continue
			}
			name := rt.quotaScopeName(ctx, scope, id)
			for _, res := range quota.Resources {
				pct := quota.Percent(used.Get(res), limits.Get(res))
				if pct < warn {
					continue
				}
				severity := attention.Warning
				if pct >= 100 {
					severity = attention.Critical
				}
				detail := fmt.Sprintf("%s of %s used", formatQuotaAmount(res, used.Get(res)), formatQuotaAmount(res, limits.Get(res)))
				it := feedItem(severity, attention.KindQuotaNearLimit, scope+"/"+id+"/"+res, detail,
					map[string]string{"scope": scope, "scope_id": id, "name": name, "resource": res, "percent": fmt.Sprint(pct)})
				it.Title = fmt.Sprintf("%s %q is at %d%% of its %s quota", scope, name, pct, res)
				it.Link = quotaLink(scope, id)
				items = append(items, it)
			}
		}
	}
	return items
}

func quotaLink(scope, id string) string {
	if scope == quota.ScopeProject {
		return "/projects/" + id
	}
	return "/settings/teams"
}
