// Package secretrotation schedules and performs per-secret rotation
// policies for app secrets: generate a new value, keep the old one for a
// grace window, restart the owning app, and record every step.
package secretrotation

import (
	"strconv"
	"strings"
	"time"
)

// Policy status values reported to operators.
const (
	StatusOK       = "ok"
	StatusDueSoon  = "due_soon"
	StatusOverdue  = "overdue"
	StatusUnknown  = "unknown"
	PreviousSuffix = "_PREVIOUS"
)

// Env names and defaults. The package keeps no global state, callers read
// the environment and pass values in.
const (
	EnvDueSoonDays   = "APP_SECRET_ROTATION_DUE_SOON_DAYS"
	EnvIntervalSecs  = "APP_SECRET_ROTATION_INTERVAL_SECONDS"
	EnvMaxPerPass    = "APP_SECRET_ROTATION_MAX_PER_PASS" //nolint:gosec // env var name, not a credential
	EnvGraceHours    = "APP_SECRET_ROTATION_GRACE_HOURS"
	EnvDefaultDays   = "APP_SECRET_ROTATION_DEFAULT_INTERVAL_DAYS"
	DefaultGraceHrs  = 24
	DefaultDays      = 90
	DefaultDueSoon   = 7 * 24 * time.Hour
	DefaultInterval  = time.Hour
	DefaultMaxPerRun = 10
)

// IntFromEnv reads a positive integer env value, or def when unset or invalid.
func IntFromEnv(getenv func(string) string, key string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(getenv(key)))
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// Basis picks the reference time for the next due date: the later of the
// policy's own last rotation and the secret value's last write, so a manual
// edit of the value also resets the clock.
func Basis(lastRotated, valueUpdated time.Time) time.Time {
	if valueUpdated.After(lastRotated) {
		return valueUpdated
	}
	return lastRotated
}

// NextDue returns when a rotation is due, or the zero time when the basis is
// unknown or the interval is not positive.
func NextDue(basis time.Time, intervalDays int) time.Time {
	if basis.IsZero() || intervalDays <= 0 {
		return time.Time{}
	}
	return basis.AddDate(0, 0, intervalDays)
}

// StatusAt classifies a due date against now and the due-soon window.
func StatusAt(now, due time.Time, soon time.Duration) string {
	switch {
	case due.IsZero():
		return StatusUnknown
	case !now.Before(due):
		return StatusOverdue
	case due.Sub(now) <= soon:
		return StatusDueSoon
	}
	return StatusOK
}

// PreviousKey is the env key that holds the prior value during the grace window.
func PreviousKey(key string) string { return key + PreviousSuffix }

// IsPreviousKey reports whether key is a grace companion, never a policy target.
func IsPreviousKey(key string) bool { return strings.HasSuffix(key, PreviousSuffix) }
