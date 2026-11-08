package footprint

import (
	"context"
	"runtime/pprof"
	"sort"
	"sync"
	"sync/atomic"
)

// Goroutine attribution: a per-subsystem live count for the health view, plus pprof labels
// so a goroutine dump names the owner.
var (
	subMu   sync.Mutex
	subLive = map[string]*atomic.Int64{}
)

// SubsystemCount is the number of live goroutines attributed to one subsystem.
type SubsystemCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

func counter(name string) *atomic.Int64 {
	subMu.Lock()
	defer subMu.Unlock()
	c, ok := subLive[name]
	if !ok {
		c = new(atomic.Int64)
		subLive[name] = c
	}
	return c
}

// Attribute counts the calling goroutine under name until the returned func runs.
func Attribute(name string) func() {
	c := counter(name)
	c.Add(1)
	return func() { c.Add(-1) }
}

// Run executes fn with a pprof "subsystem" label and the live count held.
func Run(ctx context.Context, name string, fn func(context.Context)) {
	done := Attribute(name)
	defer done()
	pprof.Do(ctx, pprof.Labels("subsystem", name), fn)
}

// Label is Run for code that cannot be wrapped in a closure, such as a stream handler: it counts
// and labels the calling goroutine until the returned func runs. Names must be a fixed set.
func Label(ctx context.Context, name string) func() {
	done := Attribute(name)
	pprof.SetGoroutineLabels(pprof.WithLabels(ctx, pprof.Labels("subsystem", name)))
	return func() {
		pprof.SetGoroutineLabels(ctx)
		done()
	}
}

// Subsystems lists non-zero counts, largest first.
func Subsystems() []SubsystemCount {
	subMu.Lock()
	defer subMu.Unlock()
	out := make([]SubsystemCount, 0, len(subLive))
	for n, c := range subLive {
		if v := c.Load(); v > 0 {
			out = append(out, SubsystemCount{Name: n, Count: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	return out
}
