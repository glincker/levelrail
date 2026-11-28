package api

import (
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/announcements"
	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// Attention kinds for the announcement feed. They are local to this source
// so the shared attention kind table stays untouched.
const (
	attentionKindAnnouncement      = "announcement"
	attentionKindAnnouncementOptIn = "announcements_optin"
	announcementOptInID            = "announcements:optin"
	announcementPreselect          = announcements.ModeSecurity
)

// announcementAttentionItems feeds the attention list: a local opt-in prompt
// for root until the setting is answered (no network is touched), then the
// non-critical, non-newsletter notices the caller has not dismissed or snoozed.
// Critical notices use the banner endpoint instead.
func (rt *Router) announcementAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	if hasAbilityExactRoot(abilities) {
		if it, ok := rt.announcementOptInItem(r); ok {
			return []attention.Item{it}
		}
	}
	views, _ := rt.announcementViews(r, abilities, now)
	var out []attention.Item
	for _, v := range views {
		if v.Tier == announcements.TierCritical || v.Tier == announcements.TierNewsletter || v.Dismissed || v.SnoozedUntil != nil {
			continue
		}
		sev := attention.Info
		if v.Tier == announcements.TierAdvisory {
			sev = attention.Warning
		}
		it := attention.NewItem(sev, attentionKindAnnouncement, v.ID, v.Body)
		it.ID = announcementItemKeyPrefix + v.ID
		it.Title = v.Title
		it.Link = announcementLink(v.CTA)
		it.Class = attention.ClassRecommendation
		it.Params = map[string]string{"tier": v.Tier}
		out = append(out, it)
	}
	return out
}

func announcementLink(cta string) string {
	if cta == announcements.CTASettings {
		return "/settings"
	}
	return "/settings/updates"
}

// announcementOptInItem asks root once whether to fetch the feed; the setting
// counts as answered when it was saved or set from the environment.
func (rt *Router) announcementOptInItem(r *http.Request) (attention.Item, bool) {
	if rt.runtimeSettings == nil {
		return attention.Item{}, false
	}
	st, err := rt.runtimeSettings.Get(r.Context(), runtimesettings.KeyAnnouncements)
	if err != nil || st.Source != runtimesettings.SourceDefault {
		return attention.Item{}, false
	}
	it := attention.NewItem(attention.Info, attentionKindAnnouncementOptIn, "announcements",
		"Fetches one public file from GitHub on a schedule and sends nothing about this instance.")
	it.ID = announcementOptInID
	it.Title = "Get security notices from the Levelrail feed?"
	it.Action = "Choose security notices, all announcements or off"
	it.Link = "/settings/updates"
	it.Class = attention.ClassRecommendation
	it.Params = map[string]string{"preselect": announcementPreselect, "choices": "off,security,all"}
	return it, true
}
