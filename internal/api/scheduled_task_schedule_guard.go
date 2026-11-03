package api

import (
	"errors"
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// checkScheduleFires rejects an expression such as 0 0 30 2 * that parses but
// has no match inside the cron search horizon, so it can never run.
func checkScheduleFires(expr string) error {
	sched, err := cronexpr.Parse(expr)
	if err != nil {
		return err
	}
	if !validNextFire(sched.Next(time.Now()), time.Now()) {
		return errors.New("this schedule never matches a real date within four years")
	}
	return nil
}

// validNextFire is false for the zero time, year 1 (a zero turned into a
// local time) and anything not strictly after now. TODO: swap for the shared cronexpr validator.
func validNextFire(next, now time.Time) bool {
	return !next.IsZero() && next.Year() > 1 && next.After(now)
}
