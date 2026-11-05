package network

import (
	"net"
	"strconv"
)

// DefaultDNSAddr is an unprivileged port so the mesh DNS server binds without root.
const DefaultDNSAddr = ":5390"

// EnvDNSAddr overrides the address the mesh DNS server binds.
const EnvDNSAddr = "APP_MESH_DNS_ADDR"

// ResolveDNSAddr returns the mesh DNS bind address from the environment, or the default.
func ResolveDNSAddr(lookup func(string) (string, bool)) string {
	if v, ok := lookup(EnvDNSAddr); ok && v != "" {
		return v
	}
	return DefaultDNSAddr
}

// DNSPort extracts the port from a bind address, or 0 when it has none.
func DNSPort(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
