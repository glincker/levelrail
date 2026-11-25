package api

import (
	"sort"
	"sync"
	"time"
)

// slowRequestRingSize bounds memory: only the latest slow requests matter for triage.
const slowRequestRingSize = 50

// slowRequestEntry is one request that crossed the slow threshold.
type slowRequestEntry struct {
	At         time.Time `json:"at"`
	Method     string    `json:"method"`
	Route      string    `json:"route"`
	Status     int       `json:"status"`
	DurationMS float64   `json:"duration_ms"`
	Breakdown  string    `json:"breakdown,omitempty"`
}

// slowRequestRing keeps the most recent slow requests; the zero value is ready.
type slowRequestRing struct {
	mu      sync.Mutex
	entries []slowRequestEntry
	next    int
	stats   routeStats
}

func (r *slowRequestRing) add(e slowRequestEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) < slowRequestRingSize {
		r.entries = append(r.entries, e)
		return
	}
	r.entries[r.next] = e
	r.next = (r.next + 1) % slowRequestRingSize
}

// snapshot returns the entries newest first.
func (r *slowRequestRing) snapshot() []slowRequestEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]slowRequestEntry, 0, len(r.entries))
	n := len(r.entries)
	for i := 1; i <= n; i++ {
		out = append(out, r.entries[((r.next-i)%n+n)%n])
	}
	return out
}

// routeStatBound is the default cap on distinct routes tracked; the live runtime setting overrides it.
const (
	routeStatBound  = 1024
	routeStatReport = 10
	routeUnmatched  = "unmatched"
)

// routeStat is the latency aggregate for one method and route pattern.
type routeStat struct {
	Method  string  `json:"method"`
	Route   string  `json:"route"`
	Count   int64   `json:"count"`
	Errors  int64   `json:"errors"`
	TotalMS float64 `json:"total_ms"`
	AvgMS   float64 `json:"avg_ms"`
	MaxMS   float64 `json:"max_ms"`
}

// routeStats counts every API request per route pattern so the busiest and costliest show up; the zero value is ready.
type routeStats struct {
	mu sync.Mutex
	m  map[string]*routeStat
	// bound reads the live cap; nil means the default.
	bound func() int
}

func (s *routeStats) observe(method, route string, status int, d time.Duration) {
	if route == "" {
		route = routeUnmatched
	}
	key := method + " " + route
	ms := float64(d) / float64(time.Millisecond)
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.m[key]
	if e == nil {
		if len(s.m) >= s.limit() {
			return
		}
		if s.m == nil {
			s.m = map[string]*routeStat{}
		}
		e = &routeStat{Method: method, Route: route}
		s.m[key] = e
	}
	e.Count++
	e.TotalMS += ms
	e.MaxMS = max(e.MaxMS, ms)
	if status >= 500 {
		e.Errors++
	}
}

func (s *routeStats) limit() int {
	if s.bound != nil {
		if n := s.bound(); n > 0 {
			return n
		}
	}
	return routeStatBound
}

// top returns the routes with the most total time, ties broken by name.
func (s *routeStats) top(n int) []routeStat {
	s.mu.Lock()
	out := make([]routeStat, 0, len(s.m))
	for _, e := range s.m {
		c := *e
		c.AvgMS = c.TotalMS / float64(c.Count)
		out = append(out, c)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].TotalMS != out[j].TotalMS {
			return out[i].TotalMS > out[j].TotalMS
		}
		return out[i].Route+out[i].Method < out[j].Route+out[j].Method
	})
	return out[:min(len(out), n)]
}

func (s *routeStats) reset() {
	s.mu.Lock()
	s.m = nil
	s.mu.Unlock()
}
