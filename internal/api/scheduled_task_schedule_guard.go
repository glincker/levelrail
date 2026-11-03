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
	if sched.Next(time.Now()).IsZero() {
		return errors.New("this schedule never matches a real date within four years")
	}
	return nil
}
