package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

// upgradeAttentionItems adds one aggregate "N databases have security
// updates" item, plus one item per database past end of life or whose last
// upgrade failed.
func (rt *Router) upgradeAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if rt.dbUpgrader == nil || !hasAbility(abilities, AbilityRead) {
		return nil
	}
	sum, err := rt.upgradeSummary(r)
	if err != nil {
		rt.logger.Warn("api: attention feed: database upgrade summary failed", slog.String("error", err.Error()))
		return nil
	}
	var items []attention.Item
	var secure []string
	for _, it := range sum.Items {
		if it.Security {
			secure = append(secure, it.Database)
		}
		if it.Support == dbupgrade.SupportEOL {
			item := feedItem(attention.Warning, attention.KindDBEOL, it.Database,
				fmt.Sprintf("%s %s reached end of life on %s and gets no more security fixes", it.Engine, it.Version, it.EOL),
				map[string]string{"engine": it.Engine, "version": it.Version, "eol": it.EOL})
			item.Link = upgradesLink(it.Database)
			items = append(items, item)
		}
		if it.LastState == store.DBUpgradeStateFailed {
			item := feedItem(attention.Warning, attention.KindDBUpgradeFailed, it.Database, it.LastReason,
				map[string]string{"reason": it.LastReason})
			item.Link = upgradesLink(it.Database)
			items = append(items, item)
		}
	}
	if len(secure) > 0 {
		detail := fmt.Sprintf("%d databases have security updates available: %s", len(secure), strings.Join(secure, ", "))
		item := feedItem(attention.Warning, attention.KindDBSecurityUpdates, "databases", detail,
			map[string]string{"count": strconv.Itoa(len(secure)), "names": strings.Join(secure, ",")})
		if len(secure) == 1 {
			item.Link = upgradesLink(secure[0])
		} else {
			item.Link = "/databases"
		}
		items = append(items, item)
	}
	return items
}

func upgradesLink(name string) string { return "/databases/" + name + "/upgrades" }
