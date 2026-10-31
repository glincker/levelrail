// Package incident opens internal incidents from confirmed monitor transitions (ADR 061).
package incident

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Defaults apply only when the matching APP_INCIDENT* variable is unset.
const (
	DefaultFoldWindow    = 10 * time.Minute
	DefaultReopenWindow  = 15 * time.Minute
	DefaultSweepInterval = time.Minute
	DefaultOpenMax       = 200
	DefaultQueueMax      = 256
	DefaultUpdateBodyMax = 4096
)

// Config holds every knob of the incident opener, all from the environment.
type Config struct {
	// Enabled needs both APP_MONITORS_ENABLED and APP_INCIDENTS_ENABLED.
	Enabled       bool
	FoldWindow    time.Duration
	ReopenWindow  time.Duration
	SweepInterval time.Duration
	OpenMax       int
	QueueMax      int
	// UpdateBodyMax caps the bytes of an internal note.
	UpdateBodyMax int
}

// ConfigFromEnv reads the settings; invalid values fall back to the default and are reported in the error.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	var errs []string
	raw := func(name string) (string, bool) {
		v, ok := lookup(name)
		v = strings.TrimSpace(v)
		return v, ok && v != ""
	}
	dur := func(name string, def time.Duration) time.Duration {
		v, ok := raw(name)
		if !ok {
			return def
		}
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			errs = append(errs, name+" must be a positive duration such as 10m")
			return def
		}
		return d
	}
	num := func(name string, def int) int {
		v, ok := raw(name)
		if !ok {
			return def
		}
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			errs = append(errs, name+" must be a positive integer")
			return def
		}
		return n
	}
	flag := func(name string) bool {
		v, ok := raw(name)
		if !ok {
			return false
		}
		b, err := strconv.ParseBool(v)
		if err != nil {
			errs = append(errs, name+" must be true or false")
		}
		return b
	}
	cfg := Config{
		FoldWindow:    dur("APP_INCIDENT_FOLD_WINDOW", DefaultFoldWindow),
		ReopenWindow:  dur("APP_INCIDENT_REOPEN_WINDOW", DefaultReopenWindow),
		SweepInterval: dur("APP_INCIDENT_SWEEP_INTERVAL", DefaultSweepInterval),
		OpenMax:       num("APP_INCIDENT_OPEN_MAX", DefaultOpenMax),
		QueueMax:      num("APP_INCIDENT_QUEUE_MAX", DefaultQueueMax),
		UpdateBodyMax: num("APP_INCIDENT_UPDATE_BODY_MAX", DefaultUpdateBodyMax),
	}
	cfg.Enabled = flag("APP_MONITORS_ENABLED") && flag("APP_INCIDENTS_ENABLED")
	if len(errs) > 0 {
		return cfg, fmt.Errorf("incident config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}
