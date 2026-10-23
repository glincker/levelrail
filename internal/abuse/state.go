package abuse

import "sync"

// State is what survives a policy reload for one app: the limiter buckets and
// the counters.
type State struct {
	Limiter  *Limiter
	Counters *Counters
}

// Registry hands out one State per app. It is touched on config load and on
// each metrics drain, never per request.
type Registry struct {
	mu     sync.Mutex
	states map[string]*State
	clock  Clock
}

// NewRegistry returns an empty registry. A nil clock uses MonotonicClock.
func NewRegistry(clock Clock) *Registry {
	return &Registry{states: make(map[string]*State), clock: clock}
}

var defaultRegistry = NewRegistry(nil)

// DefaultRegistry is the process-wide registry the ingress handler uses;
// Caddy modules are built from JSON and cannot be handed a live value.
func DefaultRegistry() *Registry { return defaultRegistry }

// For returns app's state, creating it sized by l on first use.
func (r *Registry) For(app string, l Limits) *State {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.states[app]
	if !ok {
		st = &State{
			Limiter:  NewLimiter(l.MaxTrackedClients, l.Shards, l.IdleEvict, r.clock),
			Counters: &Counters{},
		}
		r.states[app] = st
	}
	return st
}

// Forget drops an app's state, for an app that was deleted.
func (r *Registry) Forget(app string) {
	r.mu.Lock()
	delete(r.states, app)
	r.mu.Unlock()
}

// DrainAll returns each app's counters since the last call, omitting apps
// with nothing to report.
func (r *Registry) DrainAll() map[string]Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out map[string]Snapshot
	for app, st := range r.states {
		s := st.Counters.Drain()
		if s.Zero() {
			continue
		}
		if out == nil {
			out = make(map[string]Snapshot)
		}
		out[app] = s
	}
	return out
}

// NoteFailOpen counts one request served unchecked for app, because the
// policy could not be loaded or applied.
func (r *Registry) NoteFailOpen(app string, l Limits) {
	r.For(app, l).Counters.NoteFailOpen()
}

// Tracked is how many clients app's limiter holds, 0 for an unknown app.
func (r *Registry) Tracked(app string) int {
	r.mu.Lock()
	st := r.states[app]
	r.mu.Unlock()
	if st == nil {
		return 0
	}
	return st.Limiter.Len()
}
