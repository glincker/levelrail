package api

import (
	"net"
	"net/http"
	"strings"
	"time"
)

// requestIsHTTPS reports whether the client reached us over TLS, either
// directly or through the embedded Caddy ingress. X-Forwarded-Proto is
// only trusted from a loopback peer, since any remote client can set it.
func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if !peerIsLoopback(r) {
		return false
	}
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func peerIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// setSessionCookie writes the session cookie; an empty token clears it.
func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	c := &http.Cookie{ //nolint:gosec // Secure follows the transport, see requestIsHTTPS
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	}
	if token == "" {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	http.SetCookie(w, c)
}
