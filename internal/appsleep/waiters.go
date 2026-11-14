package appsleep

import "sync"

var wakeLocks sync.Map

// wakeLock returns the per-app mutex that makes check-then-start in Wake atomic.
func wakeLock(name string) *sync.Mutex {
	m, _ := wakeLocks.LoadOrStore(name, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// WaiterGate counts requests held per app while it wakes. The zero value is ready to use.
type WaiterGate struct {
	mu sync.Mutex
	in map[string]int
}

// Acquire takes a waiter slot; ok is false when limit slots are taken.
// A limit of 0 or less never refuses.
func (g *WaiterGate) Acquire(name string, limit int) (release func(), ok bool) {
	if limit <= 0 {
		return func() {}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.in == nil {
		g.in = map[string]int{}
	}
	if g.in[name] >= limit {
		return func() {}, false
	}
	g.in[name]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.in[name]--; g.in[name] <= 0 {
				delete(g.in, name)
			}
		})
	}, true
}
