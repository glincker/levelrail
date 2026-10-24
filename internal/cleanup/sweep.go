package cleanup

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

// Environment variables that bound a run.
const (
	EnvMaxRows      = "APP_CLEANUP_MAX_ROWS"
	EnvSlice        = "APP_CLEANUP_SLICE"
	EnvStagger      = "APP_CLEANUP_STAGGER"
	EnvStartupDelay = "APP_CLEANUP_STARTUP_DELAY"
	EnvHistoryDays  = "APP_CLEANUP_HISTORY_DAYS"
	EnvUsageTTL     = "APP_CLEANUP_USAGE_TTL"
)

// Limits bound how much one run and the scheduler do.
type Limits struct {
	// MaxRows caps the rows one run removes; the run reports more_remaining when it stops there.
	MaxRows int64
	// Slice is the time span of one delete batch, so no single DELETE covers an unbounded range.
	Slice time.Duration
	// Stagger spaces targets within one scheduler tick.
	Stagger time.Duration
	// StartupDelay keeps the scheduler quiet right after boot.
	StartupDelay time.Duration
	// HistoryDays is how long run history is kept.
	HistoryDays int
	// UsageTTL is how long a measured usage is reused by the list endpoint.
	UsageTTL time.Duration
}

// LimitsFromEnv reads Limits, falling back to the defaults for unset or invalid values.
func LimitsFromEnv(lookup func(string) (string, bool)) Limits {
	l := Limits{MaxRows: 100000, Slice: 24 * time.Hour, Stagger: 5 * time.Second, StartupDelay: 5 * time.Minute, HistoryDays: 365, UsageTTL: 30 * time.Second}
	num := func(name string, dst *int64) {
		if v, ok := lookup(name); ok {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
				*dst = n
			}
		}
	}
	dur := func(name string, dst *time.Duration, allowZero bool) {
		if v, ok := lookup(name); ok {
			if d, err := time.ParseDuration(v); err == nil && (d > 0 || (allowZero && d == 0)) {
				*dst = d
			}
		}
	}
	num(EnvMaxRows, &l.MaxRows)
	days := int64(l.HistoryDays)
	num(EnvHistoryDays, &days)
	l.HistoryDays = int(days)
	dur(EnvSlice, &l.Slice, false)
	dur(EnvStagger, &l.Stagger, true)
	dur(EnvStartupDelay, &l.StartupDelay, true)
	dur(EnvUsageTTL, &l.UsageTTL, true)
	return l
}

// SweepByCutoff removes rows older than cutoff by calling del with a cutoff that advances one slice at a
// time from the oldest row, so each statement covers a bounded span and the row cap can stop the run between
// slices. del must delete rows strictly older than the cutoff it gets; nothing at or after cutoff is touched.
func SweepByCutoff(ctx context.Context, oldest, cutoff time.Time, slice time.Duration, maxRows int64, del func(ctx context.Context, cutoff time.Time) (int64, error)) (rows int64, more bool, err error) {
	if oldest.IsZero() || !oldest.Before(cutoff) {
		return 0, false, nil
	}
	if slice <= 0 {
		slice = 24 * time.Hour
	}
	for step := oldest.Add(slice); ; step = step.Add(slice) {
		if err := ctx.Err(); err != nil {
			return rows, true, fmt.Errorf("cleanup: stopped: %w", err)
		}
		at := step
		last := !at.Before(cutoff)
		if last {
			at = cutoff
		}
		n, derr := del(ctx, at)
		rows += n
		if derr != nil {
			return rows, !last, derr
		}
		if last {
			return rows, false, nil
		}
		if maxRows > 0 && rows >= maxRows {
			return rows, true, nil
		}
	}
}
