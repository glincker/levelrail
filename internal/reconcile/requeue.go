package reconcile

import (
	"os"
	"sync"
	"time"
)

// EnvRequeueMax caps the backoff of Result.RequeueAfter.
const EnvRequeueMax = "APP_RECONCILE_REQUEUE_MAX"

const defaultRequeueMax = time.Minute

type requeueState struct {
	mu     sync.Mutex
	streak map[string]int
}

// scheduleRequeue nudges the engine after base, doubling per consecutive
// request for the same controller up to the cap; a zero base resets the streak.
func (e *Engine) scheduleRequeue(name string, base time.Duration) {
	rq := &e.requeue
	rq.mu.Lock()
	if base <= 0 {
		delete(rq.streak, name)
		rq.mu.Unlock()
		return
	}
	if rq.streak == nil {
		rq.streak = map[string]int{}
	}
	n := rq.streak[name]
	rq.streak[name] = n + 1
	rq.mu.Unlock()

	limit := defaultRequeueMax
	if d, err := time.ParseDuration(os.Getenv(EnvRequeueMax)); err == nil && d > 0 {
		limit = d
	}
	delay := base
	for i := 0; i < n && delay < limit; i++ {
		delay *= 2
	}
	if delay > limit {
		delay = limit
	}
	time.AfterFunc(delay, e.Nudge)
}
