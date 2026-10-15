// Package accesspolicy merges per-team session rules into one effective set
// and answers the questions the API middleware asks on every request.
package accesspolicy

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// Policy is one team's rules as stored.
type Policy struct {
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	IPAllowlist []string
	RequireMFA  bool
	// MaxSessions is the per-user cap on concurrent full sessions; 0 means none.
	MaxSessions int
	// RejectOnLimit refuses new sessions over the cap instead of evicting the oldest.
	RejectOnLimit bool
}

// Rules is the strictest combination of every policy that applies to a user.
type Rules struct {
	IdleTimeout time.Duration
	MaxLifetime time.Duration
	// AllowLists holds one list per team that set one; an address must be in every list.
	AllowLists [][]netip.Prefix
	RequireMFA bool
	// MaxSessions is the smallest cap any team sets; 0 means no cap.
	MaxSessions int
	// RejectOnLimit is true when any team that sets a cap chooses reject.
	RejectOnLimit bool
}

// Verdict names why a session is refused; empty means allowed.
type Verdict string

// Refusal reasons, also used as API error codes.
const (
	Allowed            Verdict = ""
	ExpiredIdle        Verdict = "session_idle_timeout"
	ExpiredLifetime    Verdict = "session_max_lifetime"
	AddressNotAllowed  Verdict = "ip_not_allowed"
	MFAEnrollmentNeeds Verdict = "mfa_enrollment_required"
	SessionEvicted     Verdict = "session_evicted"
	SessionLimit       Verdict = "session_limit"
)

// ErrInvalidEntry is returned for an allowlist entry that is neither an IP nor a CIDR.
var ErrInvalidEntry = errors.New("accesspolicy: invalid allowlist entry")

// ParseAllowlist accepts IPv4 or IPv6 addresses and CIDR prefixes.
func ParseAllowlist(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))
	for _, raw := range entries {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		if p, err := netip.ParsePrefix(e); err == nil {
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(e)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidEntry, e)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func stricter(a, b time.Duration) time.Duration {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case b < a:
		return b
	}
	return a
}

// Merge combines policies: the shortest non-zero timeouts, MFA if any team
// requires it, the smallest session cap, and every non-empty allowlist. A bad entry fails the merge.
func Merge(policies []Policy) (Rules, error) {
	var r Rules
	for _, p := range policies {
		r.IdleTimeout = stricter(r.IdleTimeout, p.IdleTimeout)
		r.MaxLifetime = stricter(r.MaxLifetime, p.MaxLifetime)
		r.RequireMFA = r.RequireMFA || p.RequireMFA
		if p.MaxSessions > 0 {
			if r.MaxSessions == 0 || p.MaxSessions < r.MaxSessions {
				r.MaxSessions = p.MaxSessions
			}
			r.RejectOnLimit = r.RejectOnLimit || p.RejectOnLimit
		}
		list, err := ParseAllowlist(p.IPAllowlist)
		if err != nil {
			return Rules{}, err
		}
		if len(list) > 0 {
			r.AllowLists = append(r.AllowLists, list)
		}
	}
	return r, nil
}

// IPAllowed reports whether addr satisfies every allowlist. An invalid
// address never passes a non-empty allowlist.
func (r Rules) IPAllowed(addr netip.Addr) bool {
	addr = addr.Unmap()
	for _, list := range r.AllowLists {
		if !addr.IsValid() {
			return false
		}
		ok := false
		for _, p := range list {
			if p.Contains(addr) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

// SessionAge checks a session's lifetime and idle time. A session never seen
// since sign-in counts as idle from its creation.
func (r Rules) SessionAge(now, created, lastSeen time.Time, seen bool) Verdict {
	if r.MaxLifetime > 0 && now.Sub(created) > r.MaxLifetime {
		return ExpiredLifetime
	}
	if !seen {
		lastSeen = created
	}
	if r.IdleTimeout > 0 && now.Sub(lastSeen) > r.IdleTimeout {
		return ExpiredIdle
	}
	return Allowed
}

// ClientAddr resolves the caller's address: the TCP peer, or when the peer is
// a trusted proxy, the rightmost X-Forwarded-For hop that is not one.
func ClientAddr(peer netip.Addr, forwardedFor string, trusted func(netip.Addr) bool) netip.Addr {
	addr := peer.Unmap()
	if !trusted(addr) || strings.TrimSpace(forwardedFor) == "" {
		return addr
	}
	hops := strings.Split(forwardedFor, ",")
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			return netip.Addr{}
		}
		hop = hop.Unmap()
		if !trusted(hop) {
			return hop
		}
		addr = hop
	}
	return addr
}

// OverCap says what happens to the session at position rank (0 is the oldest)
// among total live sessions: SessionEvicted when it is among the oldest
// beyond the cap under evict-oldest, SessionLimit when it is a newer session
// over the cap under reject, Allowed otherwise.
func (r Rules) OverCap(rank, total int) Verdict {
	if r.MaxSessions <= 0 || total <= r.MaxSessions {
		return Allowed
	}
	if r.RejectOnLimit {
		if rank >= r.MaxSessions {
			return SessionLimit
		}
		return Allowed
	}
	if rank < total-r.MaxSessions {
		return SessionEvicted
	}
	return Allowed
}
