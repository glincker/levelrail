package edgefw

import (
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Outcome classifies what the firewall did with one request.
type Outcome int

// Outcomes, in the order their counters are stored.
const (
	OutcomeAllowed Outcome = iota
	OutcomeDenied
	OutcomeChallenged
	OutcomeLogged
	OutcomeRateLimited
	OutcomeCount
)

var outcomeNames = [OutcomeCount]string{"allowed", "denied", "challenged", "logged", "rate_limited"}

// String is the outcome's stable wire name.
func (o Outcome) String() string {
	if o < 0 || o >= OutcomeCount {
		return "unknown"
	}
	return outcomeNames[o]
}

// Reason says which mechanism produced an outcome.
type Reason int

// Reasons, in the order their counters are stored.
const (
	ReasonNone Reason = iota
	ReasonRule
	ReasonRateRule
	ReasonBan
	ReasonBot
	ReasonAIBot
	ReasonAttack
	ReasonShieldRate
	ReasonShieldConcurrency
	ReasonShieldBot
	ReasonShieldBody
	ReasonCount
)

var reasonNames = [ReasonCount]string{"none", "rule", "rate_rule", "ban", "bot", "ai_bot", "attack_mode", "shield_rate", "shield_concurrency", "shield_bot", "shield_body"}

// String is the reason's stable wire name.
func (r Reason) String() string {
	if r < 0 || r >= ReasonCount {
		return "unknown"
	}
	return reasonNames[r]
}

const (
	ringMinutes = 1440
	ringHours   = 24
	sparkSlots  = 6
	slotSeconds = 600
	ruleMinutes = 60
)

type minuteBucket struct {
	min int64
	c   [OutcomeCount]uint32
}

type hourBucket struct {
	hour int64
	c    [ReasonCount]uint32
}

// Observation is one request's final classification.
type Observation struct {
	At      time.Time
	Outcome Outcome
	Reason  Reason
	IP      netip.Addr
	UA      string
	Path    string
	Host    string
}

type scope struct {
	mu      sync.Mutex
	minutes [ringMinutes]minuteBucket
	hours   [ringHours]hourBucket
	top     *topSet
}

func (s *scope) record(o Observation, withTop bool, capacity int) {
	m := o.At.Unix() / 60
	h := o.At.Unix() / 3600
	s.mu.Lock()
	defer s.mu.Unlock()
	b := &s.minutes[m%ringMinutes]
	if b.min != m {
		*b = minuteBucket{min: m}
	}
	b.c[o.Outcome]++
	if o.Reason != ReasonNone {
		hb := &s.hours[h%ringHours]
		if hb.hour != h {
			*hb = hourBucket{hour: h}
		}
		hb.c[o.Reason]++
	}
	if withTop {
		if s.top == nil {
			s.top = newTopSet(capacity)
		}
		s.top.add(o)
	}
}

// SeriesPoint is one chart bucket.
type SeriesPoint struct {
	Time   time.Time            `json:"time"`
	Counts [OutcomeCount]uint64 `json:"counts"`
}

// ReasonCountRow is the number of requests a reason produced.
type ReasonCountRow struct {
	Reason string `json:"reason"`
	Count  uint64 `json:"count"`
}

// RuleHit is one rule's counters.
type RuleHit struct {
	Total uint64
	Last  time.Time
	Spark [ruleMinutes / 5]uint32
}

type ruleCounter struct {
	mu      sync.Mutex
	total   uint64
	last    time.Time
	buckets [ruleMinutes]struct {
		min int64
		n   uint32
	}
}

// Stats keeps the bounded, in-memory firewall visibility: per-minute outcome
// counters for 24 hours, per-hour reason counters, per-rule hit rings and a
// rolling hour of approximate top-N tables. No URL, query string or client
// address is kept outside the capped top tables.
type Stats struct {
	lim    Limits
	mu     sync.RWMutex
	global scope
	hosts  map[string]*scope
	rules  map[string]*ruleCounter
	tick   atomic.Uint64
}

// NewStats builds empty stats.
func NewStats(l Limits) *Stats {
	return &Stats{lim: l, hosts: make(map[string]*scope), rules: make(map[string]*ruleCounter)}
}

// SetLimits replaces the capacities used for new tables.
func (s *Stats) SetLimits(l Limits) {
	s.mu.Lock()
	s.lim = l
	s.mu.Unlock()
}

// Observe records one request under the global scope and, within the host
// cap, under its own host.
func (s *Stats) Observe(o Observation) {
	n := s.tick.Add(1)
	s.mu.RLock()
	lim := s.lim
	h := s.hosts[o.Host]
	s.mu.RUnlock()
	withTop := lim.TopSample <= 1 || n%uint64(lim.TopSample) == 0
	if h == nil && o.Host != "" {
		s.mu.Lock()
		if h = s.hosts[o.Host]; h == nil && len(s.hosts) < lim.MaxStatScopes {
			h = &scope{}
			s.hosts[o.Host] = h
		}
		s.mu.Unlock()
	}
	s.global.record(o, withTop, lim.TopCapacity)
	if h != nil {
		h.record(o, withTop, lim.TopCapacity)
	}
}

// RuleMatched counts one match of rule id.
func (s *Stats) RuleMatched(id string, at time.Time) {
	s.mu.RLock()
	c := s.rules[id]
	s.mu.RUnlock()
	if c == nil {
		s.mu.Lock()
		if c = s.rules[id]; c == nil {
			c = &ruleCounter{}
			s.rules[id] = c
		}
		s.mu.Unlock()
	}
	m := at.Unix() / 60
	c.mu.Lock()
	c.total++
	c.last = at
	b := &c.buckets[m%ruleMinutes]
	if b.min != m {
		b.min, b.n = m, 0
	}
	b.n++
	c.mu.Unlock()
}

// PruneRules drops counters of rules that no longer exist.
func (s *Stats) PruneRules(keep map[string]bool) {
	s.mu.Lock()
	for id := range s.rules {
		if !keep[id] {
			delete(s.rules, id)
		}
	}
	s.mu.Unlock()
}

// RuleHits returns the counters for id: total since start, last hit and a
// sparkline of the last hour in 5 minute steps (oldest first).
func (s *Stats) RuleHits(id string, now time.Time) RuleHit {
	s.mu.RLock()
	c := s.rules[id]
	s.mu.RUnlock()
	var out RuleHit
	if c == nil {
		return out
	}
	m := now.Unix() / 60
	c.mu.Lock()
	defer c.mu.Unlock()
	out.Total, out.Last = c.total, c.last
	for back := int64(0); back < ruleMinutes; back++ {
		b := c.buckets[(m-back)%ruleMinutes]
		if b.min == m-back {
			out.Spark[len(out.Spark)-1-int(back/5)] += b.n
		}
	}
	return out
}

func (s *Stats) scopeFor(host string) *scope {
	if host == "" {
		return &s.global
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hosts[host]
}

// Series returns outcome counts between from and to in at most points
// buckets, each a whole number of minutes.
func (s *Stats) Series(host string, from, to time.Time, points int) []SeriesPoint {
	sc := s.scopeFor(host)
	if sc == nil || !to.After(from) {
		return []SeriesPoint{}
	}
	if oldest := to.Add(-ringMinutes * time.Minute); from.Before(oldest) {
		from = oldest
	}
	if points < 1 {
		points = 60
	}
	span := int64(to.Sub(from) / time.Minute)
	step := max(1, (span+int64(points)-1)/int64(points))
	startMin := from.Unix() / 60
	endMin := to.Unix() / 60
	out := make([]SeriesPoint, 0, int(span/step)+1)
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for m := startMin; m <= endMin; m += step {
		p := SeriesPoint{Time: time.Unix(m*60, 0).UTC()}
		for k := m; k < m+step && k <= endMin; k++ {
			if b := sc.minutes[k%ringMinutes]; b.min == k {
				for i := range p.Counts {
					p.Counts[i] += uint64(b.c[i])
				}
			}
		}
		out = append(out, p)
	}
	return out
}

// Totals sums the outcome counts between from and to.
func (s *Stats) Totals(host string, from, to time.Time) [OutcomeCount]uint64 {
	var t [OutcomeCount]uint64
	for _, p := range s.Series(host, from, to, ringMinutes) {
		for i, n := range p.Counts {
			t[i] += n
		}
	}
	return t
}

// Reasons sums reason counters over whole hours overlapping from..to.
func (s *Stats) Reasons(host string, from, to time.Time) []ReasonCountRow {
	sc := s.scopeFor(host)
	out := []ReasonCountRow{}
	if sc == nil {
		return out
	}
	var sum [ReasonCount]uint64
	sc.mu.Lock()
	for h := from.Unix() / 3600; h <= to.Unix()/3600; h++ {
		if b := sc.hours[h%ringHours]; b.hour == h {
			for i, n := range b.c {
				sum[i] += uint64(n)
			}
		}
	}
	sc.mu.Unlock()
	for i := 1; i < int(ReasonCount); i++ {
		if sum[i] > 0 {
			out = append(out, ReasonCountRow{Reason: Reason(i).String(), Count: sum[i]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	return out
}

// Dimension names a top table.
type Dimension string

// Top table dimensions.
const (
	DimIP       Dimension = "ip"
	DimDeniedIP Dimension = "denied_ip"
	DimUA       Dimension = "user_agent"
	DimPath     Dimension = "path"
	DimHost     Dimension = "host"
)

// TopRow is one approximate top-N entry with a six point sparkline.
type TopRow struct {
	Key   string             `json:"key"`
	Count uint64             `json:"count"`
	Spark [sparkSlots]uint32 `json:"spark"`
}

// Top returns the busiest keys of dim over the last hour (the retained
// window), most first.
func (s *Stats) Top(host string, dim Dimension, now time.Time, n int) []TopRow {
	sc := s.scopeFor(host)
	if sc == nil {
		return []TopRow{}
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.top == nil {
		return []TopRow{}
	}
	return sc.top.rows(dim, now, n)
}

const routeDepth = 3

// normalizePath reduces a path to a route template of at most routeDepth
// segments, with identifier-looking segments replaced so tokens and ids are
// never kept.
func normalizePath(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return "/"
	}
	var b strings.Builder
	for i := 0; i < routeDepth; i++ {
		seg, rest, more := strings.Cut(p, "/")
		b.WriteByte('/')
		b.WriteString(routeSegment(seg))
		if !more {
			return b.String()
		}
		p = rest
	}
	b.WriteString("/*")
	return b.String()
}

func routeSegment(s string) string {
	if len(s) > 24 {
		return ":id"
	}
	digits, allHex := 0, true
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case r >= 'a' && r <= 'f', r >= 'A' && r <= 'F', r == '-', r == '_', r == '.':
		default:
			allHex = false
		}
	}
	switch {
	case digits == len(s) && s != "":
		return ":id"
	case allHex && len(s) >= 8 && digits > 0:
		return ":id"
	case digits >= 3 && digits*3 >= len(s):
		return ":id"
	}
	return s
}

func clip(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return strings.Clone(s)
}
