package selfupgrade

import (
	"os"
	"strconv"
	"time"
)

// Env names for the upgrade liveness limits. They seed the defaults; the host
// command inherits them through the API's passthrough list.
const (
	EnvDownloadStall    = "APP_UPGRADE_DOWNLOAD_STALL_SECONDS"
	EnvDownloadDeadline = "APP_UPGRADE_DOWNLOAD_DEADLINE_SECONDS"
	EnvHeartbeat        = "APP_UPGRADE_HEARTBEAT_SECONDS"
	EnvOrphanAfter      = "APP_UPGRADE_ORPHAN_SECONDS"
)

// Limits bound how long an upgrade may go without progress.
type Limits struct {
	// Stall fails a download that received no bytes for this long.
	Stall time.Duration
	// Deadline caps the whole download step.
	Deadline time.Duration
	// Heartbeat is how often a live worker proves it is alive.
	Heartbeat time.Duration
	// OrphanAfter is how long without a heartbeat before a running attempt
	// is treated as abandoned.
	OrphanAfter time.Duration
}

// DefaultLimits are conservative: a slow link still finishes, a dead one fails fast.
func DefaultLimits() Limits {
	return Limits{Stall: 60 * time.Second, Deadline: 30 * time.Minute, Heartbeat: 10 * time.Second, OrphanAfter: 90 * time.Second}
}

// LimitsFromEnv reads the limits, falling back to the defaults for unset or invalid values.
func LimitsFromEnv(getenv func(string) string) Limits {
	if getenv == nil {
		getenv = os.Getenv
	}
	l := DefaultLimits()
	set := func(dst *time.Duration, key string) {
		if n, err := strconv.Atoi(getenv(key)); err == nil && n > 0 {
			*dst = time.Duration(n) * time.Second
		}
	}
	set(&l.Stall, EnvDownloadStall)
	set(&l.Deadline, EnvDownloadDeadline)
	set(&l.Heartbeat, EnvHeartbeat)
	set(&l.OrphanAfter, EnvOrphanAfter)
	if l.OrphanAfter < 3*l.Heartbeat {
		l.OrphanAfter = 3 * l.Heartbeat
	}
	return l
}
