package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

var errBlockedAddress = netguard.ErrBlockedAddress

// ErrBaseURLRejected means a provider base URL failed the SSRF policy.
var ErrBaseURLRejected = errors.New("ai: provider base URL is not allowed")

// NewHTTPClient returns a client for provider traffic. Unless allowPrivate,
// every dial is checked against the netguard blocklist on the resolved IP
// (so DNS rebinding is caught). Redirects to another host are refused and
// proxies are ignored.
func NewHTTPClient(allowPrivate bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("ai: parse dial address: %w", err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("ai: parse dial address: %w", err)
			}
			if netguard.IsBlocked(ip) {
				return fmt.Errorf("%w: %s", errBlockedAddress, ip)
			}
			return nil
		},
	}
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Host != via[0].URL.Host {
				return fmt.Errorf("ai: provider redirected to %q, refusing to follow", req.URL.Host)
			}
			return nil
		},
	}
}

// HostResolver resolves a hostname for the SSRF policy check; swappable in tests.
type HostResolver func(ctx context.Context, host string) ([]netip.Addr, error)

func defaultResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// BaseURLCheck is the verdict on one provider base URL.
type BaseURLCheck struct {
	URL     string
	Private bool
}

// CheckBaseURL applies the provider base URL policy: absolute http(s), no
// credentials in the URL, https for anything public, and private or
// loopback hosts only for openai_compat when privateAllowed (the caller is
// root and set the host themselves). Fails closed.
func CheckBaseURL(ctx context.Context, raw string, kind Kind, privateAllowed bool, resolve HostResolver) (BaseURLCheck, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return BaseURLCheck{}, fmt.Errorf("%w: must be an absolute http or https URL without credentials", ErrBaseURLRejected)
	}
	if resolve == nil {
		resolve = defaultResolver
	}
	host := strings.ToLower(u.Hostname())
	private := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if ip, perr := netip.ParseAddr(host); perr == nil {
		private = private || netguard.IsBlocked(ip)
	} else if !private {
		lctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if addrs, lerr := resolve(lctx, host); lerr == nil {
			for _, a := range addrs {
				if netguard.IsBlocked(a) {
					private = true
				}
			}
		}
	}
	if private && (kind != KindOpenAICompat || !privateAllowed) {
		return BaseURLCheck{}, fmt.Errorf("%w: %s is a private or loopback address", ErrBaseURLRejected, host)
	}
	if !private && u.Scheme != "https" {
		return BaseURLCheck{}, fmt.Errorf("%w: public endpoints must use https so the key is not sent in clear text", ErrBaseURLRejected)
	}
	return BaseURLCheck{URL: strings.TrimRight(u.String(), "/"), Private: private}, nil
}

// joinURL appends path to base while keeping any query string on base.
func joinURL(base, path string) string {
	u, err := url.Parse(base)
	if err != nil {
		return strings.TrimRight(base, "/") + path
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	return u.String()
}
