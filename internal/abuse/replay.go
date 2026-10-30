package abuse

import (
	"net/netip"
	"sort"
	"time"
)

// Observation is one past request as the replay needs it.
type Observation struct {
	Time      time.Time
	Client    netip.Addr
	UserAgent string
}

// ReasonTotal pairs a reason name with how many requests it covers.
type ReasonTotal struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

// ClientImpact is one client's share of a replay.
type ClientImpact struct {
	Client     string `json:"client"`
	Requests   int    `json:"requests"`
	WouldBlock int    `json:"would_block"`
	// TopReason is the reason that blocked this client most often.
	TopReason string `json:"top_reason"`
}

// ReplayResult is what a policy would have done to a stretch of past traffic.
// Nothing was blocked or changed to produce it.
type ReplayResult struct {
	Requests   int            `json:"requests"`
	WouldBlock int            `json:"would_block"`
	Percent    float64        `json:"percent"`
	ByReason   []ReasonTotal  `json:"by_reason"`
	TopClients []ClientImpact `json:"top_clients"`
	From       time.Time      `json:"from"`
	To         time.Time      `json:"to"`
}

// Replay runs obs, oldest first, through a fresh engine for p on a clock that
// follows the recorded times, and reports what it would have blocked. It uses
// private state, so the live limiter and counters are untouched.
func Replay(p Policy, obs []Observation, l Limits, top int) (ReplayResult, error) {
	var now int64
	st := &State{Limiter: NewLimiter(l.MaxTrackedClients, l.Shards, l.IdleEvict, func() int64 { return now }), Counters: &Counters{}}
	p.Mode = ModeLogOnly
	eng, err := NewEngine(p, st, l)
	if err != nil {
		return ReplayResult{}, err
	}
	type tally struct {
		requests, blocked int
		byReason          [ReasonCount]int
	}
	clients := make(map[netip.Addr]*tally)
	var byReason [ReasonCount]int
	res := ReplayResult{ByReason: []ReasonTotal{}, TopClients: []ClientImpact{}}
	var base time.Time
	for _, o := range obs {
		if base.IsZero() {
			base, res.From = o.Time, o.Time
		}
		now = int64(o.Time.Sub(base))
		res.To = o.Time
		res.Requests++
		t := clients[o.Client]
		if t == nil && o.Client.IsValid() {
			t = &tally{}
			clients[o.Client] = t
		}
		if t != nil {
			t.requests++
		}
		d := eng.Decide(o.Client, o.UserAgent)
		if d.Action != ActionWouldBlock {
			continue
		}
		res.WouldBlock++
		byReason[d.Reason]++
		if t != nil {
			t.blocked++
			t.byReason[d.Reason]++
		}
	}
	if res.Requests > 0 {
		res.Percent = float64(res.WouldBlock) / float64(res.Requests) * 100
	}
	for r := Reason(0); r < ReasonCount; r++ {
		res.ByReason = append(res.ByReason, ReasonTotal{Reason: r.String(), Count: byReason[r]})
	}
	for a, t := range clients {
		if t.blocked == 0 {
			continue
		}
		best := Reason(0)
		for r := Reason(1); r < ReasonCount; r++ {
			if t.byReason[r] > t.byReason[best] {
				best = r
			}
		}
		res.TopClients = append(res.TopClients, ClientImpact{Client: a.String(), Requests: t.requests, WouldBlock: t.blocked, TopReason: best.String()})
	}
	sort.Slice(res.TopClients, func(i, j int) bool {
		a, b := res.TopClients[i], res.TopClients[j]
		if a.WouldBlock != b.WouldBlock {
			return a.WouldBlock > b.WouldBlock
		}
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return a.Client < b.Client
	})
	if top > 0 && len(res.TopClients) > top {
		res.TopClients = res.TopClients[:top]
	}
	return res, nil
}

// Suggestion is a starting policy derived from the app's recent traffic.
type Suggestion struct {
	PeakRPS float64 `json:"peak_rps"`
	RPS     float64 `json:"rps"`
	Burst   int     `json:"burst"`
	// FromTraffic is false when there was no traffic and the instance
	// defaults were used.
	FromTraffic bool `json:"from_traffic"`
}

// Suggest sizes a per-client limit from the app's peak request rate: one
// client may use a configured share of the peak, never less than the default
// rate, so ordinary visitors are not caught by a quiet-hour measurement.
func Suggest(peakRPS float64, l Limits) Suggestion {
	s := Suggestion{PeakRPS: peakRPS, RPS: l.DefaultRPS, Burst: l.DefaultBurst}
	if peakRPS <= 0 {
		return s
	}
	rps := peakRPS * l.SuggestShare
	if rps < l.DefaultRPS {
		rps = l.DefaultRPS
	}
	if rps > l.MaxRPS {
		rps = l.MaxRPS
	}
	s.FromTraffic = true
	s.RPS = float64(int(rps + 0.5))
	s.Burst = min(max(int(s.RPS*l.SuggestBurstSeconds), 1), l.MaxBurst)
	return s
}
