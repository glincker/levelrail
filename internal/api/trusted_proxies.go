package api

import (
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
)

// resolvedTrusted is the list the edge applies (env, dashboard value, preset),
// published each reconcile pass so the API and the edge trust the same peers.
var resolvedTrusted atomic.Pointer[[]netip.Prefix]

// SetResolvedTrustedProxies publishes the edge's effective trusted proxy list.
func SetResolvedTrustedProxies(entries []string) {
	ingressdriver.PublishTrustedProxies(entries)
	if entries == nil {
		resolvedTrusted.Store(nil)
		return
	}
	out := parseTrustedEntries(entries)
	resolvedTrusted.Store(&out)
}

// ResolvedTrustedProxies returns the prefixes the API trusts right now.
func ResolvedTrustedProxies() []netip.Prefix { return trustedProxyPrefixes() }

// trustedProxyPrefixes falls back to the env list until the edge publishes one.
func trustedProxyPrefixes() []netip.Prefix {
	if p := resolvedTrusted.Load(); p != nil {
		return *p
	}
	raw := os.Getenv(envTrustedProxies)
	entries, err := ingressdriver.ParseTrustedProxyList(raw)
	if err != nil {
		entries = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	}
	return parseTrustedEntries(entries)
}

func parseTrustedEntries(entries []string) []netip.Prefix {
	var out []netip.Prefix
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if p, err := netip.ParsePrefix(e); err == nil {
			out = append(out, p)
		} else if a, err := netip.ParseAddr(e); err == nil {
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}
	return out
}

// dockerBridgeRange is where Docker allocates its bridge networks.
var dockerBridgeRange = netip.MustParsePrefix("172.16.0.0/12")

func trustedDockerBridge(trusted []netip.Prefix) (netip.Prefix, bool) {
	for _, p := range trusted {
		if p.Overlaps(dockerBridgeRange) {
			return p, true
		}
	}
	return netip.Prefix{}, false
}

// listensOnAllInterfaces reports a bind such as :8080 or 0.0.0.0:8080.
func listensOnAllInterfaces(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return host == "" || host == "0.0.0.0" || host == "::"
}
