package ingress

// This file: the "many routes on one shared server" API (TASKS.md 1.6's
// real ingress controller, internal/reconcile/ingress) plus its
// build.type: static (served by the embedded Caddy directly, no
// container) extension. config.go stays scoped
// to the Phase 0 spike's single-route BuildProxyConfig and the Caddy
// JSON config structs both files build; this file is everything a
// controller tracking many independently host-routed backends, of
// either kind, needs on top of that.

import "fmt"

// FileServerHandler is Caddy's "file_server" handler
// (http.handlers.file_server): serves files directly from Root, no
// backend, no container. This is the whole of what this design means
// by "static sites get served by the embedded Caddy directly with no
// container": a route whose Handle is this instead of a
// ReverseProxyHandler. Registered via the same
// modules/standard blank import driver.go already pulls in for
// reverse_proxy and the TLS issuers, no separate module wiring needed.
type FileServerHandler struct {
	Handler string `json:"handler"`
	// Root is the local filesystem directory to serve, e.g.
	// "/var/lib/levelrail-data/static/docs/abc1234". Caddy resolves
	// requests underneath it directly; there is no upstream to dial and
	// nothing here to route through Docker.
	Root string `json:"root,omitempty"`
}

// NewFileServerHandler builds the one handler shape a static route
// needs: serve files from root for everything matched by the route.
func NewFileServerHandler(root string) FileServerHandler {
	return FileServerHandler{Handler: "file_server", Root: root}
}

// ProxyRoute is one reverse-proxy backend routed by hostname within a
// Server potentially shared with other routes. Unlike ProxyOptions
// (BuildProxyConfig), where Hosts is optional (an empty Hosts list means
// "match everything on this listener," valid for a single-backend
// listener), a Route here always requires host matching: hostname is the
// only thing that disambiguates two different backends sharing one
// listener.
type ProxyRoute struct {
	// Hosts are the Host header values routed to BackendDial. Also
	// contributes to the Subjects list used for TLS automation when
	// RoutesOptions.TLS is true.
	Hosts []string
	// BackendDial is the reverse-proxy target, e.g. "127.0.0.1:9090".
	BackendDial string
}

// StaticRoute is one static site routed by hostname within a Server
// potentially shared with other routes and with ProxyRoutes: a static
// site and a container service can be routed by the very same Caddy
// server, on different hostnames, because both ultimately produce a
// Route in the same Routes slice, just with a different Handle
// (FileServerHandler instead of ReverseProxyHandler). This is the
// shape that lets a static site bypass the container reconciler
// entirely, not a parallel ingress mechanism.
type StaticRoute struct {
	// Hosts are the Host header values served from RootDir. Also
	// contributes to the Subjects list used for TLS automation when
	// RoutesOptions.TLS is true.
	Hosts []string
	// RootDir is the local filesystem directory to serve.
	RootDir string
}

// RoutesOptions is the input to BuildRoutesConfig: everything needed to
// stand up one Caddy server carrying many independently host-routed
// backends on a single shared listener. This is the shape a real ingress
// controller needs (TASKS.md 1.6): ADR 005's Verified section found that
// caddy.Load replaces Caddy's entire process-wide config on every call,
// so a controller tracking many services builds one complete Config from
// every currently routable service and applies it whole on every
// reconcile, rather than incrementally updating a single service's
// route. BuildProxyConfig, by contrast, only ever builds a single route
// to a single backend and stays as-is for that narrower spike use case.
type RoutesOptions struct {
	// ServerName keys the server within apps.http.servers. Arbitrary,
	// used only for logging/introspection.
	ServerName string
	// ListenAddr is a Caddy network address shared by every route, e.g.
	// ":443".
	ListenAddr string
	// Routes is every reverse-proxy backend to route on this listener.
	// Empty is valid: it produces a listener with no routes and no TLS
	// automation policy, the normal shape for a reconcile pass over zero
	// currently-routable services, not an error condition.
	Routes []ProxyRoute
	// StaticRoutes is every static site to serve directly on this same
	// listener, alongside Routes: one shared Caddy server can carry both
	// reverse-proxy and file_server routes, disambiguated by Host the
	// same way two ProxyRoutes already are. Empty (the default) is valid
	// for exactly the same reason an empty Routes is.
	StaticRoutes []StaticRoute
	// TLS, if true, adds a tls app automation policy scoped to every
	// route's Hosts, both Routes and StaticRoutes (skipped if both are
	// empty, since automatic HTTPS needs at least one subject to issue a
	// certificate for), and disables the HTTP->HTTPS redirect (see
	// Server.AutomaticHTTPS).
	TLS bool
	// AdminListen overrides Caddy's admin API bind address. Empty keeps
	// Caddy's own default (localhost:2019).
	AdminListen string
	// StorageDir overrides Caddy's storage root. Empty keeps Caddy's own
	// OS-specific default. See FileStorage's doc comment. Ignored when
	// CertStorage is set.
	StorageDir string
	// CertStorage, if non-nil, overrides StorageDir with an arbitrary
	// Caddy storage module reference (e.g. NewSQLiteStorageRef()),
	// letting the caller point Caddy's certificate/ACME-account storage
	// at internal/store's SQLite (TASKS.md 3.6) instead of the local
	// filesystem, so certificate storage lives in the database and
	// multi-node deployments can share cert state. Takes precedence over
	// StorageDir when both are set, rather than being an error, so a
	// caller migrating from one to the other doesn't need a separate
	// code path just to clear the old field.
	CertStorage any
}

