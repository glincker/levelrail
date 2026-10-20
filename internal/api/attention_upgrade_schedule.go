package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

const upgradeAttentionRecent = 3 * 24 * time.Hour

// upgradeScheduleAttentionItems raises a scheduled upgrade that is held at
// fire time, or that recently failed or was missed, so a missed maintenance
// window is never silent.
func (rt *Router) upgradeScheduleAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.upgradeSchedule == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	list, err := rt.upgradeSchedule.store.ListUpgradeSchedules(r.Context(), 5)
	if err != nil {
		rt.logger.Warn("api: attention feed: list upgrade schedules failed", slog.String("error", err.Error()))
		return nil
	}
	var items []attention.Item
	now := time.Now()
	for i, sc := range list {
		held := sc.State == store.UpgradeScheduleBlocked
		recentBad := false
		if (sc.State == store.UpgradeScheduleFailed || sc.State == store.UpgradeScheduleMissed) && i == 0 {
			if t, err := time.Parse(time.RFC3339, sc.FinishedAt); err == nil && now.Sub(t) < upgradeAttentionRecent {
				recentBad = true
			}
		}
		if !held && !recentBad {
			continue
		}
		items = append(items, feedItem(attention.Warning, attention.KindUpgradeBlocked, sc.TargetVersion,
			"scheduled upgrade to "+sc.TargetVersion+" did not run: "+sc.LastReason,
			map[string]string{"target": sc.TargetVersion, "state": sc.State, "reason": sc.LastReason, "schedule": sc.ID}))
	}
	return items
}
