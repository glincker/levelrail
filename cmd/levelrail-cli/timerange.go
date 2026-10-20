package main

import "time"

// Always sent explicitly as from, so a server default change only affects runs with no window flags.
const defaultTimeRangeWindow = time.Hour

// Raw flag input; resolveTimeRange does the time math so it stays testable without a FlagSet.
type timeRangeFlags struct {
	since string
	from  string
	to    string
}

// --since takes precedence over --from, then the default window. now is injected for tests.
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

// Empty means step 0, which returns raw samples instead of buckets.
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
