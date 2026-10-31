package netguard

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// metadataAddrs are cloud metadata endpoints that stay blocked even when a
// feature is allowed to reach its own private service.
var metadataAddrs = []netip.Addr{
	netip.MustParseAddr("100.100.100.200"),
	netip.MustParseAddr("fd00:ec2::254"),
}

// IsPrivateTargetAllowed reports whether addr may be dialed by a feature that
// talks to one operator-configured private service: loopback and private
// ranges pass, link-local, unspecified, multicast and metadata addresses do not.
func IsPrivateTargetAllowed(addr netip.Addr) bool {
	// A zone id would defeat the metadata comparison below.
	addr = addr.Unmap().WithZone("")
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() ||
		addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
		return false
	}
	for _, m := range metadataAddrs {
		if addr == m {
			return false
		}
	}
	if addr.IsLoopback() || addr.IsPrivate() {
		return true
	}
	return !IsBlocked(addr)
}

// IsPrivateNetworkAllowed is IsPrivateTargetAllowed without loopback: public
// and private-range addresses pass, loopback, link-local and metadata do not.
func IsPrivateNetworkAllowed(addr netip.Addr) bool {
	if addr.Unmap().IsLoopback() {
		return false
	}
	return IsPrivateTargetAllowed(addr)
}

// NewClientPrivateTarget is NewClient for a single configured internal
// service. It allows private and loopback dials but never link-local or
// metadata addresses, and it does not change the guard for other callers.
func NewClientPrivateTarget() *http.Client {
	return newClientWithPolicy(IsPrivateTargetAllowed)
}

// NewClientPrivateNetworks allows private-range dials (LAN services) but
// still refuses loopback, link-local and metadata addresses.
func NewClientPrivateNetworks() *http.Client {
	return newClientWithPolicy(IsPrivateNetworkAllowed)
}

func newClientWithPolicy(allowed func(netip.Addr) bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("netguard: parse dial address %q: %w", address, err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("netguard: parse dial address %q: %w", address, err)
			}
			if !allowed(ip) {
				return fmt.Errorf("%w: %s", ErrBlockedAddress, ip)
			}
			return nil
		},
	}
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport: &http.Transport{
			Proxy:               nil,
			DialContext:         dialer.DialContext,
			MaxIdleConns:        4,
			IdleConnTimeout:     90 * time.Second,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}
