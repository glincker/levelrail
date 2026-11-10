package scheduledtask

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Runtime settings keys and env seeds for the job guard rails. Every value is
// read on each run, so a saved change applies without a restart.
const (
	KeyTimeoutMinSeconds        = "jobs.timeout_min_seconds"
	EnvTimeoutMinSeconds        = "APP_JOBS_TIMEOUT_MIN_SECONDS"
	KeyTimeoutMaxSeconds        = "jobs.timeout_max_seconds"
	EnvTimeoutMaxSeconds        = "APP_JOBS_TIMEOUT_MAX_SECONDS"
	KeyRetentionMaxRuns         = "jobs.retention_max_runs"
	EnvRetentionMaxRuns         = "APP_JOBS_RETENTION_MAX_RUNS"
	KeyRetentionMaxDays         = "jobs.retention_max_days"
	EnvRetentionMaxDays         = "APP_JOBS_RETENTION_MAX_DAYS"
	KeyRetentionSweepMaxRows    = "jobs.retention_sweep_max_rows"
	EnvRetentionSweepMaxRows    = "APP_JOBS_RETENTION_SWEEP_MAX_ROWS"
	KeyFailureAlertWindowSecond = "jobs.failure_alert_window_seconds"
	EnvFailureAlertWindowSecond = "APP_JOBS_FAILURE_ALERT_WINDOW_SECONDS"
	KeyFailureAlertBatch        = "jobs.failure_alert_batch"
	EnvFailureAlertBatch        = "APP_JOBS_FAILURE_ALERT_BATCH"
	KeyRetentionSkippedKeep     = "jobs.retention_skipped_keep"
	EnvRetentionSkippedKeep     = "APP_JOBS_RETENTION_SKIPPED_KEEP"
)

// Compiled fallbacks when no settings service is wired (tests, early boot);
// they match the registered defaults so pruning never silently stops.
const (
	fallbackSweepMaxRows = 500
	fallbackSkippedKeep  = 20
)

func guardSettingOr(key string, fallback int) int {
	if v := guardSetting(key); v > 0 {
		return v
	}
	return fallback
}

var guardResolver atomic.Pointer[func(key string) int]

// SetGuardResolver feeds the saved guard settings; 0 from it means unset.
func SetGuardResolver(fn func(key string) int) {
	guardResolver.Store(&fn)
}

func guardSetting(key string) int {
	if fn := guardResolver.Load(); fn != nil {
		return (*fn)(key)
	}
	return 0
}

// FailureAlertWindow is the per-job failure alert rate limit window, 0 when
// no settings service is wired (the alert then stays off).
func FailureAlertWindow() time.Duration {
	return time.Duration(guardSetting(KeyFailureAlertWindowSecond)) * time.Second
}

// FailureAlertBatch bounds the jobs one alert rule claims per tick.
func FailureAlertBatch() int { return guardSetting(KeyFailureAlertBatch) }

// clampTimeout keeps an attempt timeout inside the live min and max.
func clampTimeout(d time.Duration) time.Duration {
	if lo := time.Duration(guardSetting(KeyTimeoutMinSeconds)) * time.Second; lo > 0 && d < lo {
		d = lo
	}
	if hi := time.Duration(guardSetting(KeyTimeoutMaxSeconds)) * time.Second; hi > 0 && d > hi {
		d = hi
	}
	return d
}

// GuardReader reads a job's guard rails; *store.DB satisfies it.
type GuardReader interface {
	GetScheduledTaskGuards(ctx context.Context, id string) (store.ScheduledTaskGuards, error)
}

// retention resolves one bounded prune pass: the job's own keep values
// win over the defaults, and both are capped by the live maximums.
func (r *Runner) retention(ctx context.Context, taskID string, ended time.Time) store.RunRetention {
	p := store.RunRetention{
		Keep:         r.historyKeep(ctx),
		OutputCutoff: ended.Add(-r.limits().OutputRetention),
		MaxRows:      guardSettingOr(KeyRetentionSweepMaxRows, fallbackSweepMaxRows),
		SkippedKeep:  guardSettingOr(KeyRetentionSkippedKeep, fallbackSkippedKeep),
	}
	var g store.ScheduledTaskGuards
	if gr, ok := r.History.(GuardReader); ok {
		if got, err := gr.GetScheduledTaskGuards(ctx, taskID); err == nil {
			g = got
		}
	}
	if g.KeepRuns > 0 {
		p.Keep = g.KeepRuns
	}
	if hi := guardSetting(KeyRetentionMaxRuns); hi > 0 && p.Keep > hi {
		p.Keep = hi
	}
	days := g.KeepDays
	if hi := guardSetting(KeyRetentionMaxDays); hi > 0 && (days == 0 || days > hi) {
		days = hi
	}
	if days > 0 {
		p.Before = ended.Add(-time.Duration(days) * 24 * time.Hour)
	}
	return p
}
