package api

import (
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/secretrotation"
	"github.com/GLINCKER/levelrail/internal/store"
)

// secretRotationAttentionItems raises overdue rotations and rotations that
// are stuck half-done, so a changed value with a failed restart is never silent.
func (rt *Router) secretRotationAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.rotation.Load() == nil || rt.secrets == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	policies, err := rt.rotation.Load().store.ListSecretRotationPolicies(r.Context(), "")
	if err != nil {
		rt.logger.Warn("api: attention feed: list secret rotation policies failed", slog.String("error", err.Error()))
		return nil
	}
	var items []attention.Item
	for _, res := range rt.rotationResources(r.Context(), policies, true) {
		subject := res.App + "/" + res.Key
		params := map[string]string{"app": res.App, "key": res.Key, "state": res.State, "status": res.Status}
		var it attention.Item
		switch {
		case res.State == store.SecretRotationRestartFailed || res.State == store.SecretRotationFailed:
			it = feedItem(attention.Warning, attention.KindSecretRotationFail, subject, res.LastError, params)
		case res.Status == secretrotation.StatusOverdue:
			it = feedItem(attention.Warning, attention.KindSecretRotationDue, subject, "owner: "+res.Owner, params)
		default:
			continue
		}
		it.Link = "/apps/" + res.App + "/environment"
		items = append(items, it)
	}
	return items
}
