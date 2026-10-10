package api

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

const (
	envDashboardURL   = "APP_DASHBOARD_URL"
	envTrustedProxies = "APP_INGRESS_TRUSTED_PROXIES" //nolint:gosec // env var name
	deviceCLIPath     = "/settings/cli-access"
)

// deviceVerificationBase returns the scheme and host the CLI's own request
// reached, so the printed link works through a tunnel, proxy or custom port.
// APP_DASHBOARD_URL wins when set. X-Forwarded-Host is honoured only from a
// loopback peer or one listed in APP_INGRESS_TRUSTED_PROXIES.
func deviceVerificationBase(r *http.Request) string {
	if base := configuredDashboardURL(); base != "" {
		return base
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); proto == "http" || proto == "https" {
		scheme = proto
	}
	host := r.Host
	if fwd := firstForwardedValue(r.Header.Get("X-Forwarded-Host")); fwd != "" && validHostPort(fwd) && peerIsTrustedProxy(r) {
		host = fwd
	}
	return scheme + "://" + host
}

func configuredDashboardURL() string {
	raw := strings.TrimRight(strings.TrimSpace(os.Getenv(envDashboardURL)), "/")
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return raw
}

func firstForwardedValue(v string) string {
	first, _, _ := strings.Cut(v, ",")
	return strings.TrimSpace(first)
}

func validHostPort(h string) bool {
	return h != "" && !strings.ContainsAny(h, "/\\@ \t?#")
}

// peerIsTrustedProxy reports whether the TCP peer may set forwarding headers.
func peerIsTrustedProxy(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, entry := range strings.Split(os.Getenv(envTrustedProxies), ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if p, err := netip.ParsePrefix(entry); err == nil && p.Contains(ip) {
			return true
		}
		if a, err := netip.ParseAddr(entry); err == nil && a == ip {
			return true
		}
	}
	return false
}
