// Package abuse is the per-app ingress abuse defense: a policy, a sharded
// per-client token bucket, deny lists and a replay of past traffic.
package abuse

import (
	"fmt"
	"net/netip"
	"strings"
)

// Mode says what the policy does with a request it would turn away.
type Mode string

// The three modes. Off is the default for an app with no policy.
const (
	ModeOff     Mode = "off"
	ModeLogOnly Mode = "log_only"
	ModeEnforce Mode = "enforce"
)

// Valid reports whether m is one of the three modes.
func (m Mode) Valid() bool { return m == ModeOff || m == ModeLogOnly || m == ModeEnforce }

// Reason names why a request would be, or was, turned away.
type Reason int

// Reasons, in the order their counters are stored.
const (
	ReasonRateLimit Reason = iota
	ReasonDenyCIDR
	ReasonDenyUserAgent
	ReasonMissingUserAgent
	ReasonCount
)

var reasonNames = [ReasonCount]string{"rate_limit", "deny_cidr", "deny_user_agent", "missing_user_agent"}

// String is the reason's stable wire and metric name.
func (r Reason) String() string {
	if r < 0 || r >= ReasonCount {
		return "unknown"
	}
	return reasonNames[r]
}

// ReasonNames lists every reason name in counter order.
func ReasonNames() []string { return append([]string(nil), reasonNames[:]...) }

// Policy is one app's ingress abuse defense. RPS 0 turns the rate limit off,
// leaving only the deny lists and the User-Agent requirement.
type Policy struct {
	Mode Mode `json:"mode"`
	// RPS is the sustained request rate allowed per client IP; Burst is the
	// bucket size (at least 1 whenever RPS is set).
	RPS   float64 `json:"rps,omitempty"`
	Burst int     `json:"burst,omitempty"`
	// DenyCIDRs are addresses and ranges turned away outright.
	DenyCIDRs []string `json:"deny_cidrs,omitempty"`
	// DenyUserAgents are case-insensitive substrings of the User-Agent header.
	DenyUserAgents   []string `json:"deny_user_agents,omitempty"`
	RequireUserAgent bool     `json:"require_user_agent,omitempty"`
}

// Active reports whether the policy evaluates requests at all.
func (p Policy) Active() bool { return p.Mode == ModeLogOnly || p.Mode == ModeEnforce }

// Normalize defaults an empty mode to off, fills the burst when a rate is set
// without one, and trims list entries. It never changes what a valid policy means.
func (p Policy) Normalize(l Limits) Policy {
	if p.Mode == "" {
		p.Mode = ModeOff
	}
	if p.RPS > 0 && p.Burst <= 0 {
		p.Burst = max(1, int(p.RPS*l.SuggestBurstSeconds))
	}
	p.DenyCIDRs = trimList(p.DenyCIDRs)
	p.DenyUserAgents = trimList(p.DenyUserAgents)
	return p
}

func trimList(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ParseDenyEntry parses an IP address or CIDR range into a prefix.
func ParseDenyEntry(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("abuse: %q is not a CIDR range", s)
		}
		return normalizePrefix(p), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("abuse: %q is not an IP address or CIDR range", s)
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// normalizePrefix unmaps IPv4-mapped IPv6 prefixes so they match the
// unmapped client addresses the handler sees.
func normalizePrefix(p netip.Prefix) netip.Prefix {
	a := p.Addr()
	if a.Is4In6() && p.Bits() >= 96 {
		return netip.PrefixFrom(a.Unmap(), p.Bits()-96).Masked()
	}
	return p.Masked()
}

// FieldError is one problem with a policy field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError lists every problem found, so a form can show them at once.
type ValidationError struct{ Fields []FieldError }

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for _, f := range e.Fields {
		parts = append(parts, f.Field+": "+f.Message)
	}
	return "abuse policy invalid: " + strings.Join(parts, "; ")
}

// Validate checks the policy against l.
func (p Policy) Validate(l Limits) error {
	var errs []FieldError
	add := func(field, format string, a ...any) {
		errs = append(errs, FieldError{Field: field, Message: fmt.Sprintf(format, a...)})
	}
	if !p.Mode.Valid() {
		add("mode", "must be off, log_only or enforce")
	}
	switch {
	case p.RPS < 0:
		add("rps", "must not be negative")
	case p.RPS > l.MaxRPS:
		add("rps", "must be at most %g", l.MaxRPS)
	}
	if p.Burst < 0 {
		add("burst", "must not be negative")
	}
	if p.Burst > l.MaxBurst {
		add("burst", "must be at most %d", l.MaxBurst)
	}
	if p.Burst > 0 && p.RPS == 0 {
		add("burst", "needs rps")
	}
	if len(p.DenyCIDRs) > l.MaxDenyEntries {
		add("deny_cidrs", "at most %d entries", l.MaxDenyEntries)
	}
	for i, e := range p.DenyCIDRs {
		if _, err := ParseDenyEntry(e); err != nil {
			add(fmt.Sprintf("deny_cidrs[%d]", i), "must be an IP address or CIDR range")
		}
	}
	if len(p.DenyUserAgents) > l.MaxAgentEntries {
		add("deny_user_agents", "at most %d entries", l.MaxAgentEntries)
	}
	for i, e := range p.DenyUserAgents {
		if strings.TrimSpace(e) == "" || len(e) > 128 || !printable(e) {
			add(fmt.Sprintf("deny_user_agents[%d]", i), "must be 1 to 128 printable characters")
		}
	}
	if p.Active() && p.RPS == 0 && len(p.DenyCIDRs) == 0 && len(p.DenyUserAgents) == 0 && !p.RequireUserAgent {
		add("", "turn on at least one protection")
	}
	if len(errs) > 0 {
		return &ValidationError{Fields: errs}
	}
	return nil
}

func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}
