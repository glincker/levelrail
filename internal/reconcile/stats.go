package reconcile

import (
	"sort"
	"sync/atomic"
	"time"
)

const passWindow = 128

// Stats summarizes recent reconcile passes for the control plane health page.
type Stats struct {
	Passes         int64         `json:"passes"`
	Controllers    int           `json:"controllers"`
	LastPassAt     time.Time     `json:"last_pass_at"`
	LastPass       time.Duration `json:"last_pass_ns"`
	P50            time.Duration `json:"p50_ns"`
	P95            time.Duration `json:"p95_ns"`
	SlowestName    string        `json:"slowest_controller"`
	SlowestElapsed time.Duration `json:"slowest_ns"`
	Pending        bool          `json:"pending"`
	Queue          *QueueStats   `json:"queue,omitempty"`
}

type passStats struct {
	passes   atomic.Int64
	lastEnd  atomic.Int64
	lastDur  atomic.Int64
	lastCtrl atomic.Int64
}

func (e *Engine) recordPass(start time.Time, controllers int) {
	d := time.Since(start)
	e.stats.passes.Add(1)
	e.stats.lastEnd.Store(time.Now().UnixNano())
	e.stats.lastDur.Store(int64(d))
	e.stats.lastCtrl.Store(int64(controllers))
	e.mu.Lock()
	e.ring = append(e.ring, d)
	if len(e.ring) > passWindow {
		e.ring = e.ring[len(e.ring)-passWindow:]
	}
	e.slowest, e.slowestName = e.passSlowest, e.passSlowestName
	e.passSlowest, e.passSlowestName = 0, ""
	e.mu.Unlock()
}

func (e *Engine) recordController(name string, d time.Duration) {
	e.mu.Lock()
	if d >= e.passSlowest {
		e.passSlowest, e.passSlowestName = d, name
	}
	e.mu.Unlock()
}

// Stats returns recent pass timing. Nothing in it is persisted.
func (e *Engine) Stats() Stats {
	s := Stats{Passes: e.stats.passes.Load(), Controllers: int(e.stats.lastCtrl.Load()), LastPass: time.Duration(e.stats.lastDur.Load()), Pending: len(e.nudge) > 0}
	if ns := e.stats.lastEnd.Load(); ns > 0 {
		s.LastPassAt = time.Unix(0, ns).UTC()
	}
	e.qmu.Lock()
	if e.queue != nil {
		qs := e.queue.stats(time.Now())
		s.Queue = &qs
	}
	e.qmu.Unlock()
	e.mu.RLock()
	win := append([]time.Duration(nil), e.ring...)
	s.SlowestName, s.SlowestElapsed = e.slowestName, e.slowest
	e.mu.RUnlock()
	if n := len(win); n > 0 {
		sort.Slice(win, func(i, j int) bool { return win[i] < win[j] })
		s.P50, s.P95 = win[n/2], win[min(n-1, n*95/100)]
	}
	return s
}
