package api

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
)

// advisorLink is the dashboard page a finding is acted on from.
func advisorLink(ref resourceadvisor.Ref) string {
	switch ref.Kind {
	case resourceadvisor.ResourceApp:
		return "/apps/" + ref.Name + "/resources"
	case resourceadvisor.ResourceDatabase:
		return "/databases/" + ref.Name + "/resources"
	}
	return "/resource-advisor"
}

// resourceAdviceAttentionItems adds advisor findings to the attention feed
// as recommendations: one item per warning, one roll-up for the rest. It
// reads only the cache so the feed never waits on an analysis.
func (rt *Router) resourceAdviceAttentionItems(r *http.Request) []attention.Item {
	if !experimental.Enabled(experimental.ResourceAdvisor) {
		return nil
	}
	cached := rt.advisorFindingsCached()
	if len(cached) == 0 {
		return nil
	}
	visible, err := rt.visibleAdvisorFindings(r, cached, false)
	if err != nil {
		rt.logger.Warn("api: attention feed: resource advice", slog.String("error", err.Error()))
		return nil
	}
	var items []attention.Item
	suggestions := 0
	for _, f := range visible {
		switch f.Severity {
		case resourceadvisor.SeverityWarn:
			it := feedItem(attention.Warning, attention.KindResourceAdvice, f.Resource.Name, f.Why, map[string]string{"finding": string(f.Kind), "title": f.Title})
			it.ID += ":" + f.ID
			it.Title, it.Link, it.Class = f.Title, advisorLink(f.Resource), attention.ClassRecommendation
			items = append(items, it)
		case resourceadvisor.SeveritySuggest:
			suggestions++
		}
	}
	if suggestions > 0 {
		it := feedItem(attention.Info, attention.KindResourceAdvice, "suggestions", fmt.Sprintf("%d suggestions to right-size limits, sleep idle apps or tune databases", suggestions), map[string]string{"count": fmt.Sprint(suggestions)})
		it.Title, it.Class = fmt.Sprintf("%d resource suggestions", suggestions), attention.ClassRecommendation
		items = append(items, it)
	}
	return items
}
