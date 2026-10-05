package orphans

import (
	"strconv"
	"time"
)

// Mode is how the reaper acts on what it finds.
type Mode string

// Reaper modes.
const (
	ModeOn     Mode = "on"
	ModeDryRun Mode = "dry-run"
	ModeOff    Mode = "off"
)

// Environment variables that tune the reaper.
const (
	EnvMode        = "APP_ORPHAN_REAPER"
	EnvGrace       = "APP_ORPHAN_GRACE"
	EnvReapVolumes = "APP_ORPHAN_REAP_VOLUMES"
	EnvVolumeGrace = "APP_ORPHAN_VOLUME_GRACE"
	EnvCertGrace   = "APP_ORPHAN_CERT_GRACE"
	EnvMaxRemovals = "APP_ORPHAN_MAX_REMOVALS"
	EnvInterval    = "APP_ORPHAN_INTERVAL"
)

// Config tunes the reaper.
type Config struct {
	Mode        Mode
	Grace       time.Duration
	VolumeGrace time.Duration
	CertGrace   time.Duration
	ReapVolumes bool
	MaxPerPass  int
	// Interval is the minimum time between scheduled passes.
	Interval time.Duration
}

// DefaultConfig is the safe default: containers reaped after 15 minutes,
// volumes only listed.
func DefaultConfig() Config {
	return Config{Mode: ModeOn, Grace: 15 * time.Minute, VolumeGrace: 7 * 24 * time.Hour, CertGrace: 24 * time.Hour, MaxPerPass: 20, Interval: time.Minute}
}

// ConfigFromEnv reads the reaper settings, falling back to DefaultConfig for
// anything unset or malformed.
func ConfigFromEnv(lookup func(string) (string, bool)) Config {
	cfg := DefaultConfig()
	get := func(k string) string { v, _ := lookup(k); return v }
	switch Mode(get(EnvMode)) {
	case ModeOff, ModeDryRun, ModeOn:
		cfg.Mode = Mode(get(EnvMode))
	}
	if d, err := time.ParseDuration(get(EnvGrace)); err == nil && d >= 0 {
		cfg.Grace = d
	}
	if d, err := time.ParseDuration(get(EnvCertGrace)); err == nil && d >= 0 {
		cfg.CertGrace = d
	}
	if d, err := time.ParseDuration(get(EnvVolumeGrace)); err == nil && d >= 0 {
		cfg.VolumeGrace = d
	}
	if b, err := strconv.ParseBool(get(EnvReapVolumes)); err == nil {
		cfg.ReapVolumes = b
	}
	if d, err := time.ParseDuration(get(EnvInterval)); err == nil && d >= 0 {
		cfg.Interval = d
	}
	if n, err := strconv.Atoi(get(EnvMaxRemovals)); err == nil && n > 0 {
		cfg.MaxPerPass = n
	}
	return cfg
}

func (c Config) graceFor(k Kind) time.Duration {
	switch k {
	case KindVolume:
		return c.VolumeGrace
	case KindCertificate:
		return c.CertGrace
	}
	return c.Grace
}
