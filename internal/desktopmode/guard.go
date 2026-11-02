package desktopmode

import (
	"net"
	"net/http"
	"strings"
)

var loopbackNames = []string{"localhost", "127.0.0.1", "[::1]"}

// AllowedHosts expands the loopback names over the given ports into the
// exact Host header values the dashboard may be reached by.
func AllowedHosts(ports []string) map[string]bool {
	out := make(map[string]bool, len(loopbackNames)*len(ports))
	for _, p := range ports {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		for _, n := range loopbackNames {
			out[n+":"+p] = true
		}
	}
	return out
}

func (s *Service) hostAllowed(r *http.Request) bool {
	return s.hosts[strings.ToLower(r.Host)]
}

// originAllowed accepts a missing Origin (native clients send none) but
// never a foreign, opaque or malformed one.
func (s *Service) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	host, ok := strings.CutPrefix(strings.ToLower(origin), "http://")
	return ok && s.hosts[host]
}

func stateChanging(r *http.Request) bool {
	switch r.Method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func peerIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
