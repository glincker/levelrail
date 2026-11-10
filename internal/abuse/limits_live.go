package abuse

import (
	"context"
	"os"
	"sync/atomic"
)

// liveLimits lets the settings layer feed saved values to code that cannot
// import it (this package is imported by the settings registry).
var liveLimits atomic.Pointer[func(context.Context) Limits]

// SetLiveLimits installs the saved-settings source. Nil restores env reads.
func SetLiveLimits(f func(context.Context) Limits) {
	if f == nil {
		liveLimits.Store(nil)
		return
	}
	liveLimits.Store(&f)
}

// CurrentLimits returns the saved settings when a source is installed, else
// the env seeded values, so a changed setting reaches the next policy pass.
func CurrentLimits(ctx context.Context) Limits {
	if f := liveLimits.Load(); f != nil {
		return (*f)(ctx)
	}
	return LimitsFromEnv(os.Getenv)
}
