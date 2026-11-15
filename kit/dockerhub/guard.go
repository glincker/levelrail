package dockerhub

import (
	"fmt"
	"net/http"
)

// allowedHosts are the only hosts this client may reach, redirects included.
var allowedHosts = map[string]bool{
	"hub.docker.com":       true,
	"auth.docker.io":       true,
	"registry-1.docker.io": true,
}

// hostGuard refuses any request outside allowedHosts or without TLS, so a
// redirect or a crafted name cannot send a Docker Hub token elsewhere.
type hostGuard struct{ next http.RoundTripper }

func (g hostGuard) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" || !allowedHosts[req.URL.Hostname()] {
		return nil, fmt.Errorf("dockerhub: host %q is not allowed", req.URL.Hostname())
	}
	return g.next.RoundTrip(req)
}
