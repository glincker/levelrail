package docker

import "net/netip"

// addrStr renders an optional address the way the old string fields did:
// empty when unset.
func addrStr(a netip.Addr) string {
	if !a.IsValid() {
		return ""
	}
	return a.String()
}

// parseHostIP maps a spec bind address to the typed field, empty or invalid meaning all interfaces.
func parseHostIP(s string) netip.Addr {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}
	}
	return a
}
