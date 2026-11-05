package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

// HostLookup is the part of net.Resolver the hub endpoint needs; tests fake it.
type HostLookup interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// HubHostFailure says why a hostname could not become a mesh endpoint IP.
type HubHostFailure string

// Reasons a hub hostname fails to resolve into a usable endpoint.
const (
	HubHostNotFound   HubHostFailure = "not_found"
	HubHostTimeout    HubHostFailure = "timeout"
	HubHostNoAddress  HubHostFailure = "no_address"
	HubHostPrivate    HubHostFailure = "private"
	HubHostLookupFail HubHostFailure = "lookup_failed"
)

// HubHostError is the typed failure of a hub hostname resolution.
type HubHostError struct {
	Host   string
	Reason HubHostFailure
	// Resolved is the unusable address when Reason is HubHostPrivate.
	Resolved netip.Addr
	Err      error
}

func (e *HubHostError) Error() string {
	switch e.Reason {
	case HubHostNotFound:
		return fmt.Sprintf("%s does not exist in DNS (NXDOMAIN)", e.Host)
	case HubHostTimeout:
		return fmt.Sprintf("the DNS lookup for %s timed out", e.Host)
	case HubHostNoAddress:
		return fmt.Sprintf("%s has no A or AAAA record", e.Host)
	case HubHostPrivate:
		return fmt.Sprintf("%s resolved to %s, a private, loopback or link-local address; for a LAN mesh set the private IP itself as the advertise address (an IP literal is accepted unchanged), otherwise use a public name", e.Host, e.Resolved)
	}
	return fmt.Sprintf("the DNS lookup for %s failed: %v", e.Host, e.Err)
}

func (e *HubHostError) Unwrap() error { return e.Err }

// HubResolution is the endpoint IP for a hub host. Stale means the last good
// IP is returned because the latest lookup failed.
type HubResolution struct {
	Host       string
	IP         netip.Addr
	Stale      bool
	ResolvedAt time.Time
	// LastGood is the last working answer even when it is too old to use.
	LastGood   netip.Addr
	LastGoodAt time.Time
}

// HubHostConfig wires a HubHostResolver; TTL and Timeout are read on every
// call so a runtime setting change applies live.
type HubHostConfig struct {
	Lookup  HostLookup
	TTL     func() time.Duration
	Timeout func() time.Duration
	// StaleMax bounds how long the last good IP outlives failed lookups.
	StaleMax func() time.Duration
	Now      func() time.Time
}

// HubHostResolver turns the advertise host into a WireGuard-usable IP and
// remembers the last good answer so one failed lookup never drops a mesh.
type HubHostResolver struct {
	cfg HubHostConfig

	mu       sync.Mutex
	host     string
	at       time.Time
	cur      HubResolution
	curErr   error
	lastGood netip.Addr
	goodAt   time.Time
	// gen bumps on a host change; seq orders lookups so an older one never overwrites a newer.
	gen, seq, applied uint64
}

