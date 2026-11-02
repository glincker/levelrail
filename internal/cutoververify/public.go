package cutoververify

import (
	"errors"
	"net/netip"
)

// ErrNonPublicAddress is returned when a probe would connect to an address
// that is not on the public internet.
var ErrNonPublicAddress = errors.New("address is not public")

// reservedPrefixes are ranges the standard helpers do not call private but
// that never hold a public server: "this network", carrier-grade NAT (which
// also holds some cloud metadata addresses), IETF protocol space, reserved and
// broadcast.
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// IsPublicAddr reports whether ip may be probed: not loopback, private (RFC1918
// and ULA), link-local (which covers the 169.254.169.254 metadata address),
// unspecified, multicast or reserved. Documentation ranges count as public.
func IsPublicAddr(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, p := range reservedPrefixes {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// IsPublicIP is IsPublicAddr for a textual address; unparsable text is not public.
func IsPublicIP(s string) bool {
	ip, err := netip.ParseAddr(s)
	return err == nil && IsPublicAddr(ip)
}
