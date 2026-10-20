package models

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// EndpointAllowPrivateEnv lets external inference endpoints sit on private
// addresses (a Mac on the mesh or LAN). netguard.AllowPrivateEnv enables it too.
const EndpointAllowPrivateEnv = "APP_MODEL_ENDPOINT_ALLOW_PRIVATE_NETWORKS"

// Endpoint URL refusals.
var (
	ErrEndpointBlocked = errors.New("models: endpoint address is never allowed (link-local, metadata or unspecified)")
	ErrEndpointPrivate = errors.New("models: endpoint address is private and internal destinations are not enabled")
)

// EndpointPrivateAllowed reports whether the operator enabled internal destinations.
func EndpointPrivateAllowed() bool {
	for _, name := range []string{EndpointAllowPrivateEnv, netguard.AllowPrivateEnv} {
		if v, err := strconv.ParseBool(os.Getenv(name)); err == nil && v {
			return true
		}
	}
	return false
}

var alwaysBlocked = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("100.100.100.200/32"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fd00:ec2::/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
}

// endpointAlwaysBlocked is true for addresses no opt-in unlocks: link-local,
// cloud metadata, unspecified, multicast and tunnels to those.
func endpointAlwaysBlocked(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsUnspecified() || a.IsMulticast() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() {
		return true
	}
	for _, p := range alwaysBlocked {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func checkEndpointAddr(a netip.Addr) error {
	a = a.Unmap()
	if endpointAlwaysBlocked(a) {
		return fmt.Errorf("%w: %s", ErrEndpointBlocked, a)
	}
	if netguard.IsBlocked(a) && !EndpointPrivateAllowed() {
		return fmt.Errorf("%w: %s (set %s=true on the control plane)", ErrEndpointPrivate, a, EndpointAllowPrivateEnv)
	}
	return nil
}

// endpointDialControl runs on the exact IP about to be dialed, so a hostname
// that re-resolves to a forbidden address is still refused.
func endpointDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("models: parse dial address %q: %w", address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("models: parse dial address %q: %w", address, err)
	}
	return checkEndpointAddr(ip)
}

func newEndpointTransport(dialTimeout, headerTimeout time.Duration) *http.Transport {
	d := &net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second, Control: endpointDialControl}
	return &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		ResponseHeaderTimeout: max(headerTimeout, 0),
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
	}
}

// HostLookup resolves a hostname for URL validation; swappable in tests.
type HostLookup func(ctx context.Context, host string) ([]netip.Addr, error)

func defaultLookup(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// NormalizeEndpointURL validates an endpoint base URL and returns it without
// a trailing slash or /v1 suffix. A hostname that does not resolve is
// accepted, since the dial check decides at request time.
func NormalizeEndpointURL(ctx context.Context, raw string, lookup HostLookup) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("%w: base URL must be an absolute http or https URL", ErrInvalid)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%w: base URL must not embed credentials, a query or a fragment", ErrInvalid)
	}
	if lookup == nil {
		lookup = defaultLookup
	}
	host := strings.ToLower(u.Hostname())
	var addrs []netip.Addr
	if ip, perr := netip.ParseAddr(host); perr == nil {
		addrs = []netip.Addr{ip}
	} else if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		addrs = []netip.Addr{netip.MustParseAddr("127.0.0.1")}
	} else {
		lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if got, lerr := lookup(lctx, host); lerr == nil {
			addrs = got
		}
	}
	private := false
	for _, a := range addrs {
		if err := checkEndpointAddr(a); err != nil {
			return "", err
		}
		private = private || netguard.IsBlocked(a.Unmap())
	}
	if !private && len(addrs) > 0 && u.Scheme != "https" {
		return "", fmt.Errorf("%w: public endpoints must use https so the credential is not sent in clear text", ErrInvalid)
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	return strings.TrimRight(u.String(), "/"), nil
}