// NewHubHostResolver fills defaults: the system resolver and the real clock.
func NewHubHostResolver(cfg HubHostConfig) *HubHostResolver {
	if cfg.Lookup == nil {
		cfg.Lookup = net.DefaultResolver
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &HubHostResolver{cfg: cfg}
}

// Bounds applied inside the resolver so a bad env value can never disable
// caching or keep a dead IP forever.
const (
	MinHubTTL          = 30 * time.Second
	MaxHubTTL          = 24 * time.Hour
	DefaultHubTTL      = 5 * time.Minute
	MinHubStaleMax     = 5 * time.Minute
	MaxHubStaleMax     = 30 * 24 * time.Hour
	DefaultHubStaleMax = 24 * time.Hour
)

func clampDuration(f func() time.Duration, def, lo, hi time.Duration) time.Duration {
	d := def
	if f != nil {
		d = f()
	}
	return min(max(d, lo), hi)
}

// TTL is how long an answer is reused, clamped to the registered range.
func (r *HubHostResolver) TTL() time.Duration {
	return clampDuration(r.cfg.TTL, DefaultHubTTL, MinHubTTL, MaxHubTTL)
}

// Now is the resolver's clock.
func (r *HubHostResolver) Now() time.Time { return r.cfg.Now() }

// StaleMax is how long the last good IP outlives failed lookups.
func (r *HubHostResolver) StaleMax() time.Duration {
	return clampDuration(r.cfg.StaleMax, DefaultHubStaleMax, MinHubStaleMax, MaxHubStaleMax)
}

// Resolve returns the IP for host. An IP literal comes back unchanged. On a
// failed lookup the error is non-nil and res.IP still holds the last good IP
// (Stale) while it is younger than StaleMax, so callers keep the working endpoint.
func (r *HubHostResolver) Resolve(ctx context.Context, host string) (HubResolution, error) {
	if a, err := netip.ParseAddr(host); err == nil {
		return HubResolution{Host: host, IP: a.Unmap(), ResolvedAt: r.cfg.Now()}, nil
	}
	ttl := r.TTL()
	r.mu.Lock()
	if r.host == host && r.cfg.Now().Sub(r.at) < ttl {
		res, err := r.cur, r.curErr
		r.mu.Unlock()
		return res, err
	}
	if r.host != host {
		// A changed host never inherits the previous host's last good IP.
		r.host, r.lastGood, r.cur, r.curErr = host, netip.Addr{}, HubResolution{}, nil
		r.gen++
	}
	gen := r.gen
	r.seq++
	mine := r.seq
	r.mu.Unlock()

	lctx := ctx
	if r.cfg.Timeout != nil {
		if d := r.cfg.Timeout(); d > 0 {
			var cancel context.CancelFunc
			lctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}
	ip, lookupErr := r.lookup(lctx, host)

	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.cfg.Now()
	if r.gen != gen {
		// The advertise host changed during the lookup: answer the caller, cache nothing.
		if lookupErr == nil {
			return HubResolution{Host: host, IP: ip, ResolvedAt: now}, nil
		}
		return HubResolution{Host: host}, lookupErr
	}
	if mine < r.applied {
		return r.cur, r.curErr
	}
	r.applied, r.at = mine, now
	if lookupErr == nil {
		r.lastGood, r.goodAt = ip, now
		r.cur, r.curErr = HubResolution{Host: host, IP: ip, ResolvedAt: now}, nil
		return r.cur, nil
	}
	r.cur = HubResolution{Host: host, LastGood: r.lastGood, LastGoodAt: r.goodAt}
	if r.lastGood.IsValid() && now.Sub(r.goodAt) <= r.StaleMax() {
		r.cur.IP, r.cur.Stale, r.cur.ResolvedAt = r.lastGood, true, r.goodAt
	}
	r.curErr = lookupErr
	return r.cur, lookupErr
}

// lookup prefers an A record and falls back to AAAA.
func (r *HubHostResolver) lookup(ctx context.Context, host string) (netip.Addr, error) {
	addrs, err := r.cfg.Lookup.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return netip.Addr{}, classifyHubLookup(host, err)
	}
	var v6, unusable netip.Addr
	for _, a := range addrs {
		a = a.Unmap()
		if !publicHubAddr(a) {
			if !unusable.IsValid() {
				unusable = a
			}
			continue
		}
		if a.Is4() {
			return a, nil
		}
		if !v6.IsValid() {
			v6 = a
		}
	}
	if v6.IsValid() {
		return v6, nil
	}
	if unusable.IsValid() {
		return netip.Addr{}, &HubHostError{Host: host, Reason: HubHostPrivate, Resolved: unusable}
	}
	return netip.Addr{}, &HubHostError{Host: host, Reason: HubHostNoAddress}
}

func publicHubAddr(a netip.Addr) bool {
	return a.IsValid() && !a.IsLoopback() && !a.IsPrivate() && !a.IsLinkLocalUnicast() &&
		!a.IsLinkLocalMulticast() && !a.IsMulticast() && !a.IsUnspecified()
}

func classifyHubLookup(host string, err error) error {
	var dnsErr *net.DNSError
	switch {
	case errors.As(err, &dnsErr) && dnsErr.IsNotFound:
		return &HubHostError{Host: host, Reason: HubHostNotFound, Err: err}
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &dnsErr) && dnsErr.IsTimeout:
		return &HubHostError{Host: host, Reason: HubHostTimeout, Err: err}
	}
	return &HubHostError{Host: host, Reason: HubHostLookupFail, Err: err}
}
