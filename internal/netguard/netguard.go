// Package netguard builds HTTP clients for user-configured outbound URLs
// (alert and deploy notification webhooks) that refuse to connect to
// loopback, private, link-local, or otherwise internal addresses.
package netguard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"syscall"
	"time"
)

// AllowPrivateEnv names the env var that, when true, lets outbound
// notification requests reach internal addresses (for example a
// self-hosted chat server on a private IP).
const AllowPrivateEnv = "APP_NOTIFY_ALLOW_PRIVATE_NETWORKS"

// ErrBlockedAddress is returned when a dial targets an internal address
// while AllowPrivateEnv is not enabled.
var ErrBlockedAddress = errors.New("netguard: destination address is internal and not allowed")

var extraBlocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("fec0::/10"),
}

var nat64Prefixes = []netip.Prefix{
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
}

var sixToFour = netip.MustParsePrefix("2002::/16")

// AllowPrivate reports whether AllowPrivateEnv is set to a true value.
func AllowPrivate() bool {
	v, err := strconv.ParseBool(os.Getenv(AllowPrivateEnv))
	return err == nil && v
}

// IsBlocked reports whether addr is an address an outbound notification
// must not reach unless AllowPrivateEnv is enabled.
func IsBlocked(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() {
		return true
	}
	for _, p := range extraBlocked {
		if p.Contains(addr) {
			return true
		}
	}
	if embedded, ok := embeddedIPv4(addr); ok {
		return IsBlocked(embedded)
	}
	return false
}

// embeddedIPv4 extracts the IPv4 address a NAT64 or 6to4 address
// tunnels to, since those reach the embedded address, not a public one.
func embeddedIPv4(addr netip.Addr) (netip.Addr, bool) {
	if !addr.Is6() {
		return netip.Addr{}, false
	}
	b := addr.As16()
	for _, p := range nat64Prefixes {
		if p.Contains(addr) {
			return netip.AddrFrom4([4]byte{b[12], b[13], b[14], b[15]}), true
		}
	}
	if sixToFour.Contains(addr) {
		return netip.AddrFrom4([4]byte{b[2], b[3], b[4], b[5]}), true
	}
	return netip.Addr{}, false
}

// control runs after DNS resolution, on the exact IP about to be dialed,
// so a hostname that re-resolves to an internal address is still caught.
func control(_, address string, _ syscall.RawConn) error {
	if AllowPrivate() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("netguard: parse dial address %q: %w", address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("netguard: parse dial address %q: %w", address, err)
	}
	if IsBlocked(ip) {
		return fmt.Errorf("%w: %s (set %s=true to allow)", ErrBlockedAddress, ip, AllowPrivateEnv)
	}
	return nil
}

// NewClient returns an HTTP client whose every connection, including
// redirects, is refused when it targets an internal address. It ignores
// HTTP(S)_PROXY, since a proxy would dial the destination on our behalf
// and bypass the check.
func NewClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   control,
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}
