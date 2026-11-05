package api

import (
	"time"

	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// checkScheduleFires rejects an expression such as 0 0 30 2 * that parses but
// has no occurrence, so it can never run.
func checkScheduleFires(expr string) error {
	return cronexpr.Validate(expr, time.UTC, time.Now())
}
