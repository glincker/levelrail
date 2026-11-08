package upgradesched

import (
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Defaults is the automatic upgrade policy an install gets until an operator
// saves one. The environment only seeds it; a saved policy always wins.
type Defaults struct {
	Mode           string
	WindowCron     string
	WindowDuration time.Duration
	Timezone       string
}

// Environment keys that seed Defaults and the setting bounds.
const (
	EnvDefaultMode           = "APP_UPGRADE_AUTO_DEFAULT_MODE"
	EnvDefaultWindowCron     = "APP_UPGRADE_AUTO_DEFAULT_WINDOW_CRON"
	EnvDefaultWindowDuration = "APP_UPGRADE_AUTO_DEFAULT_WINDOW_DURATION"
	EnvDefaultTimezone       = "APP_UPGRADE_AUTO_DEFAULT_TIMEZONE"
	EnvMaxPauseDays          = "APP_UPGRADE_MAX_PAUSE_DAYS"
	EnvMaxHealthWindow       = "APP_UPGRADE_MAX_HEALTH_WINDOW"
)

// DefaultsFromEnv returns the seeded default policy: on (any eligible
// release), nightly window. Set the mode key to "off" to ship an install opt-in.
func DefaultsFromEnv(lookup func(string) (string, bool)) Defaults {
	d := Defaults{Mode: ModeAny, WindowCron: "0 3 * * *", WindowDuration: 2 * time.Hour, Timezone: "UTC"}
	if v, ok := lookup(EnvDefaultMode); ok {
		d.Mode = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvDefaultWindowCron); ok && strings.TrimSpace(v) != "" {
		d.WindowCron = strings.TrimSpace(v)
	}
	if v, ok := lookup(EnvDefaultWindowDuration); ok {
		if dur, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && dur > 0 {
			d.WindowDuration = dur
		}
	}
	if v, ok := lookup(EnvDefaultTimezone); ok && strings.TrimSpace(v) != "" {
		d.Timezone = strings.TrimSpace(v)
	}
	return d
}

// Resolve returns the policy row in force. A row nobody has saved (empty
// UpdatedAt) gets the defaults; any saved choice, including off, is kept.
func (d Defaults) Resolve(row store.UpgradePolicy) (store.UpgradePolicy, bool) {
	if row.UpdatedAt != "" || d.Mode == "" || d.Mode == ModeOff || row.Mode != ModeOff {
		return row, false
	}
	pol := Policy{Mode: d.Mode, MinReleaseAgeDays: row.MinReleaseAgeDays, Window: Window{Cron: d.WindowCron, Duration: d.WindowDuration, Timezone: d.Timezone}}
	if pol.Validate() != nil {
		return row, false
	}
	row.Mode, row.WindowCron, row.WindowTimezone = d.Mode, d.WindowCron, d.Timezone
	row.WindowDurationSeconds = int(d.WindowDuration / time.Second)
	return row, true
}

// MaxPauseDays bounds a timed pause.
func MaxPauseDays(lookup func(string) (string, bool)) int {
	if v, ok := lookup(EnvMaxPauseDays); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return 90
}

// MaxHealthWindow bounds the post-upgrade health window an operator may set.
func MaxHealthWindow(lookup func(string) (string, bool)) time.Duration {
	if v, ok := lookup(EnvMaxHealthWindow); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			return d
		}
	}
	return 30 * time.Minute
}

// PausedUntil parses the stored timed pause; zero means none.
func PausedUntil(row store.UpgradePolicy) time.Time {
	return parseStamp(row.PausedUntil)
}
