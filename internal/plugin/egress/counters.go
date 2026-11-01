package egress

import (
	"encoding/json"
	"net/http"
	"sort"
	"sync"
	"time"
)

// DenialCount is how often one plugin was refused for one reason.
type DenialCount struct {
	Plugin string `json:"plugin"`
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
	// LastDenied is the UTC time of the latest refusal (RFC 3339).
	LastDenied string `json:"last_denied,omitempty"`
}

type tally struct {
	n    int64
	last time.Time
}

// Counters tallies denials per plugin and reason. It holds no hosts or payload,
// so its size is bounded by plugins times reasons.
type Counters struct {
	mu sync.Mutex
	m  map[[2]string]tally
}

// NewCounters returns an empty tally.
func NewCounters() *Counters { return &Counters{m: map[[2]string]tally{}} }

// Record adds one denial. An unknown caller is filed under an empty plugin name.
func (c *Counters) Record(plugin, reason string) {
	c.mu.Lock()
	k := [2]string{plugin, reason}
	t := c.m[k]
	t.n++
	t.last = time.Now().UTC()
	c.m[k] = t
	c.mu.Unlock()
}

// Snapshot returns the tally sorted by plugin then reason.
func (c *Counters) Snapshot() []DenialCount {
	c.mu.Lock()
	out := make([]DenialCount, 0, len(c.m))
	for k, t := range c.m {
		out = append(out, DenialCount{Plugin: k[0], Reason: k[1], Count: t.n, LastDenied: t.last.Format(time.RFC3339)})
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
