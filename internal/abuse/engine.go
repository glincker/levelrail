package abuse

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Action is what the caller should do with a request.
type Action int

// Actions. WouldBlock means log_only saw a violation and let the request through.
const (
	ActionAllow Action = iota
	ActionWouldBlock
	ActionBlock
)

// Decision is the engine's verdict on one request.
type Decision struct {
	Action Action
	Reason Reason
	// RetryAfter is whole seconds, at least 1, set only for rate limit blocks.
	RetryAfter int
}

// Engine evaluates one compiled policy. It is immutable after NewEngine and
// safe for concurrent use; the limiter and counters it points at are shared
// across policy reloads so a reload never hands clients a fresh bucket.
type Engine struct {
	mode        Mode
	rps         float64
	burst       int
	exact       map[netip.Addr]struct{}
	v4          []v4Net
	v6          []netip.Prefix
	agents      [][]byte
	requireUA   bool
	maxRetry    int
	limiter     *Limiter
	counters    *Counters
	checkDenies bool
}

// NewEngine compiles p against the shared state st.
func NewEngine(p Policy, st *State, l Limits) (*Engine, error) {
	p = p.Normalize(l)
	if err := p.Validate(l); err != nil {
		return nil, fmt.Errorf("abuse: compile policy: %w", err)
	}
	e := &Engine{
		mode: p.Mode, rps: p.RPS, burst: p.Burst, requireUA: p.RequireUserAgent,
		limiter: st.Limiter, counters: st.Counters, maxRetry: max(int(l.MaxRetryAfter/time.Second), 1),
	}
	for _, s := range p.DenyCIDRs {
		pf, err := ParseDenyEntry(s)
		if err != nil {
			return nil, err
		}
		if pf.IsSingleIP() {
			if e.exact == nil {
				e.exact = make(map[netip.Addr]struct{})
			}
			e.exact[pf.Addr()] = struct{}{}
			continue
		}
		if pf.Addr().Is4() {
			ab := pf.Addr().As4()
			mask := ^uint32(0) << (32 - pf.Bits())
			e.v4 = append(e.v4, v4Net{net: binary.BigEndian.Uint32(ab[:]) & mask, mask: mask})
			continue
		}
		e.v6 = append(e.v6, pf)
	}
	for _, a := range p.DenyUserAgents {
		e.agents = append(e.agents, []byte(strings.ToLower(a)))
	}
	e.checkDenies = len(e.exact) > 0 || len(e.v4) > 0 || len(e.v6) > 0
	return e, nil
}

// Decide judges one request. client may be invalid (address unknown), in which
// case only the User-Agent checks apply. The allow path takes at most one
// shard lock and allocates nothing.
func (e *Engine) Decide(client netip.Addr, ua string) Decision {
	if e.mode != ModeLogOnly && e.mode != ModeEnforce {
		return Decision{}
	}
	var h uint64
	known := client.IsValid()
	if known {
		h = hashAddr(client)
	}
	reason, retry, violated := e.violation(client, known, ua)
	st := e.counters.at(h)
	if !violated {
		st.allowed.Add(1)
		return Decision{}
	}
	if e.mode == ModeLogOnly {
		st.wouldBlock[reason].Add(1)
		return Decision{Action: ActionWouldBlock, Reason: reason}
	}
	st.blocked[reason].Add(1)
	return Decision{Action: ActionBlock, Reason: reason, RetryAfter: retry}
}

// violation checks the cheap stateless rules first so a denied client never
// spends a rate token.
func (e *Engine) violation(client netip.Addr, known bool, ua string) (reason Reason, retry int, violated bool) {
	if known && e.checkDenies && e.denied(client) {
		return ReasonDenyCIDR, 0, true
	}
	if e.requireUA && ua == "" {
		return ReasonMissingUserAgent, 0, true
	}
	if ua != "" && len(e.agents) > 0 && e.agentDenied(ua) {
		return ReasonDenyUserAgent, 0, true
	}
	if known && e.rps > 0 {
		if ok, wait := e.limiter.Allow(client, e.rps, e.burst); !ok {
			secs := int((wait + time.Second - 1) / time.Second)
			return ReasonRateLimit, min(max(secs, 1), e.maxRetry), true
		}
	}
	return 0, 0, false
}

// agentBufSize is the stack buffer the User-Agent is lower-cased into; a longer
// header takes the slower allocation-free scan so padding cannot hide a token.
const agentBufSize = 512

func (e *Engine) agentDenied(ua string) bool {
	if len(ua) > agentBufSize {
		for _, a := range e.agents {
			if containsFold(ua, a) {
				return true
			}
		}
		return false
	}
	var buf [agentBufSize]byte
	lower := buf[:len(ua)]
	for i := 0; i < len(ua); i++ {
		c := ua[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		lower[i] = c
	}
	for _, a := range e.agents {
		if bytes.Contains(lower, a) {
			return true
		}
	}
	return false
}

func (e *Engine) denied(a netip.Addr) bool {
	if _, ok := e.exact[a]; ok {
		return true
	}
	if a.Is4() {
		ab := a.As4()
		v := binary.BigEndian.Uint32(ab[:])
		for _, n := range e.v4 {
			if v&n.mask == n.net {
				return true
			}
		}
		return false
	}
	for _, p := range e.v6 {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// v4Net is an IPv4 range as integers, so the hot path compares with one AND.
type v4Net struct{ net, mask uint32 }

// containsFold reports whether s contains lowerSub, ignoring ASCII case, with
// no allocation. lowerSub must already be lower case.
func containsFold(s string, lowerSub []byte) bool {
	n := len(lowerSub)
	if n == 0 {
		return true
	}
	for i := 0; i+n <= len(s); i++ {
		j := 0
		for ; j < n; j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != lowerSub[j] {
				break
			}
		}
		if j == n {
			return true
		}
	}
	return false
}
