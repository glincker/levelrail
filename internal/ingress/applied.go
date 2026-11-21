package ingress

import (
	"context"
	"sync"
)

// passTracker lets an API call wait until an ingress pass that began after the call has been applied.
type passTracker struct {
	mu      sync.Mutex
	started uint64
	applied uint64
	waiters []chan struct{}
}

var passes passTracker

// BeginPass marks the start of an ingress reconcile pass and returns its sequence number.
func BeginPass() uint64 {
	passes.mu.Lock()
	defer passes.mu.Unlock()
	passes.started++
	return passes.started
}

// EndPass records that the pass with this sequence number applied its config.
func EndPass(seq uint64) {
	passes.mu.Lock()
	defer passes.mu.Unlock()
	if seq > passes.applied {
		passes.applied = seq
	}
	for _, w := range passes.waiters {
		close(w)
	}
	passes.waiters = nil
}

// PassMark snapshots the pass counter; call it before nudging the reconciler.
func PassMark() uint64 {
	passes.mu.Lock()
	defer passes.mu.Unlock()
	return passes.started
}

// WaitApplied blocks until a pass that started after mark has applied, or ctx ends.
// It reports whether such a pass applied.
func WaitApplied(ctx context.Context, mark uint64) bool {
	for {
		passes.mu.Lock()
		if passes.applied > mark {
			passes.mu.Unlock()
			return true
		}
		ch := make(chan struct{})
		passes.waiters = append(passes.waiters, ch)
		passes.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return false
		}
	}
}
