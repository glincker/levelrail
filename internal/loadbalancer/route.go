package loadbalancer

import (
	"strconv"
	"time"
)

// Upstream is one replica endpoint the balancer can send traffic to.
type Upstream struct {
	// ID is stable across passes (service name plus replica index), used to
	// track first-seen time for slow start.
	ID      string `json:"id"`
	Dial    string `json:"dial"`
	Replica int    `json:"replica"`
	// NodeID is empty for the control plane's own node.
	NodeID string `json:"node_id,omitempty"`
}

// EffectiveWeights returns one weight per upstream. When slow start is set,
// an upstream first seen less than SlowStart ago ramps linearly from 1 to its
// configured weight. Only meaningful for the weighted algorithm.
func EffectiveWeights(cfg Config, ups []Upstream, firstSeen map[string]time.Time, now time.Time) []int {
	slow := ParseDuration(cfg.SlowStart)
	out := make([]int, len(ups))
	for i, u := range ups {
		w := 1
		if u.Replica < len(cfg.Weights) && cfg.Weights[u.Replica] > 0 {
			w = cfg.Weights[u.Replica]
		}
		if seen, ok := firstSeen[u.ID]; ok && slow > 0 && now.Sub(seen) < slow {
			frac := float64(now.Sub(seen)) / float64(slow)
			ramped := 1 + int(float64(w-1)*frac)
			if ramped < w {
				w = ramped
			}
		}
		out[i] = w
	}
	return out
}

// UpstreamID builds the stable ID for a service replica.
func UpstreamID(service string, replica int) string {
	return service + "#" + strconv.Itoa(replica)
}
