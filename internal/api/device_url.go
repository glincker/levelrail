package api

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const (
	envDashboardURL   = "APP_DASHBOARD_URL"
	envTrustedProxies = "APP_INGRESS_TRUSTED_PROXIES" //nolint:gosec // env var name
	deviceCLIPath     = "/settings/cli-access"
)

// APP_DASHBOARD_URL wins when set; X-Forwarded-Host is trusted only from loopback or listed proxies.
func deviceVerificationBase(r *http.Request) string {
	if base := configuredDashboardURL(); base != "" {
		return base
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); (proto == "http" || proto == "https") && peerIsTrustedProxy(r) {
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

// PeerIsTrustedProxy is peerIsTrustedProxy for wiring outside this package.
func PeerIsTrustedProxy(r *http.Request) bool { return peerIsTrustedProxy(r) }

// peerIsTrustedProxy reports whether the request may carry forwarding headers: it came
// through this process's ingress, or its TCP peer is loopback or a listed proxy.
func peerIsTrustedProxy(r *http.Request) bool {
	if requestFromIngress(r) {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return isTrustedPeer(host, trustedProxyPrefixes())
}
