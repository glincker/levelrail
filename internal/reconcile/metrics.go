package reconcile

import (
	"sort"
	"sync"
	"time"
)

// ControllerCost is the cumulative time one controller has cost since start.
type ControllerCost struct {
	Name   string        `json:"name"`
	Runs   int64         `json:"runs"`
	Errors int64         `json:"errors"`
	Total  time.Duration `json:"total_ns"`
	Max    time.Duration `json:"max_ns"`
}

// costTable accumulates per-controller cost; the zero value is ready to use.
type costTable struct {
	mu sync.Mutex
	m  map[string]*ControllerCost
}

func (t *costTable) add(name string, d time.Duration, failed bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.m == nil {
		t.m = map[string]*ControllerCost{}
	}
	c := t.m[name]
	if c == nil {
		c = &ControllerCost{Name: name}
		t.m[name] = c
	}
	c.Runs++
	c.Total += d
	if d > c.Max {
		c.Max = d
	}
	if failed {
		c.Errors++
	}
}

// top returns up to n controllers by total time, most expensive first.
func (t *costTable) top(n int) []ControllerCost {
	t.mu.Lock()
	out := make([]ControllerCost, 0, len(t.m))
	for _, c := range t.m {
		out = append(out, *c)
	}
	t.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].Name < out[j].Name
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// SetResyncSource makes Run re-read the resync interval on every tick, so a
// live setting change applies without a restart. A non-positive value keeps
// the current interval.
func (e *Engine) SetResyncSource(f func() time.Duration) { e.resyncSource = f }

// refreshResync resets ticker when the source reports a new interval and
// returns the interval now in force.
func (e *Engine) refreshResync(t *time.Ticker, cur time.Duration) time.Duration {
	if e.resyncSource == nil {
		return cur
	}
	if d := e.resyncSource(); d > 0 && d != cur {
		t.Reset(d)
		return d
	}
	return cur
}
