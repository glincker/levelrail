package reconcile

import (
	"strings"
	"sync"
	"time"
)

// lagLedger remembers when each controller was first told its desired state
// changed and has not yet been reconciled successfully since.
type lagLedger struct {
	mu      sync.Mutex
	seen    map[string]struct{}
	pending map[string]time.Time
}

// markAll starts the lag clock for every known controller; markOne for one.
func (l *lagLedger) markAll(names []string, at time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, n := range names {
		l.markLocked(n, at)
	}
	for n := range l.seen {
		l.markLocked(n, at)
	}
}

func (l *lagLedger) markOne(name string, at time.Time) {
	l.mu.Lock()
	l.markLocked(name, at)
	l.mu.Unlock()
}

func (l *lagLedger) markLocked(name string, at time.Time) {
	if l.seen == nil {
		l.seen, l.pending = map[string]struct{}{}, map[string]time.Time{}
	}
	l.seen[name] = struct{}{}
	if _, ok := l.pending[name]; !ok {
		l.pending[name] = at
	}
}

// observed clears the clock when a run that began at start happened, pass or
// fail: lag counts missed reconciles, so a crash loop must not look like lag.
// A change that arrived during the run stays pending.
func (l *lagLedger) observed(name string, start time.Time, ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen, l.pending = map[string]struct{}{}, map[string]time.Time{}
	}
	l.seen[name] = struct{}{}
	if at, has := l.pending[name]; ok && has && !at.After(start) {
		delete(l.pending, name)
	}
}

func (l *lagLedger) forget(live map[string]struct{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for n := range l.seen {
		if _, ok := live[n]; !ok {
			delete(l.seen, n)
			delete(l.pending, n)
		}
	}
}

// byReconciler returns the worst lag per reconciler kind (the controller name
// up to the first slash), zero for kinds with nothing pending.
func (l *lagLedger) byReconciler(now time.Time) map[string]time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]time.Duration{}
	for n := range l.seen {
		kind := n
		if i := strings.IndexByte(n, '/'); i > 0 {
			kind = n[:i]
		}
		var d time.Duration
		if at, ok := l.pending[n]; ok {
			d = max(0, now.Sub(at))
		}
		if d >= out[kind] {
			out[kind] = d
		}
	}
	return out
}

func (e *Engine) controllerNames() []string {
	names := make([]string, 0, len(e.controllers))
	for _, c := range e.controllers {
		names = append(names, c.Name())
	}
	return names
}

// Lag reports, per reconciler kind, how long its oldest desired change has
// waited for a successful reconcile. Zero means converged. Nothing is persisted.
func (e *Engine) Lag() map[string]time.Duration {
	return e.lag.byReconciler(e.now())
}
