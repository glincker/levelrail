package runtimesettings

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Keys that keep automatic updates reliable when the release source is slow or blocked.
const (
	KeyUpdatesCheckBackoffMax  = "updates.check_backoff_max"
	KeyUpdatesCheckAlertAfter  = "updates.check_alert_after"
	KeyUpdatesFallbackBaseURL  = "updates.fallback_release_base_url"
	updatesCheckBackoffMaxFlor = time.Minute
	updatesCheckBackoffMaxCeil = 72 * time.Hour
)

func init() {
	Register(Definition{
		Key: KeyUpdatesCheckBackoffMax, Category: CategoryDeploys, Kind: KindDuration, Default: "6h",
		Apply: ApplyLive, CheckValue: durationRangeCheck(updatesCheckBackoffMaxFlor, updatesCheckBackoffMaxCeil),
		Title:       "Update check retry ceiling",
		Description: "When looking for a release fails, the wait before the next try doubles from one minute up to this value, from 1m to 72h. A successful check resets it.",
	})
	Register(Definition{
		Key: KeyUpdatesCheckAlertAfter, Category: CategoryDeploys, Kind: KindInt, Default: "5",
		Apply: ApplyLive, CheckValue: intRangeCheck(1, 100),
		Title:       "Update check failures before an alert",
		Description: "How many checks in a row may fail before an alert is sent through your notification channels, from 1 to 100. One alert is sent per streak, and another when checks recover.",
	})
	Register(Definition{
		Key: KeyUpdatesFallbackBaseURL, Category: CategoryDeploys, Kind: KindString, Default: "",
		Apply: ApplyLive, CheckValue: fallbackBaseURLCheck,
		Title:       "Fallback release download URL",
		Description: "An https mirror laid out like the primary release downloads (base/tag/asset), tried only when the primary download fails. A release from the mirror must carry a valid signature. Empty means no fallback. The next upgrade uses a changed value.",
	})
}

func fallbackBaseURLCheck(canonical string) error {
	v := strings.TrimSpace(canonical)
	if v == "" {
		return nil
	}
	u, err := url.Parse(v)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return fmt.Errorf("value must be an https URL without credentials, or empty")
	}
	return nil
}
