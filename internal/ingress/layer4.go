package ingress

// This file: the raw TCP passthrough half of the ingress config, applied
// in the same Config/caddy.Load call as routes.go's HTTP routes. A
// stream gets its own dedicated listener (no Host header to match on),
// via caddy-l4's "layer4" app, blank-imported in driver.go.

// Layer4App is Caddy's "layer4" app (github.com/mholt/caddy-l4/layer4),
// mirrored here the same way HTTPApp/TLSApp hand-roll the wire-compatible
// subset of Caddy's own config types rather than importing them
// directly (see config.go's package doc comment).
type Layer4App struct {
	Servers map[string]*Layer4Server `json:"servers,omitempty"`
}

// Layer4Server is one layer4 listener plus the routes it serves. Unlike
// HTTPApp's Server (one shared listener, many Host-matched routes), one
// StreamRoute gets its own Layer4Server keyed by its own listen address,
// because a stream has nothing else to match requests on.
type Layer4Server struct {
	Listen []string      `json:"listen,omitempty"`
	Routes []Layer4Route `json:"routes,omitempty"`
}

// Layer4Route is one layer4 route: Match is always left empty (omitted),
// which caddy-l4's own MatcherSets.AnyMatch treats as "always match",
// the same convention Route.Match's absence has in Caddy's HTTP app.
type Layer4Route struct {
	Handle []any `json:"handle"`
}

// Layer4ProxyHandler is caddy-l4's "proxy" handler
// (layer4.handlers.proxy, github.com/mholt/caddy-l4/modules/l4proxy):
// plain TCP passthrough to Upstreams, no TLS termination or
// re-encryption. Handler is always the literal string "proxy": the
// module's own CaddyModule ID, mirroring ReverseProxyHandler's identical
// "fixed discriminator as a plain field" shape.
type Layer4ProxyHandler struct {
	Handler   string               `json:"handler"`
	Upstreams []Layer4UpstreamDial `json:"upstreams,omitempty"`
}

// Layer4UpstreamDial is one caddy-l4 proxy backend. Dial is a list for
// caddy-l4's own failover support; this package only ever sets one
// address, since a stream has exactly one backend (no load balancing,
// out of scope for this first slice, see RoutesOptions.Streams's own
// doc comment).
type Layer4UpstreamDial struct {
	Dial []string `json:"dial,omitempty"`
}

// NewLayer4ProxyHandler builds the one handler shape a stream needs:
// forward every byte to backendDial, unconditionally.
func NewLayer4ProxyHandler(backendDial string) Layer4ProxyHandler {
	return Layer4ProxyHandler{
		Handler:   "proxy",
		Upstreams: []Layer4UpstreamDial{{Dial: []string{backendDial}}},
	}
}
