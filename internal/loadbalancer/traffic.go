package loadbalancer

import "time"

// TrafficTotals are cumulative ingress counters for one upstream dial address.
// Latency holds histogram bucket counts, the last slot being overflow.
type TrafficTotals struct {
	Requests uint64
	Errors   uint64
	Latency  []uint64
}

// TrafficSource returns cumulative per-dial counters since process start.
type TrafficSource func() map[string]TrafficTotals

// UpstreamTraffic is the recent request activity of one upstream.
type UpstreamTraffic struct {
	WindowSeconds  float64   `json:"window_seconds"`
	RequestsPerSec float64   `json:"requests_per_sec"`
	ErrorRate      float64   `json:"error_rate"`
	P50Ms          float64   `json:"p50_ms"`
	P95Ms          float64   `json:"p95_ms"`
	RPSSeries      []float64 `json:"rps_series"`
}

type trafficSnap struct {
	at time.Time
	t  TrafficTotals
}

type trafficState struct {
	src    TrafficSource
	bounds []float64
	snaps  map[string]map[string][]trafficSnap
	last   map[string]time.Time
}

func (s *trafficState) retain(keep map[string]bool) {
	for svc := range s.snaps {
		if !keep[svc] {
			delete(s.snaps, svc)
			delete(s.last, svc)
		}
	}
}

// SetTrafficSource wires the ingress counters that feed per-upstream traffic.
// boundsMS are the latency bucket upper bounds matching TrafficTotals.Latency.
func (r *Registry) SetTrafficSource(src TrafficSource, boundsMS []float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.traffic.src = src
	r.traffic.bounds = append([]float64(nil), boundsMS...)
}

// recordTraffic samples the counters at most once per series step and fills
// Traffic on each upstream. Callers hold r.mu.
func (r *Registry) recordTraffic(st *Status, now time.Time) {
	ts := &r.traffic
	if ts.src == nil {
		return
	}
	if ts.snaps == nil {
		ts.snaps = map[string]map[string][]trafficSnap{}
		ts.last = map[string]time.Time{}
	}
	prev := ts.snaps[st.Service]
	next := make(map[string][]trafficSnap, len(st.Upstreams))
	sample := ts.last[st.Service].IsZero() || now.Sub(ts.last[st.Service]) >= r.histCfg.SeriesStep
	var totals map[string]TrafficTotals
	if sample {
		totals = ts.src()
		ts.last[st.Service] = now
	}
	window := r.histCfg.TrafficWindow
	for i := range st.Upstreams {
		u := &st.Upstreams[i]
		snaps := prev[u.ID]
		if t, ok := totals[u.Dial]; ok && sample {
			snaps = appendSnap(snaps, trafficSnap{at: now, t: t}, now.Add(-window))
		}
		next[u.ID] = snaps
		u.Traffic = computeTraffic(snaps, ts.bounds)
	}
	ts.snaps[st.Service] = next
}

// appendSnap adds s, dropping snapshots older than cutoff (keeping one just
// before it as the window baseline) and anything before a counter reset.
func appendSnap(snaps []trafficSnap, s trafficSnap, cutoff time.Time) []trafficSnap {
	if n := len(snaps); n > 0 && s.t.Requests < snaps[n-1].t.Requests {
		snaps = nil
	}
	snaps = append(snaps, s)
	keep := 0
	for i := 1; i < len(snaps); i++ {
		if snaps[i].at.After(cutoff) {
			break
		}
		keep = i
	}
	return append([]trafficSnap(nil), snaps[keep:]...)
}

// computeTraffic derives rates from the oldest to the newest snapshot. It
// returns nil when fewer than two snapshots or no elapsed time exist.
func computeTraffic(snaps []trafficSnap, bounds []float64) *UpstreamTraffic {
	if len(snaps) < 2 {
		return nil
	}
	first, last := snaps[0], snaps[len(snaps)-1]
	secs := last.at.Sub(first.at).Seconds()
	if secs <= 0 {
		return nil
	}
	reqs := last.t.Requests - first.t.Requests
	out := &UpstreamTraffic{WindowSeconds: secs, RequestsPerSec: float64(reqs) / secs, RPSSeries: []float64{}}
	if reqs > 0 {
		out.ErrorRate = float64(last.t.Errors-first.t.Errors) / float64(reqs)
		counts := make([]float64, len(last.t.Latency))
		for i := range counts {
			var base uint64
			if i < len(first.t.Latency) {
				base = first.t.Latency[i]
			}
			counts[i] = float64(last.t.Latency[i] - base)
		}
		out.P50Ms = bucketQuantile(counts, bounds, 0.5)
		out.P95Ms = bucketQuantile(counts, bounds, 0.95)
	}
	for i := 1; i < len(snaps); i++ {
		dt := snaps[i].at.Sub(snaps[i-1].at).Seconds()
		if dt > 0 {
			out.RPSSeries = append(out.RPSSeries, float64(snaps[i].t.Requests-snaps[i-1].t.Requests)/dt)
		}
	}
	return out
}

// bucketQuantile interpolates the q-quantile inside the containing bucket;
// the overflow bucket reports the last finite bound.
func bucketQuantile(counts, bounds []float64, q float64) float64 {
	var total float64
	for _, c := range counts {
		total += c
	}
	if total <= 0 || len(bounds) == 0 {
		return 0
	}
	rank := q * total
	var cum float64
	for i, c := range counts {
		if c <= 0 {
			continue
		}
		if cum+c < rank {
			cum += c
			continue
		}
		if i >= len(bounds) {
			break
		}
		lower := 0.0
		if i > 0 {
			lower = bounds[i-1]
		}
		return lower + (bounds[i]-lower)*((rank-cum)/c)
	}
	return bounds[len(bounds)-1]
}
