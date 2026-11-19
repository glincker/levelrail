package ingress

import (
	"net"
	"strconv"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

// autoRedirectServer is the name Caddy gives the server it adds for the
// automatic HTTP to HTTPS redirect.
const autoRedirectServer = "remaining_auto_https_redirects"

// BoundPorts reports the TCP ports the running HTTP servers really listen on,
// which differ from the configured ones when an address ends in :0. Zero means
// unknown (not started, or no such server).
func (d *Driver) BoundPorts() (httpPort, httpsPort int) {
	app, err := caddy.ActiveContext().AppIfConfigured("http")
	if err != nil {
		return 0, 0
	}
	httpApp, ok := app.(*caddyhttp.App)
	if !ok {
		return 0, 0
	}
	for name, srv := range httpApp.Servers {
		for _, ln := range srv.Listeners() {
			port := portOfAddr(ln.Addr())
			if port == 0 {
				continue
			}
			if name == autoRedirectServer {
				httpPort = port
			} else if httpsPort == 0 {
				httpsPort = port
			}
		}
	}
	return httpPort, httpsPort
}

func portOfAddr(a net.Addr) int {
	if a == nil {
		return 0
	}
	_, p, err := net.SplitHostPort(a.String())
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return 0
	}
	return n
}
