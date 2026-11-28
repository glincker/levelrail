package ingress

import (
	"net/netip"
	"os"
	"strings"
	"sync/atomic"

	"github.com/glincker/levelrail/internal/abuse"
)

type resolvedList struct {
	entries  []string
	prefixes []netip.Prefix
}

// resolvedTrusted is the list the edge applies (env, dashboard value, preset),
// published each reconcile pass so every reader trusts the same peers.
var resolvedTrusted atomic.Pointer[resolvedList]

// PublishTrustedProxies records the edge's effective trusted proxy list. A nil
// list clears it so readers fall back to the env seed.
func PublishTrustedProxies(entries []string) {
	if entries == nil {
		resolvedTrusted.Store(nil)
		return
	}
	p, _ := abuse.ParseTrusted(strings.Join(entries, ","))
	resolvedTrusted.Store(&resolvedList{entries: append([]string(nil), entries...), prefixes: p})
}

// TrustedProxyPrefixes returns the published list, else the env seed.
func TrustedProxyPrefixes() []netip.Prefix {
	if l := resolvedTrusted.Load(); l != nil {
		return l.prefixes
	}
	p, _ := abuse.ParseTrusted(os.Getenv(EnvTrustedProxies))
	return p
}

// TrustedProxiesConfigured reports whether any trusted proxy is set, from the
// published list or, before one exists, from seed.
func TrustedProxiesConfigured(seed string) bool {
	if l := resolvedTrusted.Load(); l != nil {
		return len(l.entries) > 0
	}
	return strings.TrimSpace(seed) != ""
}
