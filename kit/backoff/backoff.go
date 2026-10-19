// Package backoff computes exponential retry delays.
package backoff

import "time"

// Exponential returns the wait before retry number attempt (1 is the first
// failure): base doubled for each earlier failure, capped at ceiling. A ceiling of
// zero or less caps the result to zero, so callers with an optional cap
// must handle that case themselves.
func Exponential(base, ceiling time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt && d < ceiling; i++ {
		d *= 2
	}
	if d > ceiling {
		d = ceiling
	}
	return d
}
