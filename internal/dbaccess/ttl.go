package dbaccess

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// Environment variables that tune temporary credential lifetimes, in minutes.
const (
	EnvTTLMinMinutes     = "APP_DB_TEMP_TTL_MIN_MINUTES"
	EnvTTLMaxMinutes     = "APP_DB_TEMP_TTL_MAX_MINUTES"
	EnvTTLDefaultMinutes = "APP_DB_TEMP_TTL_DEFAULT_MINUTES"
	EnvSweepSeconds      = "APP_DB_TEMP_SWEEP_SECONDS"

	defaultTTLMin     = 15 * time.Minute
	defaultTTLMax     = 24 * time.Hour
	defaultTTLDefault = time.Hour
	defaultSweep      = 30 * time.Second

	// TempConnLimit caps concurrent sessions for a temporary role.
	TempConnLimit = 5
)

// TTLLimits bound how long a temporary credential may live.
type TTLLimits struct {
	Min, Max, Default time.Duration
}

// DefaultTTLLimits returns the built-in 15 minute to 24 hour window.
func DefaultTTLLimits() TTLLimits {
	return TTLLimits{Min: defaultTTLMin, Max: defaultTTLMax, Default: defaultTTLDefault}
}

// TTLLimitsFromEnv reads overrides through getenv, ignoring unparseable or
// inconsistent values.
func TTLLimitsFromEnv(getenv func(string) string) TTLLimits {
	l := DefaultTTLLimits()
	l.Min = minutesFromEnv(getenv, EnvTTLMinMinutes, l.Min)
	l.Max = minutesFromEnv(getenv, EnvTTLMaxMinutes, l.Max)
	l.Default = minutesFromEnv(getenv, EnvTTLDefaultMinutes, l.Default)
	if l.Min > l.Max {
		return DefaultTTLLimits()
	}
	l.Default = min(max(l.Default, l.Min), l.Max)
	return l
}

func minutesFromEnv(getenv func(string) string, key string, fallback time.Duration) time.Duration {
	n, err := strconv.Atoi(getenv(key))
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Minute
}

// SweepIntervalFromEnv returns how often the sweeper runs.
func SweepIntervalFromEnv(getenv func(string) string) time.Duration {
	n, err := strconv.Atoi(getenv(EnvSweepSeconds))
	if err != nil || n <= 0 {
		return defaultSweep
	}
	return time.Duration(n) * time.Second
}

// Clamp returns requested limited to the window, and whether it had to be
// changed. A zero request means the default.
func (l TTLLimits) Clamp(requested time.Duration) (time.Duration, bool) {
	switch {
	case requested <= 0:
		return l.Default, false
	case requested < l.Min:
		return l.Min, true
	case requested > l.Max:
		return l.Max, true
	}
	return requested, false
}

// NewTempRoleName returns a fresh tmp_ role name.
func NewTempRoleName() (string, error) {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("dbaccess: temp role name: %w", err)
	}
	return TempRolePrefix + hex.EncodeToString(b), nil
}
