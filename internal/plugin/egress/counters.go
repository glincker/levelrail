package egress

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
)

// DenialCount is how often one plugin was refused for one reason.
type DenialCount struct {
	Plugin string `json:"plugin"`
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// Counters tallies denials per plugin and reason. It holds no hosts or payload,
// so its size is bounded by plugins times reasons.
type Counters struct {
	mu sync.Mutex
	m  map[[2]string]int64
}

// NewCounters returns an empty tally.
func NewCounters() *Counters { return &Counters{m: map[[2]string]int64{}} }

// Record adds one denial. An unknown caller is filed under an empty plugin name.
func (c *Counters) Record(plugin, reason string) {
	c.mu.Lock()
	c.m[[2]string{plugin, reason}]++
	c.mu.Unlock()
}

// Snapshot returns the tally sorted by plugin then reason.
func (c *Counters) Snapshot() []DenialCount {
	c.mu.Lock()
	out := make([]DenialCount, 0, len(c.m))
	for k, n := range c.m {
		out = append(out, DenialCount{Plugin: k[0], Reason: k[1], Count: n})
	}
	c.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Plugin != out[j].Plugin {
			return out[i].Plugin < out[j].Plugin
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

// Handler serves the tally as JSON on the policy socket only.
func (c *Counters) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c.Snapshot())
	})
}
