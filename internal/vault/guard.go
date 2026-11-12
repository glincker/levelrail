package vault

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Errors returned when the configured address fails the connection guard.
var (
	ErrBlockedAddress  = errors.New("vault address is link-local, metadata or private; for a Vault on a private or loopback address turn on the vault.allow_private_address setting")
	ErrRedirectBlocked = errors.New("vault redirected to another host or scheme")
)

// maxRedirects bounds same-host redirects; Vault itself only redirects standbys.
const maxRedirects = 3

// blockedIP always rejects link-local (cloud metadata), unspecified and multicast,
// and rejects loopback and private ranges unless the admin allowed them.
func blockedIP(ip net.IP, allowPrivate bool) bool {
	if alwaysBlockedRule(ip) != "" {
		return true
	}
	return !allowPrivate && (ip.IsPrivate() || ip.IsLoopback())
}

// alwaysBlockedRule names the never-allowed range ip falls in, "" when none.
func alwaysBlockedRule(ip net.IP) string {
	switch {
	case ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast():
		return "link-local (cloud metadata) range"
	case ip.IsUnspecified():
		return "unspecified address"
	case ip.IsMulticast():
		return "multicast range"
	}
	return ""
}

// metadataHosts are names that resolve to a cloud metadata service.
var metadataHosts = map[string]bool{"metadata.google.internal": true, "metadata": true, "instance-data": true}

// AddressRule reports the always-blocked rule a literal host hits, "" when the
// host passes. Names are only checked against metadata hosts: the dial-time
// guard still covers whatever a name resolves to.
func AddressRule(host string) string {
	host = strings.Trim(strings.ToLower(host), "[]")
	if metadataHosts[host] {
		return "cloud metadata host"
	}
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	if ip := net.ParseIP(host); ip != nil {
		return alwaysBlockedRule(ip)
	}
	return ""
}

// guardedClient checks the resolved IP at dial time, so DNS rebinding cannot
// bypass it, and refuses any redirect to another host or scheme.
func guardedClient(address string, allowPrivate bool, timeout time.Duration) (*http.Client, error) {
	base, err := url.Parse(address)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, fmt.Errorf("vault: invalid address")
	}
	dialer := &net.Dialer{Timeout: timeout, Control: func(_, addr string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return err
		}
		if ip := net.ParseIP(host); ip == nil || blockedIP(ip, allowPrivate) {
			return ErrBlockedAddress
		}
		return nil
	}}
	// Proxy stays off so the dial check sees the real Vault address.
	transport := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: timeout, ResponseHeaderTimeout: timeout}
	return &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != base.Host || req.URL.Scheme != base.Scheme || len(via) > maxRedirects {
			return ErrRedirectBlocked
		}
		return nil
	}}, nil
}
