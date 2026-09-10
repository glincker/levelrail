package main

import "time"

// defaultTimeRangeWindow mirrors internal/api's own defaultQueryWindow
// (internal/api/metrics.go), the lookback the server applies when a
// caller omits "from". Shared by every command that queries a from/to
// window (apps logs, apps/databases/nodes metrics) so they can't drift
// from each other or from the server's own default. Every request still
// sends an explicit "from", so a future change to the server's own
// default only affects what these commands show with no window flags
// given, never a silent disagreement about what "no window" means.
const defaultTimeRangeWindow = time.Hour

// timeRangeFlags is resolveTimeRange's raw, unvalidated input: exactly
// what flag.FlagSet parsed for --since/--from/--to, no time math done
// yet. Kept separate so resolveTimeRange is a pure function over plain
// data, testable without a flag.FlagSet or the real clock in the loop.
type timeRangeFlags struct {
	since string
	from  string
	to    string
}

// resolveTimeRange turns f into the concrete [from, to] window a query
// request sends, applying the --since > --from > default-window
// precedence the flags' own usage text documents. now stands in for
// time.Now() so this is deterministic and testable.
func resolveTimeRange(f timeRangeFlags, now time.Time) (from, to time.Time, err error) {
	if f.since != "" && f.from != "" {
		return time.Time{}, time.Time{}, newValidationError("--since and --from are mutually exclusive")
	}

	to = now
	if f.to != "" {
		parsed, parseErr := time.Parse(time.RFC3339, f.to)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--to must be RFC3339: %v", parseErr)
		}
		to = parsed
	}

	from = to.Add(-defaultTimeRangeWindow)
	switch {
	case f.from != "":
		parsed, parseErr := time.Parse(time.RFC3339, f.from)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--from must be RFC3339: %v", parseErr)
		}
		from = parsed
	case f.since != "":
		d, parseErr := time.ParseDuration(f.since)
		if parseErr != nil {
			return time.Time{}, time.Time{}, newValidationError("--since must be a valid duration (e.g. \"1h\"): %v", parseErr)
		}
		from = to.Add(-d)
	}
	return from, to, nil
}

// parseStepFlag parses --step's optional Go duration string (e.g.
// "60s"), matching internal/api's own parseStep contract (metrics.go):
// empty means step<=0, raw unaggregated samples rather than a bucketed
// aggregation.
func parseStepFlag(step string) (time.Duration, error) {
	if step == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(step)
	if err != nil {
		return 0, newValidationError("--step must be a valid duration (e.g. \"60s\"): %v", err)
	}
	return d, nil
}
