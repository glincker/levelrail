package desktopmode

import (
	"net"
	"net/http"
	"strings"
)

// Host is the only name desktop mode answers to. Browsers share cookies across ports of
// one host name, so a name of its own keeps the session away from other local servers.
const Host = "levelrail.localhost"

// AllowedHosts expands Host over the given ports into the exact Host header values allowed.
func AllowedHosts(ports []string) map[string]bool {
	out := make(map[string]bool, len(ports))
	for _, p := range ports {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out[Host+":"+p] = true
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

func peerHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func peerIsLoopback(r *http.Request) bool {
	ip := net.ParseIP(peerHost(r))
	return ip != nil && ip.IsLoopback()
}
