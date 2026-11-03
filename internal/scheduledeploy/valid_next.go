package scheduledeploy

import "time"

// validNext is false for the zero time, year 1 (a zero turned into a local
// time) and anything not strictly after now. TODO: swap for the shared cronexpr validator.
func validNext(next, now time.Time) bool {
	return !next.IsZero() && next.UTC().Year() > 1 && next.After(now)
}
