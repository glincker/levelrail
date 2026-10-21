package scheduledtask

import (
	"strconv"
	"time"
)

// Limits are the control-plane-wide knobs for scheduled jobs, all
// environment-driven so no threshold is hardcoded at a call site.
type Limits struct {
	DefaultTimeout      time.Duration
	MaxRetries          int
	RetryBackoff        time.Duration
	MaxRetryBackoff     time.Duration
	CatchUpLookback     time.Duration
	HistoryKeep         int
	OutputRetention     time.Duration
	MaxPerApp           int
	MaxConcurrentPerApp int
	MaxOutputBytes      int
	BackgroundTimeout   time.Duration
}

// Defaults for Limits; each is overridable by the env var named next to it.
const (
	defaultMaxRetries          = 5
	defaultRetryBackoff        = 10 * time.Second
	defaultMaxRetryBackoff     = 5 * time.Minute
	defaultCatchUpLookback     = time.Hour
	defaultHistoryKeep         = 200
	defaultOutputRetention     = 30 * 24 * time.Hour
	defaultMaxPerApp           = 20
	defaultMaxConcurrentPerApp = 3
	defaultBackgroundTimeout   = 15 * time.Minute
)

// LimitsFromEnv reads APP_JOBS_DEFAULT_TIMEOUT, APP_JOBS_MAX_RETRIES,
// APP_JOBS_RETRY_BACKOFF, APP_JOBS_RETRY_BACKOFF_MAX,
// APP_JOBS_CATCH_UP_LOOKBACK, APP_JOBS_HISTORY_KEEP,
// APP_JOBS_OUTPUT_RETENTION, APP_JOBS_MAX_PER_APP,
// APP_JOBS_MAX_CONCURRENT_PER_APP, APP_JOBS_MAX_OUTPUT_BYTES and
// APP_JOBS_BACKGROUND_TIMEOUT (how long a background HTTP job is awaited). A value
// that does not parse or is not positive falls back to its default.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	dur := func(key string, def time.Duration) time.Duration {
		if raw, ok := lookup(key); ok {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				return d
			}
		}
		return def
	}
	num := func(key string, def int) int {
		if raw, ok := lookup(key); ok {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				return n
			}
		}
		return def
	}
	return Limits{
		DefaultTimeout:      dur("APP_JOBS_DEFAULT_TIMEOUT", defaultRunTimeout),
		MaxRetries:          num("APP_JOBS_MAX_RETRIES", defaultMaxRetries),
		RetryBackoff:        dur("APP_JOBS_RETRY_BACKOFF", defaultRetryBackoff),
		MaxRetryBackoff:     dur("APP_JOBS_RETRY_BACKOFF_MAX", defaultMaxRetryBackoff),
		CatchUpLookback:     dur("APP_JOBS_CATCH_UP_LOOKBACK", defaultCatchUpLookback),
		HistoryKeep:         num("APP_JOBS_HISTORY_KEEP", defaultHistoryKeep),
		OutputRetention:     dur("APP_JOBS_OUTPUT_RETENTION", defaultOutputRetention),
		MaxPerApp:           num("APP_JOBS_MAX_PER_APP", defaultMaxPerApp),
		MaxConcurrentPerApp: num("APP_JOBS_MAX_CONCURRENT_PER_APP", defaultMaxConcurrentPerApp),
		MaxOutputBytes:      num("APP_JOBS_MAX_OUTPUT_BYTES", maxOutputBytes),
		BackgroundTimeout:   dur("APP_JOBS_BACKGROUND_TIMEOUT", defaultBackgroundTimeout),
	}
}

// DefaultLimits is LimitsFromEnv with an empty environment.
func DefaultLimits() Limits {
	return LimitsFromEnv(func(string) (string, bool) { return "", false })
}