// BuildRoutesConfig builds a Config with one server carrying one route
// per entry in opts.Routes (reverse_proxy) and one per entry in
// opts.StaticRoutes (file_server), each matched by its own Hosts. Both
// kinds share the same listener and the same TLS automation policy;
// nothing about a static route requires a different Server or a second
// Caddy config document. opts.Routes and opts.StaticRoutes both being
// empty is valid and produces a Config with a listener but no routes and
// no TLS app: this is the normal shape for a reconcile pass over zero
// currently-routable resources (every known service or static site
// either declares no domains or, for a container service, has no
// running container yet), not an error.
func BuildRoutesConfig(opts RoutesOptions) (*Config, error) {
	if opts.ServerName == "" {
		return nil, fmt.Errorf("ingress: build routes config: server name is required")
	}
	if opts.ListenAddr == "" {
		return nil, fmt.Errorf("ingress: build routes config: listen address is required")
	}

	var allHosts []string
	routes := make([]Route, 0, len(opts.Routes)+len(opts.StaticRoutes))
	for i, r := range opts.Routes {
		if len(r.Hosts) == 0 {
			return nil, fmt.Errorf("ingress: build routes config: route %d has no hosts", i)
		}
		if r.BackendDial == "" {
			return nil, fmt.Errorf("ingress: build routes config: route %d has no backend dial address", i)
		}
		routes = append(routes, Route{
			Match:  []Matcher{{Host: r.Hosts}},
			Handle: []any{NewReverseProxyHandler(r.BackendDial)},
		})
		allHosts = append(allHosts, r.Hosts...)
	}
	for i, r := range opts.StaticRoutes {
		if len(r.Hosts) == 0 {
			return nil, fmt.Errorf("ingress: build routes config: static route %d has no hosts", i)
		}
		if r.RootDir == "" {
			return nil, fmt.Errorf("ingress: build routes config: static route %d has no root directory", i)
		}
		routes = append(routes, Route{
			Match:  []Matcher{{Host: r.Hosts}},
			Handle: []any{NewFileServerHandler(r.RootDir)},
		})
		allHosts = append(allHosts, r.Hosts...)
	}

	server := &Server{
		Listen: []string{opts.ListenAddr},
		Routes: routes,
	}

	cfg := &Config{
		Apps: Apps{
			HTTP: &HTTPApp{
				Servers: map[string]*Server{
					opts.ServerName: server,
				},
			},
		},
	}
	cfg.Admin = newAdminConfig(opts.AdminListen)
	switch {
	case opts.CertStorage != nil:
		cfg.Storage = opts.CertStorage
	case opts.StorageDir != "":
		cfg.Storage = NewFileStorage(opts.StorageDir)
	}

	if opts.TLS && len(allHosts) > 0 {
		// See Server.AutomaticHTTPS: skip the HTTP->HTTPS redirect,
		// which would otherwise try to bind Caddy's default HTTP port
		// (80) and needs root.
		server.AutomaticHTTPS = &AutoHTTPSConfig{DisableRedir: true}
		cfg.Apps.TLS = internalIssuerTLSApp(allHosts)
		cfg.Apps.PKI = newInternalPKIApp()
	}

	return cfg, nil
}
