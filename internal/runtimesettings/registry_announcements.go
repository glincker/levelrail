package runtimesettings

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Keys for the signed announcement feed. The feature fetches nothing while
// updates.announcements is off, which is the default until root answers the
// local opt-in prompt.
const (
	KeyAnnouncements           = "updates.announcements"
	KeyAnnouncementsURL        = "updates.announcements_feed_url"
	KeyAnnouncementsInterval   = "updates.announcements_interval"
	KeyAnnouncementsJitter     = "updates.announcements_jitter"
	KeyAnnouncementsNewsletter = "updates.announcements_newsletter"
	KeyAnnouncementsMaxBytes   = "updates.announcements_max_bytes"
	KeyAnnouncementsMaxItems   = "updates.announcements_max_items"
	KeyAnnouncementsMaxTitle   = "updates.announcements_max_title"
	KeyAnnouncementsMaxBody    = "updates.announcements_max_body"
	KeyAnnouncementsSnoozeMax  = "updates.announcements_snooze_max"
	KeyAnnouncementsClockSkew  = "updates.announcements_clock_skew"
	KeyAnnouncementsMaxAge     = "updates.announcements_feed_max_age"
	KeyAnnouncementsDevKeys    = "updates.announcements_dev_keys"

	// DefaultAnnouncementsFeedURL is the public feed on the dedicated feed branch.
	DefaultAnnouncementsFeedURL = "https://raw.githubusercontent.com/glincker/levelrail/feed/feed.v1.json"
)

func init() {
	Register(Definition{
		Key: KeyAnnouncements, Category: CategoryDeploys, Kind: KindString, Default: "off",
		Env: "APP_ANNOUNCEMENTS", Apply: ApplyLive, AllowedStrings: []string{"off", "security", "all"},
		Title:       "Announcement feed",
		Description: "Which signed announcements to fetch: off, security (security notices only) or all. The request is one public file with no instance identifiers. Off fetches nothing.",
	})
	Register(Definition{
		Key: KeyAnnouncementsURL, Category: CategoryDeploys, Kind: KindString, Default: DefaultAnnouncementsFeedURL,
		Env: "APP_ANNOUNCEMENTS_URL", Apply: ApplyLive, CheckValue: announcementsURLCheck,
		Title:       "Announcement feed URL",
		Description: "An https URL serving the signed feed file, with its signature at the same URL plus .sig. A mirror cannot alter content, because every item is verified against the keys built into this release.",
	})
	Register(Definition{
		Key: KeyAnnouncementsInterval, Category: CategoryDeploys, Kind: KindDuration, Default: "6h",
		Env: "APP_ANNOUNCEMENTS_INTERVAL", Apply: ApplyLive, CheckValue: durationRangeCheck(15*time.Minute, 7*24*time.Hour),
		Title:       "Announcement check interval",
		Description: "How often the feed is checked, from 15m to 168h. Failures back off up to this value.",
	})
	Register(Definition{
		Key: KeyAnnouncementsJitter, Category: CategoryDeploys, Kind: KindInt, Default: "20",
		Env: "APP_ANNOUNCEMENTS_JITTER", Apply: ApplyLive, CheckValue: intRangeCheck(0, 100),
		Title:       "Announcement check jitter",
		Description: "Random extra wait, as a percent of the interval, so instances do not all check at once, from 0 to 100.",
	})
	Register(Definition{
		Key: KeyAnnouncementsNewsletter, Category: CategoryDeploys, Kind: KindBool, Default: "true",
		Env: "APP_ANNOUNCEMENTS_NEWSLETTER", Apply: ApplyLive,
		Title:       "Show newsletter items",
		Description: "Show the quiet newsletter list when announcements are set to all. Newsletter items never appear in banners or the attention feed.",
	})
	registerAnnouncementCaps()
	Register(Definition{
		Key: KeyAnnouncementsDevKeys, Category: CategoryExperimental, Kind: KindStringList, Default: "",
		Env: "APP_ANNOUNCEMENTS_DEV_KEYS", Apply: ApplyLive,
		Title:       "Announcement dev signing keys",
		Description: "Extra trusted keys as id:base64key:tier|tier, for development and tests only. Ignored unless the instance runs in dev mode.",
	})
}

func registerAnnouncementCaps() {
	ints := []struct {
		key, env, def, title, desc string
		lo, hi                     int
	}{
		{KeyAnnouncementsMaxBytes, "APP_ANNOUNCEMENTS_MAX_BYTES", "262144", "Announcement feed size cap", "Largest feed file read, in bytes, from 1024 to 4194304.", 1024, 4194304},
		{KeyAnnouncementsMaxItems, "APP_ANNOUNCEMENTS_MAX_ITEMS", "50", "Announcement item cap", "Most items accepted in one feed, from 1 to 500. A feed over the cap is rejected whole.", 1, 500},
		{KeyAnnouncementsMaxTitle, "APP_ANNOUNCEMENTS_MAX_TITLE", "120", "Announcement title length", "Longest title, in characters, from 20 to 500.", 20, 500},
		{KeyAnnouncementsMaxBody, "APP_ANNOUNCEMENTS_MAX_BODY", "2000", "Announcement body length", "Longest body, in characters, from 100 to 10000.", 100, 10000},
	}
	for _, d := range ints {
		Register(Definition{
			Key: d.key, Category: CategoryDeploys, Kind: KindInt, Default: d.def, Env: d.env, Apply: ApplyLive,
			CheckValue: intRangeCheck(d.lo, d.hi), Title: d.title, Description: d.desc,
		})
	}
	durs := []struct{ key, env, def, title, desc string }{
		{KeyAnnouncementsSnoozeMax, "APP_ANNOUNCEMENTS_SNOOZE_MAX", "24h", "Critical notice snooze limit", "Longest a critical notice can be snoozed, up to 168h. It returns after that until the update is installed."},
		{KeyAnnouncementsClockSkew, "APP_ANNOUNCEMENTS_CLOCK_SKEW", "24h", "Announcement clock skew", "Tolerance for a host clock that is off when checking feed and item expiry, up to 168h."},
		{KeyAnnouncementsMaxAge, "APP_ANNOUNCEMENTS_FEED_MAX_AGE", "720h", "Announcement feed lifetime", "Longest lifetime a signed feed may claim, from 24h to 8760h. Longer claims are rejected."},
	}
	for _, d := range durs {
		lo, hi := time.Minute, 7*24*time.Hour
		if d.key == KeyAnnouncementsMaxAge {
			lo, hi = 24*time.Hour, 365*24*time.Hour
		}
		Register(Definition{
			Key: d.key, Category: CategoryDeploys, Kind: KindDuration, Default: d.def, Env: d.env, Apply: ApplyLive,
			CheckValue: durationRangeCheck(lo, hi), Title: d.title, Description: d.desc,
		})
	}
}

func announcementsURLCheck(canonical string) error {
	u, err := url.Parse(strings.TrimSpace(canonical))
	// Plain http to loopback is stored but the fetcher only follows it on dev builds.
	loopbackHTTP := u != nil && u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost")
	if err != nil || (u.Scheme != "https" && !loopbackHTTP) || u.Host == "" || u.User != nil {
		return fmt.Errorf("value must be an https URL without credentials")
	}
	return nil
}
