package incident

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Defaults apply only when the matching APP_MONITOR_LINK_* variable is unset.
const (
	DefaultLinkConfirmFailures = 1
	DefaultLinkRecoveryPeriod  = 2 * time.Minute
	DefaultLinkSweepInterval   = 30 * time.Second
	DefaultLinkImpact          = "major"
)

// LinkConfig tunes the monitor to status page link. Per-monitor settings override the first two.
type LinkConfig struct {
	// ConfirmFailures is the consecutive failed probes a confirmed-down monitor needs before an incident opens.
	ConfirmFailures int
	// RecoveryPeriod is how long a monitor must stay up before its incident resolves.
	RecoveryPeriod time.Duration
	SweepInterval  time.Duration
	// Impact is the component impact the incident carries (minor or major).
	Impact   string
	QueueMax int
}

// LinkConfigFromEnv reads the settings; invalid values fall back to the default and are reported in the error.
func LinkConfigFromEnv(lookup func(string) (string, bool)) (LinkConfig, error) {
	var errs []string
	raw := func(name string) (string, bool) {
		v, ok := lookup(name)
		v = strings.TrimSpace(v)
		return v, ok && v != ""
	}
	cfg := LinkConfig{ConfirmFailures: DefaultLinkConfirmFailures, RecoveryPeriod: DefaultLinkRecoveryPeriod,
		SweepInterval: DefaultLinkSweepInterval, Impact: DefaultLinkImpact, QueueMax: DefaultQueueMax}
	if v, ok := raw("APP_MONITOR_LINK_CONFIRM_FAILURES"); ok {
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			errs = append(errs, "APP_MONITOR_LINK_CONFIRM_FAILURES must be a positive integer")
		} else {
			cfg.ConfirmFailures = n
		}
	}
	for name, dst := range map[string]*time.Duration{"APP_MONITOR_LINK_RECOVERY_PERIOD": &cfg.RecoveryPeriod, "APP_MONITOR_LINK_SWEEP_INTERVAL": &cfg.SweepInterval} {
		if v, ok := raw(name); ok {
			if d, err := time.ParseDuration(v); err != nil || d < 0 || (d == 0 && name != "APP_MONITOR_LINK_RECOVERY_PERIOD") {
				errs = append(errs, name+" must be a duration such as 2m")
			} else {
				*dst = d
			}
		}
	}
	if v, ok := raw("APP_MONITOR_LINK_IMPACT"); ok {
		if v != "minor" && v != "major" {
			errs = append(errs, "APP_MONITOR_LINK_IMPACT must be minor or major")
		} else {
			cfg.Impact = v
		}
	}
	if len(errs) > 0 {
		return cfg, fmt.Errorf("monitor link config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}
