package appsleep

import (
	"context"
	"sync/atomic"
)

var stackSource atomic.Pointer[func(context.Context) bool]

// SetStackSource wires the live sleep.stack runtime setting. Nil turns stack sleep off.
func SetStackSource(fn func(context.Context) bool) {
	if fn == nil {
		stackSource.Store(nil)
		return
	}
	stackSource.Store(&fn)
}

// StackEnabled reports the current value so a change applies on the next tick.
func StackEnabled(ctx context.Context) bool {
	fn := stackSource.Load()
	return fn != nil && (*fn)(ctx)
}
