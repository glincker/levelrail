package vault

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// Errors returned when the configured address fails the connection guard.
var (
	ErrBlockedAddress  = errors.New("vault address is link-local, metadata or private and private addresses are not allowed")
	ErrRedirectBlocked = errors.New("vault redirected to another host or scheme")
)

// maxRedirects bounds same-host redirects; Vault itself only redirects standbys.
const maxRedirects = 3

// blockedIP always rejects link-local (cloud metadata), unspecified and multicast,
// and rejects loopback and private ranges unless the admin allowed them.
func blockedIP(ip net.IP, allowPrivate bool) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	return !allowPrivate && (ip.IsPrivate() || ip.IsLoopback())
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
