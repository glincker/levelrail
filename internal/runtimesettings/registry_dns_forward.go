package runtimesettings

import (
	"fmt"
	"net/netip"
	"time"
)

// Mesh DNS forwarding to the host's upstream resolvers (beta, only active while the mesh resolver is on).
const (
	KeyDNSForwardEnabled        = "dns.forward_enabled"
	KeyDNSForwardAttemptTimeout = "dns.forward_attempt_timeout"
	KeyDNSForwardQueryTimeout   = "dns.forward_query_timeout"
	KeyDNSForwardUpstreams      = "dns.forward_upstreams"
	KeyDNSCacheBytes            = "dns.cache_bytes"
	KeyDNSCacheMaxTTL           = "dns.cache_max_ttl"
	KeyDNSCacheNegativeMaxTTL   = "dns.cache_negative_max_ttl"
	KeyDNSContainerFallback     = "dns.container_fallback"
	KeyDNSRateLimitQPS          = "dns.rate_limit_qps"
	KeyDNSRateLimitBurst        = "dns.rate_limit_burst"

	// DNSFallbackHostUpstream and DNSFallbackNone are the values of dns.container_fallback.
	DNSFallbackHostUpstream = "host_upstream"
	DNSFallbackNone         = "none"
)

// dnsUpstreamCheck accepts a bare IP or an ip:port, the only shapes the forwarder dials.
func dnsUpstreamCheck(item string) error {
	if _, err := netip.ParseAddr(item); err == nil {
		return nil
	}
	if _, err := netip.ParseAddrPort(item); err == nil {
		return nil
	}
	return fmt.Errorf("%q must be an IP address or ip:port", item)
}

func init() {
	Register(Definition{
		Key: KeyDNSForwardEnabled, Category: CategoryNetworking, Kind: KindBool, Default: "true",
		Env: "LEVELRAIL_DNS_FORWARD_ENABLED", Apply: ApplyLive,
		Title: "Mesh DNS forwards public names (beta)",
		Description: "Answer names outside the mesh zone by asking the host's upstream resolvers instead of refusing them. " +
			"Only has an effect while the mesh resolver on bridge gateways is on.",
	})
	Register(Definition{
		Key: KeyDNSForwardAttemptTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "800ms",
		Env: "LEVELRAIL_DNS_FORWARD_ATTEMPT_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(50*time.Millisecond, 10*time.Second),
		Title:       "Mesh DNS upstream attempt timeout",
		Description: "How long one upstream resolver may take before the next one is tried.",
	})
	Register(Definition{
		Key: KeyDNSForwardQueryTimeout, Category: CategoryNetworking, Kind: KindDuration, Default: "2s",
		Env: "LEVELRAIL_DNS_FORWARD_QUERY_TIMEOUT", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(100*time.Millisecond, time.Minute),
		Title:       "Mesh DNS total query timeout",
		Description: "Longest a forwarded lookup may take across all upstreams before the client gets SERVFAIL.",
	})
	Register(Definition{
		Key: KeyDNSForwardUpstreams, Category: CategoryNetworking, Kind: KindStringList, Default: "",
		Env: "LEVELRAIL_DNS_FORWARD_UPSTREAMS", Apply: ApplyLive, CheckItem: dnsUpstreamCheck,
		Title:       "Mesh DNS upstream resolvers",
		Description: "IPs or ip:port entries to forward to, in order. Empty means the host's own resolv.conf.",
	})
	Register(Definition{
		Key: KeyDNSCacheBytes, Category: CategoryNetworking, Kind: KindInt, Default: "4194304",
		Env: "LEVELRAIL_DNS_CACHE_BYTES", Apply: ApplyLive, CheckValue: intRangeCheck(0, 1<<30),
		Title:       "Mesh DNS cache size in bytes",
		Description: "Memory budget of the forwarded answer cache. 0 turns the cache off. Kept small on purpose.",
	})
	Register(Definition{
		Key: KeyDNSCacheMaxTTL, Category: CategoryNetworking, Kind: KindDuration, Default: "1h",
		Env: "LEVELRAIL_DNS_CACHE_MAX_TTL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, 7*24*time.Hour),
		Title:       "Mesh DNS cache longest TTL",
		Description: "Cached answers never live longer than this, whatever the upstream TTL says.",
	})
	Register(Definition{
		Key: KeyDNSCacheNegativeMaxTTL, Category: CategoryNetworking, Kind: KindDuration, Default: "5m",
		Env: "LEVELRAIL_DNS_CACHE_NEGATIVE_MAX_TTL", Apply: ApplyLive,
		CheckValue:  durationRangeCheck(time.Second, 24*time.Hour),
		Title:       "Mesh DNS negative cache longest TTL",
		Description: "How long a name-does-not-exist answer that carries an SOA record may be cached.",
	})
	Register(Definition{
		Key: KeyDNSContainerFallback, Category: CategoryNetworking, Kind: KindString, Default: DNSFallbackHostUpstream,
		Env: "LEVELRAIL_DNS_CONTAINER_FALLBACK", Apply: ApplyLive,
		AllowedStrings: []string{DNSFallbackHostUpstream, DNSFallbackNone},
		Title:          "Container DNS fallback server",
		Description: "host_upstream gives new containers a second DNS server (a host upstream resolver) so lookups keep working while the gateway listener is down. " +
			"none keeps only the gateway (fail closed). New containers pick up a change.",
	})
	Register(Definition{
		Key: KeyDNSRateLimitQPS, Category: CategoryNetworking, Kind: KindInt, Default: "200",
		Env: "LEVELRAIL_DNS_RATE_LIMIT_QPS", Apply: ApplyLive, CheckValue: intRangeCheck(0, 1000000),
		Title:       "Mesh DNS forward rate per source",
		Description: "Upstream lookups per second one container may cause. 0 turns the limit off. Excess queries get REFUSED.",
	})
	Register(Definition{
		Key: KeyDNSRateLimitBurst, Category: CategoryNetworking, Kind: KindInt, Default: "400",
		Env: "LEVELRAIL_DNS_RATE_LIMIT_BURST", Apply: ApplyLive, CheckValue: intRangeCheck(1, 1000000),
		Title:       "Mesh DNS forward burst per source",
		Description: "How many upstream lookups one container may send at once before the rate limit applies.",
	})
}
