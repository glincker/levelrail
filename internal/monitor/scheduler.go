package monitor

import (
	"container/heap"
	"math/rand/v2"
	"sync"
	"time"
)

type item struct {
	m   Monitor
	due time.Time
	idx int
}

type dueHeap []*item

func (h dueHeap) Len() int           { return len(h) }
func (h dueHeap) Less(i, j int) bool { return h[i].due.Before(h[j].due) }
func (h dueHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].idx, h[j].idx = i, j
}
func (h *dueHeap) Push(x any) {
	it := x.(*item)
	it.idx = len(*h)
	*h = append(*h, it)
}
func (h *dueHeap) Pop() any {
	old := *h
	it := old[len(old)-1]
	*h = old[:len(old)-1]
	return it
}

// Scheduler keeps monitors in a min-heap by due time. It takes the time as an
// argument so tests drive it with a fake clock.
type Scheduler struct {
	mu       sync.Mutex
	cfg      Config
	h        dueHeap
	byID     map[string]*item
	inflight map[string]bool
	jitter   func(limit time.Duration) time.Duration
}

// NewScheduler builds a Scheduler. A nil jitter uses a random share of the interval.
func NewScheduler(cfg Config, jitter func(time.Duration) time.Duration) *Scheduler {
	if jitter == nil {
		jitter = func(limit time.Duration) time.Duration {
			if limit <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(limit))) //nolint:gosec,forbidigo // jitter only spreads probe start times
		}
	}
	return &Scheduler{cfg: cfg, byID: map[string]*item{}, inflight: map[string]bool{}, jitter: jitter}
}

// interval clamps to the current floor so raising a floor also applies to stored monitors.
func (s *Scheduler) interval(m Monitor) time.Duration {
	if floor := s.cfg.MinInterval(m.Kind); m.Interval < floor {
		return floor
	}
	return m.Interval
}

func (s *Scheduler) spread(iv time.Duration) time.Duration {
	return s.jitter(iv * time.Duration(s.cfg.JitterPercent) / 100)
}

// Add schedules m, replacing an earlier entry with the same id. A disabled monitor is only removed.
func (s *Scheduler) Add(m Monitor, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(m.ID)
	if !m.Enabled {
		return
	}
	it := &item{m: m, due: now.Add(s.spread(s.interval(m)))}
	s.byID[m.ID] = it
	heap.Push(&s.h, it)
}

// Remove drops a monitor from the schedule.
func (s *Scheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked(id)
}

func (s *Scheduler) removeLocked(id string) {
	if it, ok := s.byID[id]; ok {
		heap.Remove(&s.h, it.idx)
		delete(s.byID, id)
	}
}

// Due returns the monitors to probe at now and schedules their next run. A
// monitor with a probe still in flight is pushed back, never run twice at once.
func (s *Scheduler) Due(now time.Time) []Monitor {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Monitor
	for s.h.Len() > 0 && !s.h[0].due.After(now) {
		it := s.h[0]
		iv := s.interval(it.m)
		it.due = now.Add(iv + s.spread(iv))
		heap.Fix(&s.h, 0)
		if s.inflight[it.m.ID] {
			continue
		}
		s.inflight[it.m.ID] = true
		out = append(out, it.m)
	}
	return out
}

// Done marks a probe finished so the monitor may be dispatched again.
func (s *Scheduler) Done(id string) {
	s.mu.Lock()
	delete(s.inflight, id)
	s.mu.Unlock()
}

// Len is the number of scheduled monitors.
func (s *Scheduler) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.h.Len()
}
