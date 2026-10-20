// Package profile is the server profile: one switch that sets which optional subsystems start.
package profile

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Profile names a bundle of defaults for optional subsystems.
type Profile string

const (
	// Minimal is a single node with the optional machinery off.
	Minimal Profile = "minimal"
	// Standard is the default install.
	Standard Profile = "standard"
	// Suite is standard plus mesh, plugins and every experimental feature.
	Suite Profile = "suite"
)

// EnvVar pins the profile and wins over the saved choice. APP_PROFILE already
// names the CLI credentials profile, so the server uses its own variable.
const EnvVar = "APP_SERVER_PROFILE"

// Where the effective profile came from.
const (
	SourceEnv     = "env"
	SourceSetting = "setting"
	SourceDefault = "default"
)

// Env knobs for the profile thresholds.
const (
	EnvSmallDiskBytes     = "APP_SMALL_DISK_BYTES"
	EnvSmallDiskRetention = "APP_SMALL_DISK_RETENTION"
	EnvMinimalRetention   = "APP_PROFILE_MINIMAL_RETENTION"
	EnvLowRAMBytes        = "APP_LOW_RAM_BYTES"
	EnvLazyStartInterval  = "APP_PROFILE_LAZY_START_INTERVAL"
)

const (
	defaultSmallDiskBytes     = int64(20) << 30
	defaultSmallDiskRetention = 7 * 24 * time.Hour
	defaultMinimalRetention   = 3 * 24 * time.Hour
	defaultStandardRetention  = 15 * 24 * time.Hour
	defaultLowRAMBytes        = int64(2) << 30
	defaultLazyStart          = 5 * time.Minute
	minimalMinRAMBytes        = int64(512) << 20
)

// All returns every profile, lightest first.
func All() []Profile { return []Profile{Minimal, Standard, Suite} }

// Parse reads a profile name, case and space insensitive.
func Parse(s string) (Profile, bool) {
	p := Profile(strings.ToLower(strings.TrimSpace(s)))
	for _, c := range All() {
		if c == p {
			return p, true
		}
	}
	return "", false
}

// Rank orders profiles by how much they start, so a move can be called down.
func (p Profile) Rank() int {
	switch p {
	case Minimal:
		return 0
	case Suite:
		return 2
	}
	return 1
}

// Settings is what a profile turns on or off by default.
type Settings struct {
	Mesh            bool
	Features        []experimental.Feature
	LazyLoops       bool
	LoopbackAgent   bool
	Recommendations bool
	MinRAMBytes     int64
	MinCPUCount     int
	// EnvDefaults are read as if set in the environment when the real variable is unset.
	EnvDefaults map[string]string
}

// For returns the defaults of p.
func For(p Profile) Settings {
	switch p {
	case Minimal:
		return Settings{LazyLoops: true, LoopbackAgent: true, Recommendations: true, MinRAMBytes: minimalMinRAMBytes, MinCPUCount: 1}
	case Suite:
		return Settings{Mesh: true, Features: experimental.All(), EnvDefaults: map[string]string{"APP_BUILD_ATTEST": "true"}}
	}
	return Settings{}
}

// Resolve picks the profile: env, then the saved choice, then standard.
func Resolve(lookup func(string) (string, bool), saved string) (Profile, string) {
	if v, ok := lookup(EnvVar); ok {
		if p, valid := Parse(v); valid {
			return p, SourceEnv
		}
	}
	if p, ok := Parse(saved); ok {
		return p, SourceSetting
	}
	return Standard, SourceDefault
}

// Lookup wraps an env lookup so profile env defaults answer for unset variables.
func Lookup(base func(string) (string, bool), p Profile) func(string) (string, bool) {
	defaults := For(p).EnvDefaults
	return func(k string) (string, bool) {
		if v, ok := base(k); ok {
			return v, true
		}
		v, ok := defaults[k]
		return v, ok
	}
}

// Retention is the default metrics and log retention: short on minimal, shorter
// on a small disk, 15 days otherwise. An explicit retention env var still wins at the call site.
func Retention(p Profile, diskBytes int64, lookup func(string) (string, bool)) time.Duration {
	if p == Minimal {
		return duration(lookup, EnvMinimalRetention, defaultMinimalRetention)
	}
	if diskBytes > 0 && diskBytes < integer(lookup, EnvSmallDiskBytes, defaultSmallDiskBytes) {
		return duration(lookup, EnvSmallDiskRetention, defaultSmallDiskRetention)
	}
	return defaultStandardRetention
}

// LowRAMBytes is the memory below which an installer recommends the minimal profile.
func LowRAMBytes(lookup func(string) (string, bool)) int64 {
	return integer(lookup, EnvLowRAMBytes, defaultLowRAMBytes)
}

// LazyStartInterval is how often a lazily started loop checks whether work appeared.
func LazyStartInterval(lookup func(string) (string, bool)) time.Duration {
	return duration(lookup, EnvLazyStartInterval, defaultLazyStart)
}

func duration(lookup func(string) (string, bool), key string, def time.Duration) time.Duration {
	if v, ok := lookup(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
			return d
		}
	}
	return def
}

func integer(lookup func(string) (string, bool), key string, def int64) int64 {
	if v, ok := lookup(key); ok {
		if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return def
}

var (
	mu     sync.RWMutex
	active = Standard
	source = SourceDefault
)

// SetActive records the profile this process booted with.
func SetActive(p Profile, src string) {
	mu.Lock()
	defer mu.Unlock()
	active, source = p, src
}

// Active returns the profile this process booted with.
func Active() Profile {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// ActiveSource says where the active profile came from.
func ActiveSource() string {
	mu.RLock()
	defer mu.RUnlock()
	return source
}
