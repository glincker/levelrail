package announcements

import (
	"encoding/json"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Viewer describes who is asking and which build they run.
type Viewer struct {
	IsRoot  bool
	CanRead bool
	Version string
	Channel string
	Mode    string
	// Newsletter is the updates.announcements_newsletter setting.
	Newsletter bool
	Now        time.Time
}

// IsSecurity reports whether a tier is a security notice, visible to root only.
func IsSecurity(tier string) bool {
	return tier == TierCritical || tier == TierAdvisory
}

// Decode parses a cached row back into an Item.
func Decode(r store.AnnouncementRow) (Item, error) {
	var it Item
	err := json.Unmarshal([]byte(r.PayloadJSON), &it)
	return it, err
}

// Visible filters cached items for one viewer: the mode, audience ability,
// expiry, release channel and the applies_to range against the running version.
func Visible(rows []store.AnnouncementRow, v Viewer) []Item {
	var out []Item
	for _, r := range rows {
		it, err := Decode(r)
		if err != nil || it.Kind != KindAnnouncement || !it.ExpiresAt.After(v.Now) {
			continue
		}
		if !audienceAllows(it, v) || !targets(it, v) {
			continue
		}
		out = append(out, it)
	}
	return out
}

func audienceAllows(it Item, v Viewer) bool {
	if v.Mode == ModeOff || v.Mode == "" {
		return false
	}
	if IsSecurity(it.Tier) {
		return v.IsRoot
	}
	if v.Mode != ModeAll || !v.CanRead {
		return false
	}
	return it.Tier != TierNewsletter || v.Newsletter
}

func targets(it Item, v Viewer) bool {
	if len(it.Channels) > 0 {
		match := false
		for _, c := range it.Channels {
			match = match || c == v.Channel
		}
		if !match {
			return false
		}
	}
	if it.AppliesTo == "" {
		return true
	}
	r, err := ParseRange(it.AppliesTo)
	return err == nil && r.Match(v.Version)
}
