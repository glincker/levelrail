package api

import (
	"net/http"
	"net/netip"
	"testing"
)

func TestClientAddrUsesResolvedTrustedList(t *testing.T) {
	t.Setenv(envTrustedProxies, "")
	t.Cleanup(func() { resolvedTrusted.Store(nil) })
	SetResolvedTrustedProxies([]string{"203.0.113.0/24"})
	r := &http.Request{RemoteAddr: "203.0.113.9:443", Header: http.Header{"X-Forwarded-For": {"198.51.100.7"}}}
	if got := clientIP(r); got != "198.51.100.7" {
		t.Fatalf("clientIP = %s, want the forwarded visitor", got)
	}
}

func TestClientAddrEnvPrivateRangesFallback(t *testing.T) {
	resolvedTrusted.Store(nil)
	t.Setenv(envTrustedProxies, "private_ranges")
	if !isTrustedPeer("10.1.2.3", trustedProxyPrefixes()) {
		t.Fatal("private_ranges must expand for the api")
	}
}

func TestDoctorTrustedBridgeWarning(t *testing.T) {
	if _, ok := trustedDockerBridge([]netip.Prefix{netip.MustParsePrefix("172.17.0.0/16")}); !ok {
		t.Fatal("bridge range not detected")
	}
	if _, ok := trustedDockerBridge([]netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}); ok {
		t.Fatal("public range flagged")
	}
	if !listensOnAllInterfaces(":8080") || listensOnAllInterfaces("127.0.0.1:8080") {
		t.Fatal("listen address classification wrong")
	}
}
