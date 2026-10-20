package main

import (
	"math/rand/v2"
	"time"
)

// A session that stayed up this long counts as healthy, so the next failure restarts the backoff.
const healthySessionAfter = time.Minute

// nextReconnect returns how long to wait now (the current delay plus up to 50% jitter, so a fleet that lost the control plane together does not return together) and the delay for the attempt after.
func nextReconnect(cur, sessionLasted time.Duration, rnd func(int64) int64) (wait, next time.Duration) {
	if sessionLasted >= healthySessionAfter || cur < reconnectBaseDelay {
		cur = reconnectBaseDelay
	}
	wait = cur + time.Duration(rnd(int64(cur)/2+1))
	next = min(cur*2, reconnectMaxDelay)
	return wait, next
}

func randInt63n(n int64) int64 {
	return rand.Int64N(n) //nolint:gosec,forbidigo // reconnect jitter, not security
}
