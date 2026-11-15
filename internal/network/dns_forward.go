package network

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/sync/singleflight"
)

// loopTagCode is a private-use EDNS0 option; its payload is the id of the node that forwarded the query.
const loopTagCode uint16 = 65001

// Host resolver files, in preference order. The systemd one lists the real upstreams, not the stub.
var defaultResolvFiles = []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"}

// Forwarder errors a caller can tell apart.
var (
	ErrForwardRateLimited = errors.New("network: dns forward rate limited")
	ErrNoUpstream         = errors.New("network: no dns upstream available")
)

// ForwarderConfig wires a Forwarder; every func is read live so settings apply without a restart.
type ForwarderConfig struct {
	NodeID         string
	Upstreams      func() []string
	AttemptTimeout func() time.Duration
	QueryTimeout   func() time.Duration
	CacheBytes     func() int
	CacheMaxTTL    func() time.Duration
	CacheNegTTL    func() time.Duration
	RateQPS        func() int
	RateBurst      func() int
	// Self lists our own listen addresses so the host resolver can never point back at us.
	Self func() []netip.Addr
	// ResolvFiles overrides the host resolver files, for tests.
	ResolvFiles []string
	Logger      *slog.Logger
}

type upstreamState struct{ fails atomic.Int64 }

// Forwarder sends queries outside the mesh zone to the host's upstream resolvers.
type Forwarder struct {
	cfg   ForwarderConfig
	cache *dnsCache
	sf    singleflight.Group

	mu      sync.Mutex
	health  map[string]*upstreamState
	buckets map[netip.Addr]*bucket
	swept   time.Time
	now     func() time.Time

	loopDrops atomic.Int64
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewForwarder builds a forwarder. It starts no goroutine and opens no socket until a query arrives.
func NewForwarder(cfg ForwarderConfig) *Forwarder {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if len(cfg.ResolvFiles) == 0 {
		cfg.ResolvFiles = defaultResolvFiles
	}
	return &Forwarder{
		cfg: cfg, now: time.Now, health: map[string]*upstreamState{}, buckets: map[netip.Addr]*bucket{},
		cache: newDNSCache(cfg.CacheBytes, cfg.CacheMaxTTL, cfg.CacheNegTTL),
	}
}

// LoopDrops counts queries dropped because they carried this node's own loop tag.
func (f *Forwarder) LoopDrops() int64 { return f.loopDrops.Load() }

// IsLoop reports whether req was already forwarded by this node.
func (f *Forwarder) IsLoop(req *dns.Msg) bool {
	opt := req.IsEdns0()
	if opt == nil {
		return false
	}
	for _, o := range opt.Option {
		if l, ok := o.(*dns.EDNS0_LOCAL); ok && l.Code == loopTagCode && string(l.Data) == f.cfg.NodeID {
			f.loopDrops.Add(1)
			return true
		}
	}
	return false
}

// Upstreams lists the resolvers in use now: the configured list, else the host's, minus our own addresses.
func (f *Forwarder) Upstreams() []string {
	var raw []string
	if f.cfg.Upstreams != nil {
		raw = f.cfg.Upstreams()
	}
	if len(raw) == 0 {
		raw = f.hostUpstreams()
	}
	self := map[netip.Addr]bool{}
	if f.cfg.Self != nil {
		for _, a := range f.cfg.Self() {
			self[a] = true
		}
	}
	out := make([]string, 0, len(raw))
	for _, u := range raw {
		hp := u
		if _, _, err := net.SplitHostPort(u); err != nil {
			hp = net.JoinHostPort(strings.Trim(u, "[]"), "53")
		}
		if ap, err := netip.ParseAddrPort(hp); err == nil && self[ap.Addr().Unmap()] {
			continue
		}
		out = append(out, hp)
	}
	return out
}

// hostUpstreams reads the real upstream file when it exists; the loopback stub is only kept
// when /etc/resolv.conf is all there is, because then it is the only way out.
func (f *Forwarder) hostUpstreams() []string {
	for i, path := range f.cfg.ResolvFiles {
		if _, err := os.Stat(path); err != nil {
			continue
		}
		conf, err := dns.ClientConfigFromFile(path)
		if err != nil {
			continue
		}
		realFile := i < len(f.cfg.ResolvFiles)-1
		var out []string
		for _, s := range conf.Servers {
			if ip, err := netip.ParseAddr(s); err == nil && realFile && ip.IsLoopback() {
				continue
			}
			out = append(out, net.JoinHostPort(s, conf.Port))
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

// FallbackAddr is a host upstream a container can use as its second DNS server, or "".
// Docker only accepts a bare address on port 53, and a loopback one is unreachable from a container.
func (f *Forwarder) FallbackAddr() string {
	for _, u := range f.Upstreams() {
		ap, err := netip.ParseAddrPort(u)
		if err != nil || ap.Port() != 53 || ap.Addr().IsLoopback() || ap.Addr().IsUnspecified() {
			continue
		}
		return ap.Addr().Unmap().String()
	}
	return ""
}

// Resolve answers req from cache or upstream. reason says which; guard (may be nil) runs on every
// answer for this caller, so the shared cache only ever holds unfiltered upstream replies.
func (f *Forwarder) Resolve(ctx context.Context, src netip.Addr, req *dns.Msg, guard ResponseGuardFunc) (*dns.Msg, string, error) {
	q := req.Question[0]
	key := cacheKey{name: strings.ToLower(q.Name), qtype: q.Qtype, qclass: q.Qclass}
	if opt := req.IsEdns0(); opt != nil {
		key.do = opt.Do()
	}
	if hit := f.cache.get(key); hit != nil {
		hit.Id = req.Id
		return applyResponseGuard(guard, src, req, hit), ReasonCached, nil
	}
	if !f.allow(src) {
		return nil, ReasonRefused, ErrForwardRateLimited
	}
	sfKey := key.name + "/" + dns.TypeToString[key.qtype] + "/" + key.view
	if key.do {
		sfKey += "/do"
	}
	v, err, _ := f.sf.Do(sfKey, func() (any, error) {
		resp, err := f.exchangeAll(ctx, req)
		if err != nil {
			return nil, err
		}
		f.cache.put(key, resp)
		return resp, nil
	})
	if err != nil {
		return nil, ReasonUpstreamFailed, err
	}
	out := v.(*dns.Msg).Copy()
	out.Id = req.Id
	return applyResponseGuard(guard, src, req, out), ReasonForwarded, nil
}

// applyResponseGuard lets the per-source guard replace resp; a nil guard or result keeps it.
func applyResponseGuard(guard ResponseGuardFunc, src netip.Addr, req, resp *dns.Msg) *dns.Msg {
	if guard == nil {
		return resp
	}
	if g, _ := guard(src, req, resp.Copy()); g != nil {
		g.Id = req.Id
		return g
	}
	return resp
}

// exchangeAll tries upstreams healthiest first within the total query budget.
func (f *Forwarder) exchangeAll(ctx context.Context, req *dns.Msg) (*dns.Msg, error) {
	ups := f.ordered(f.Upstreams())
	if len(ups) == 0 {
		return nil, ErrNoUpstream
	}
	ctx, cancel := context.WithTimeout(ctx, f.cfg.QueryTimeout())
	defer cancel()
	out := f.outbound(req)
	last := ErrNoUpstream
	for _, u := range ups {
		if ctx.Err() != nil {
			break
		}
		resp, err := f.exchangeOne(ctx, out, u)
		st := f.state(u)
		if err != nil {
			st.fails.Add(1)
			last = err
			continue
		}
		// SERVFAIL and REFUSED are answers: one broken domain must not bench the resolver.
		st.fails.Store(0)
		return f.inbound(req, resp), nil
	}
	if ctx.Err() != nil {
		last = ctx.Err()
	}
	return nil, last
}

func (f *Forwarder) exchangeOne(ctx context.Context, out *dns.Msg, upstream string) (*dns.Msg, error) {
	c := &dns.Client{Net: "udp", Timeout: f.cfg.AttemptTimeout()}
	resp, _, err := c.ExchangeContext(ctx, out, upstream)
	if err != nil {
		return nil, err
	}
	if resp.Truncated {
		c.Net = "tcp"
		resp, _, err = c.ExchangeContext(ctx, out, upstream)
	}
	return resp, err
}

// outbound copies req with a fresh id and our loop tag; the client's own EDNS options are not passed on.
func (f *Forwarder) outbound(req *dns.Msg) *dns.Msg {
	out := req.Copy()
	out.Id = dns.Id()
	out.RecursionDesired = true
	keep := out.Extra[:0]
	size, do := uint16(dns.DefaultMsgSize), false
	for _, rr := range out.Extra {
		if o, ok := rr.(*dns.OPT); ok {
			size, do = max(o.UDPSize(), uint16(dns.MinMsgSize)), o.Do()
			continue
		}
		keep = append(keep, rr)
	}
	out.Extra = keep
	out.SetEdns0(size, do)
	out.IsEdns0().Option = append(out.IsEdns0().Option, &dns.EDNS0_LOCAL{Code: loopTagCode, Data: []byte(f.cfg.NodeID)})
	return out
}

// inbound restores the client's id and drops an OPT record the client never asked for.
func (f *Forwarder) inbound(req, resp *dns.Msg) *dns.Msg {
	resp.Id = req.Id
	// Clients check RA before trusting a resolver; we are one for them even when the upstream forgot the bit.
	resp.RecursionAvailable = true
	if req.IsEdns0() == nil {
		keep := resp.Extra[:0]
		for _, rr := range resp.Extra {
			if _, ok := rr.(*dns.OPT); !ok {
				keep = append(keep, rr)
			}
		}
		resp.Extra = keep
	}
	return resp
}

// ordered puts upstreams with fewer recent failures first, keeping configured order among equals.
func (f *Forwarder) ordered(ups []string) []string {
	out := append([]string(nil), ups...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && f.state(out[j]).fails.Load() < f.state(out[j-1]).fails.Load(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func (f *Forwarder) state(u string) *upstreamState {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.health[u]
	if !ok {
		s = &upstreamState{}
		f.health[u] = s
	}
	return s
}

// allow is a per-source token bucket over upstream lookups. Idle buckets are full, so sweeping them loses nothing.
func (f *Forwarder) allow(src netip.Addr) bool {
	qps := f.cfg.RateQPS()
	if qps <= 0 {
		return true
	}
	burst := float64(f.cfg.RateBurst())
	now := f.now()
	f.mu.Lock()
	defer f.mu.Unlock()
	refill := time.Duration(burst / float64(qps) * float64(time.Second))
	if now.Sub(f.swept) >= refill {
		for a, b := range f.buckets {
			if now.Sub(b.last) >= refill {
				delete(f.buckets, a)
			}
		}
		f.swept = now
	}
	b, ok := f.buckets[src]
	if !ok {
		b = &bucket{tokens: burst, last: now}
		f.buckets[src] = b
	}
	b.tokens = min(burst, b.tokens+now.Sub(b.last).Seconds()*float64(qps))
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}
