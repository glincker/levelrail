package trafficpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strconv"
)

// Environment switches that let forwarders reach non-public addresses.
const (
	EnvForwardAllowPrivate  = "APP_FORWARDER_ALLOW_PRIVATE_NETWORKS"
	EnvForwardAllowLoopback = "APP_FORWARDER_ALLOW_LOOPBACK"
)

// ErrBlockedTarget is returned when a forwarder URL resolves to an address
// the egress guard refuses.
var ErrBlockedTarget = errors.New("trafficpolicy: forward target address is not allowed")

// EgressGuard decides which addresses an external forwarder may reach.
// Metadata and link-local ranges are refused even when private is allowed,
// mirroring platformimport.NetworkPolicy.
type EgressGuard struct {
	AllowPrivate  bool
	AllowLoopback bool
}

// EgressGuardFromEnv reads the two switches above.
func EgressGuardFromEnv(getenv func(string) string) EgressGuard {
	if getenv == nil {
		getenv = os.Getenv
	}
	p, _ := strconv.ParseBool(getenv(EnvForwardAllowPrivate))
	lo, _ := strconv.ParseBool(getenv(EnvForwardAllowLoopback))
	return EgressGuard{AllowPrivate: p, AllowLoopback: lo}
}

var (
	metadataPrefixes = []netip.Prefix{
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fd00:ec2::/32"),
		netip.MustParsePrefix("100.100.100.200/32"),
	}
	internalPrefixes = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("fec0::/10"),
		netip.MustParsePrefix("0.0.0.0/8"),
	}
)

// Check reports whether addr may be dialed.
func (g EgressGuard) Check(addr netip.Addr) error {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() {
		return fmt.Errorf("%w: %s", ErrBlockedTarget, addr)
	}
	for _, pre := range metadataPrefixes {
		if pre.Contains(addr) {
			return fmt.Errorf("%w: %s is a metadata or link-local address", ErrBlockedTarget, addr)
		}
	}
	if addr.IsLoopback() {
		if g.AllowLoopback {
			return nil
		}
		return fmt.Errorf("%w: %s is loopback (set %s=true to permit)", ErrBlockedTarget, addr, EnvForwardAllowLoopback)
	}
	internal := addr.IsPrivate()
	for _, pre := range internalPrefixes {
		if pre.Contains(addr) {
			internal = true
		}
	}
	if internal && !g.AllowPrivate {
		return fmt.Errorf("%w: %s is a private address (set %s=true to permit)", ErrBlockedTarget, addr, EnvForwardAllowPrivate)
	}
	return nil
}

// Resolver is the DNS surface CheckTarget needs; *net.Resolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// ResolvedTarget is an external forwarder URL pinned to one checked address.
type ResolvedTarget struct {
	Scheme     string
	Host       string
	Port       string
	Addr       netip.Addr
	BasePath   string
	ServerName string
}

// Dial is the ip:port Caddy should connect to.
func (t ResolvedTarget) Dial() string {
	return net.JoinHostPort(t.Addr.String(), t.Port)
}

// CheckTarget resolves rawURL and checks every address it resolves to, so a
// name with one public and one private record is refused as a whole.
func (g EgressGuard) CheckTarget(ctx context.Context, r Resolver, rawURL string) (ResolvedTarget, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return ResolvedTarget{}, fmt.Errorf("trafficpolicy: parse forward url: invalid url")
	}
	port := u.Port()
	if port == "" {
		port = "443"
		if u.Scheme == "http" {
			port = "80"
		}
	}
	t := ResolvedTarget{Scheme: u.Scheme, Host: u.Hostname(), Port: port, BasePath: u.EscapedPath()}
	if u.Scheme == "https" {
		t.ServerName = u.Hostname()
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil {
		if err := g.Check(ip); err != nil {
			return ResolvedTarget{}, err
		}
		t.Addr = ip.Unmap()
		return t, nil
	}
	addrs, err := r.LookupNetIP(ctx, "ip", u.Hostname())
	if err != nil {
		return ResolvedTarget{}, fmt.Errorf("trafficpolicy: resolve %s: %w", u.Hostname(), err)
	}
	if len(addrs) == 0 {
		return ResolvedTarget{}, fmt.Errorf("trafficpolicy: resolve %s: no addresses", u.Hostname())
	}
	for _, a := range addrs {
		if err := g.Check(a); err != nil {
			return ResolvedTarget{}, err
		}
	}
	t.Addr = addrs[0].Unmap()
	return t, nil
}
