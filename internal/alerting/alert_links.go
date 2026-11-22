package alerting

import (
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
)

// alertLinks derives the logs (for the firing window) and nearest deploy links from the app's alerts page link.
func alertLinks(alertsLink string, res changes.Result) (logs, deploy string) {
	base, ok := strings.CutSuffix(alertsLink, "/alerts")
	if !ok || base == "" {
		return "", ""
	}
	q := url.Values{"tab": {"search"}}
	if !res.Since.IsZero() && !res.Until.IsZero() {
		q.Set("from", res.Since.UTC().Format(time.RFC3339))
		q.Set("to", res.Until.UTC().Format(time.RFC3339))
	}
	logs = base + "/logs?" + q.Encode()
	for _, c := range res.Changes {
		if c.Kind == changes.KindDeploy || c.Kind == changes.KindRollback {
			deploy = base + "/deploys"
			break
		}
	}
	return logs, deploy
}

// linksSuffix adds the "jump to" lines to a firing message.
func linksSuffix(ev Event) string {
	if ev.Resolved || ev.LogsLink == "" {
		return ""
	}
	out := "\nLogs for this window: " + ev.LogsLink
	if ev.DeployLink != "" {
		out += "\nNearest deploy: " + ev.DeployLink
	}
	return out
}
