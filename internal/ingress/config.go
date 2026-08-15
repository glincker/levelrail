// Package ingress is the Phase 0 spike for embedding Caddy as a Go
// library, driven entirely through its in-process admin API. There is
// no `caddy` binary anywhere in this package and no Caddyfile on disk.
// Config goes in as a JSON document built from typed Go structs (never a
// hand-written Caddyfile string), because Phase 1's real requirement is
// building that document programmatically from app.yaml (the declarative
// app spec's contract), not editing config by hand.
//
// This package intentionally defines its own minimal structs mirroring the
// slice of Caddy's JSON config schema this spike needs (admin, apps.http,
// apps.tls), rather than importing caddy's own config types directly.
// Caddy's own types lean heavily on json.RawMessage for every extension
// point (module namespaces), which makes them awkward to construct
// directly from Go; a small hand-rolled struct set that marshals to the
// same wire shape is both easier to build from app.yaml data later and
// easier to table-test in isolation, without starting Caddy at all. See
// docs-local/research/caddy-spike.md for what this traded off.
package ingress

import "fmt"

// Config is the root of a Caddy JSON config document
// (https://caddyserver.com/docs/json/), scoped to the subset this spike
// exercises: the admin endpoint, the http app, and the tls app.
type Config struct {
	Admin *AdminConfig `json:"admin,omitempty"`
	// Storage selects the Caddy storage module certificates, ACME
	// account state, and the internal issuer's local CA are persisted
	// through. Typed any rather than a single concrete struct for the
	// same reason Route.Handle and AutomationPolicy.Issuers already are
	// in this file: it is a Caddy module reference, and Caddy's module
	// system is inherently polymorphic (any value marshaling to
	// {"module": "<id>", ...} is valid here). *FileStorage (the local
	// filesystem, via NewFileStorage) and SQLiteStorageRef
	// (internal/store's SQLite, via NewSQLiteStorageRef, TASKS.md 3.6)
	// are this package's two concrete options as of this writing; a nil
	// Storage leaves Caddy's own OS-specific default in place.
	Storage any  `json:"storage,omitempty"`
	Apps    Apps `json:"apps"`
}

// AdminConfig configures Caddy's admin API endpoint. Leaving Listen empty
// (the zero value) means "don't set this field" (omitempty), which lets
// Caddy apply its own default of localhost:2019 rather than us duplicating
// that default here.
type AdminConfig struct {
	// Listen is the admin API bind address. Empty means Caddy's default,
	// localhost:2019.
	Listen string `json:"listen,omitempty"`
	// Disabled, if true, turns the admin listener off entirely. This
	// spike leaves it false (the real default) so the admin endpoint
	// actually comes up, since a production driver would want it
	// reachable for introspection even though this package itself talks
	// to Caddy via Go function calls (caddy.Load), not HTTP requests to
	// that listener. See docs-local/research/caddy-spike.md for what
	// happened when this collided with an already-bound port.
	Disabled bool `json:"disabled,omitempty"`
	// Config holds admin-level config-management settings, notably
	// Persist. See AdminConfigSettings.
	Config *AdminConfigSettings `json:"config,omitempty"`
}

// AdminConfigSettings mirrors Caddy's admin.config JSON field.
type AdminConfigSettings struct {
	// Persist controls whether Caddy keeps its own on-disk copy of
	// whatever config was last pushed to it (Caddy's default is true,
	// written to a fixed OS-level app-data path independent of the
	// storage module configured for certificates, see FileStorage's doc
	// comment). This spike found that surprising: it means an embedded
	// Caddy instance maintains a second, out-of-band record of its own
	// config on disk even when the storage module is pointed elsewhere,
	// which is exactly the "second config surface" ADR 005 argues
	// against embedding Caddy specifically to avoid. This spike's
	// builder pins it to false: the platform's own reconcile loop and
	// database are the single source of truth for desired ingress
	// state, not a file Caddy manages on its own. See
	// docs-local/research/caddy-spike.md.
	Persist *bool `json:"persist,omitempty"`
}

// Apps holds the top-level Caddy apps this package configures.
type Apps struct {
	HTTP *HTTPApp `json:"http,omitempty"`
	TLS  *TLSApp  `json:"tls,omitempty"`
	PKI  *PKIApp  `json:"pki,omitempty"`
}

// HTTPApp is Caddy's "http" app: one or more named servers, each with its
// own listeners and routes.
type HTTPApp struct {
	Servers map[string]*Server `json:"servers,omitempty"`
}

// Server is a single HTTP(S) listener plus the routes it serves.
type Server struct {
	Listen []string `json:"listen,omitempty"`
	Routes []Route  `json:"routes,omitempty"`

	// AutomaticHTTPS controls Caddy's automatic-HTTPS behavior for this
	// server specifically. This spike disables the HTTP->HTTPS redirect
	// (DisableRedir) for the TLS server: automatic redirects bind Caddy's
	// default HTTP port (80), which needs root and would surprise a local
	// spike run under a normal user, so the standalone TLS demo makes
	// that trade-off explicit instead of silently trying to grab port 80.
	AutomaticHTTPS *AutoHTTPSConfig `json:"automatic_https,omitempty"`
}

// AutoHTTPSConfig mirrors Caddy's automatic_https server field. See the
// Server.AutomaticHTTPS doc comment for why this spike sets DisableRedir.
type AutoHTTPSConfig struct {
	Disabled     bool `json:"disable,omitempty"`
	DisableRedir bool `json:"disable_redirects,omitempty"`
}

// Route is one matcher-set-plus-handlers pair, in Caddy's route shape.
type Route struct {
	Match  []Matcher `json:"match,omitempty"`
	Handle []any     `json:"handle"`
}

// Matcher is a single Caddy request matcher set. Only the host matcher is
// needed for this spike; more (path, header, etc.) would be added the same
// way once Phase 1 needs them.
type Matcher struct {
	Host []string `json:"host,omitempty"`
}

// ReverseProxyHandler is Caddy's "reverse_proxy" handler
// (http.handlers.reverse_proxy). Handler is always the literal string
// "reverse_proxy"; it is a field rather than a constant embedded via
// MarshalJSON so the struct stays a plain, table-testable value.
type ReverseProxyHandler struct {
	Handler   string     `json:"handler"`
	Upstreams []Upstream `json:"upstreams"`
}

// Upstream is a single reverse-proxy backend target.
type Upstream struct {
	// Dial is a host:port address, e.g. "127.0.0.1:8080". Caddy's own
	// docs note this must resolve to exactly one socket, no port ranges.
	Dial string `json:"dial"`
}

// NewReverseProxyHandler builds the one handler shape this spike needs:
// proxy everything matched by the route to a single backend.
func NewReverseProxyHandler(backendDial string) ReverseProxyHandler {
	return ReverseProxyHandler{
		Handler:   "reverse_proxy",
		Upstreams: []Upstream{{Dial: backendDial}},
	}
}

// TLSApp is Caddy's "tls" app: certificate automation policy.
type TLSApp struct {
	Automation *Automation `json:"automation,omitempty"`
}

// Automation holds the ordered list of automation policies. The first
// policy whose Subjects match a given hostname wins.
type Automation struct {
	Policies []AutomationPolicy `json:"policies,omitempty"`
}

// AutomationPolicy binds a set of subjects (hostnames) to the issuer(s)
// allowed to obtain certificates for them.
type AutomationPolicy struct {
	Subjects []string `json:"subjects,omitempty"`
	Issuers  []any    `json:"issuers,omitempty"`
}

// InternalIssuer is Caddy's "internal" TLS issuer (tls.issuance.internal):
// a locally-generated, self-signed CA, meant for exactly the case this
// spike is in, no public domain and no inbound port 80/443 reachable from
// the internet for real ACME. It works fully offline. See
// docs-local/research/caddy-spike.md for why this stands in for real ACME
// in this spike and what still needs verifying against a real domain.
type InternalIssuer struct {
	Module string `json:"module"`
}

// NewInternalIssuer returns the JSON shape for Caddy's internal issuer
// module, used as an AutomationPolicy's Issuers entry.
func NewInternalIssuer() InternalIssuer {
	return InternalIssuer{Module: "internal"}
}

// ACMEIssuer is Caddy's "acme" TLS issuer (tls.issuance.acme): real ACME
// issuance (Let's Encrypt or any RFC 8555-compatible CA), as opposed to
// InternalIssuer's fully offline, self-signed certificates. Field names
// and the "module" value ("acme", resolved within the tls.issuance
// namespace the same way InternalIssuer's "internal" is, per
// AutomationPolicy.Issuers' inline_key=module tag) are verified against
// the vendored source for the pinned caddyserver/caddy/v2 version this
// module uses (modules/caddytls/acmeissuer.go's ACMEIssuer struct and
// its CaddyModule ID "tls.issuance.acme"), not guessed: this package
// intentionally hand-rolls the wire-compatible subset of Caddy's own
// config types rather than importing them directly (see this file's
// package doc comment), and ACMEIssuer only needs the three fields an
// operator-facing settings form actually collects (email, an optional
// directory URL override, and the module discriminator), not the full
// surface (challenges config, account key, EAB, and so on) Caddy's own
// struct exposes for cases this codebase doesn't build a UI for yet.
//
// ADR 005's Verified section is explicit that only the internal issuer
// path was proven end to end by that spike; this type exists to close
// that named, expected gap (see internal/api/certificates.go's own doc
// comment referencing the same open item), not to introduce a new
// architecture decision.
type ACMEIssuer struct {
	Module string `json:"module"`
	// Email is the ACME account contact address, Caddy's own "email"
	// field verbatim. Not technically required by the ACME protocol
	// itself, but this codebase's own settings validation
	// (internal/api) requires a non-empty, syntactically valid address
	// whenever ACME is enabled: an unreachable ACME account is exactly
	// the kind of "renewal fails silently" risk this project treats as
	// its central one to catch before it bites a real user.
	Email string `json:"email,omitempty"`
	// CA is Caddy's own "ca" field: the ACME directory endpoint URL.
	// Empty leaves Caddy's compiled-in default in place, Let's Encrypt's
	// real production directory
	// (https://acme-v02.api.letsencrypt.org/directory, per
	// caddytls.ACMEIssuer's own doc comment). An operator who wants to
	// avoid Let's Encrypt's production rate limits while testing should
	// set this explicitly to Let's Encrypt's staging directory
	// (https://acme-staging-v02.api.letsencrypt.org/directory); this
	// package does not default to staging on its own, since silently
	// issuing a staging (browser-untrusted) certificate for what an
	// operator believes is a production toggle would be a worse
	// surprise than requiring them to opt in explicitly.
	CA string `json:"ca,omitempty"`
}

// NewACMEIssuer returns the JSON shape for Caddy's real ACME issuer
// module, used as an AutomationPolicy's Issuers entry in place of
// NewInternalIssuer when an operator has opted into real ACME
// (internal/store.IngressSettings.ACMEEnabled). directoryURL empty means
// "use Caddy's own default" (see ACMEIssuer.CA's doc comment); email
// empty is accepted by this constructor (it does not itself enforce the
// non-empty rule), since that validation belongs to the caller that owns
// the operator-facing form (internal/api), not to this low-level config
// builder.
func NewACMEIssuer(email, directoryURL string) ACMEIssuer {
	return ACMEIssuer{Module: "acme", Email: email, CA: directoryURL}
}

// FileStorage is Caddy's default storage module
// (caddy.storage.file_system): certificates, ACME account state, and the
// internal issuer's local CA all live under Root. Caddy's own default,
// when Storage is left nil, is an OS-specific application-data directory
// outside the project entirely (e.g. ~/Library/Application Support/Caddy
// on macOS) which is the right thing for the `caddy` CLI on a developer's
// own machine, and the wrong thing for an embedded control plane: it
// scatters state outside the platform's own data directory and, worse, is
// shared and reused across every unrelated process on the machine that
// also happens to embed Caddy with default settings. See
// docs-local/research/caddy-spike.md.
type FileStorage struct {
	Module string `json:"module"`
	Root   string `json:"root,omitempty"`
}

// NewFileStorage returns the JSON shape for Caddy's file-system storage
// module rooted at dir. Module is "file_system", not the fully-qualified
// "caddy.storage.file_system": the "storage" field resolves modules
// within the caddy.storage namespace already (see StorageRaw's caddy
// struct tag in caddy's own Config type), so the namespace prefix here
// would be doubled up, a real surprise this spike hit and is recording,
// see docs-local/research/caddy-spike.md.
func NewFileStorage(dir string) *FileStorage {
	return &FileStorage{Module: "file_system", Root: dir}
}

// PKIApp is Caddy's "pki" app: the certificate authorities available to
// issuers like InternalIssuer.
type PKIApp struct {
	CertificateAuthorities map[string]*CA `json:"certificate_authorities,omitempty"`
}

// CA configures a single certificate authority managed by the pki app.
type CA struct {
	// InstallTrust controls whether Caddy attempts to install this CA's
	// root certificate into the local OS/browser trust store on first
	// use. Caddy's own default is true, which on this spike's dev
	// machine meant an unprompted `sudo` invocation that failed
	// noisily (no interactive terminal, no `certutil`) but did not
	// block certificate issuance. That's tolerable on a developer
	// laptop; it is the wrong default for a headless server process,
	// which has no desktop trust store to install into and should
	// never attempt an interactive sudo prompt at startup. This spike's
	// TLS builder pins it to false explicitly rather than relying on
	// Caddy's default. See docs-local/research/caddy-spike.md.
	InstallTrust *bool `json:"install_trust,omitempty"`
}

// ProxyOptions is the input to BuildProxyConfig: everything needed to
// stand up one reverse-proxy route, with or without TLS.
type ProxyOptions struct {
	// ServerName keys the server within apps.http.servers. Arbitrary,
	// used only for logging/introspection.
	ServerName string
	// ListenAddr is a Caddy network address, e.g. ":8080" or
	// "127.0.0.1:8443".
	ListenAddr string
	// BackendDial is the reverse-proxy target, e.g. "127.0.0.1:9090".
	BackendDial string
	// Hosts, if non-empty, restricts the route to these Host header
	// values and is also the Subjects list used for TLS automation when
	// TLS is enabled. Empty means "match every request on this
	// listener", which is only valid for the plain-HTTP case: Caddy's
	// automatic HTTPS needs at least one qualifying hostname to know
	// what to issue a certificate for.
	Hosts []string
	// TLS, if true, adds a tls app automation policy scoping the
	// internal issuer to Hosts, and disables the HTTP->HTTPS redirect
	// (see Server.AutomaticHTTPS).
	TLS bool
	// AdminListen overrides Caddy's admin API bind address. Empty keeps
	// Caddy's own default (localhost:2019).
	AdminListen string
	// StorageDir overrides Caddy's storage root (certificates, ACME
	// account state, the internal issuer's local CA). Empty keeps
	// Caddy's own OS-specific default. See FileStorage's doc comment for
	// why Phase 1 should always set this rather than rely on the
	// default.
	StorageDir string
}

// BuildProxyConfig builds a Config with exactly one server and one route:
// reverse-proxy every matching request to opts.BackendDial. This is the
// pure, no-Caddy-required half of the spike, table-tested in
// config_test.go; driver_test.go covers the half that actually starts
// Caddy and proxies a real request through it.
func BuildProxyConfig(opts ProxyOptions) (*Config, error) {
	if opts.ServerName == "" {
		return nil, fmt.Errorf("ingress: build proxy config: server name is required")
	}
	if opts.ListenAddr == "" {
		return nil, fmt.Errorf("ingress: build proxy config: listen address is required")
	}
	if opts.BackendDial == "" {
		return nil, fmt.Errorf("ingress: build proxy config: backend dial address is required")
	}
	if opts.TLS && len(opts.Hosts) == 0 {
		return nil, fmt.Errorf("ingress: build proxy config: TLS requires at least one host, automatic HTTPS has no subject to issue a certificate for otherwise")
	}

	route := Route{
		Handle: []any{NewReverseProxyHandler(opts.BackendDial)},
	}
	if len(opts.Hosts) > 0 {
		route.Match = []Matcher{{Host: opts.Hosts}}
	}

	server := &Server{
		Listen: []string{opts.ListenAddr},
		Routes: []Route{route},
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
	if opts.StorageDir != "" {
		cfg.Storage = NewFileStorage(opts.StorageDir)
	}

	if opts.TLS {
		// See Server.AutomaticHTTPS: skip the HTTP->HTTPS redirect,
		// which would otherwise try to bind Caddy's default HTTP port
		// (80) and needs root.
		server.AutomaticHTTPS = &AutoHTTPSConfig{DisableRedir: true}
		cfg.Apps.TLS = internalIssuerTLSApp(opts.Hosts)
		cfg.Apps.PKI = newInternalPKIApp()
	}

	return cfg, nil
}

// newAdminConfig builds the AdminConfig every builder in this package
// uses: Persist always disabled (see AdminConfigSettings.Persist), Listen
// left at Caddy's own default unless the caller overrides it.
func newAdminConfig(adminListen string) *AdminConfig {
	noPersist := false
	admin := &AdminConfig{Config: &AdminConfigSettings{Persist: &noPersist}}
	if adminListen != "" {
		admin.Listen = adminListen
	}
	return admin
}

// internalIssuerTLSApp builds a TLSApp with a single automation policy
// scoping Caddy's internal issuer to hosts. See InternalIssuer's doc
// comment for why this is the only issuer this package builds; real ACME
// is explicitly unverified per ADR 005.
func internalIssuerTLSApp(hosts []string) *TLSApp {
	return &TLSApp{
		Automation: &Automation{
			Policies: []AutomationPolicy{
				{
					Subjects: hosts,
					Issuers:  []any{NewInternalIssuer()},
				},
			},
		},
	}
}

// acmeIssuerTLSApp builds a TLSApp with a single automation policy
// scoping Caddy's real ACME issuer to hosts. Mirrors
// internalIssuerTLSApp's shape exactly, swapping NewInternalIssuer for
// NewACMEIssuer; unlike the internal issuer path, this never pairs with
// a PKIApp, real ACME has no local CA for Caddy's pki app to manage
// trust for.
func acmeIssuerTLSApp(hosts []string, email, directoryURL string) *TLSApp {
	return &TLSApp{
		Automation: &Automation{
			Policies: []AutomationPolicy{
				{
					Subjects: hosts,
					Issuers:  []any{NewACMEIssuer(email, directoryURL)},
				},
			},
		},
	}
}

// newInternalPKIApp builds a PKIApp for the internal issuer's local CA
// with InstallTrust always disabled. See CA.InstallTrust's doc comment:
// a headless control plane must never attempt an interactive sudo
// trust-store install.
func newInternalPKIApp() *PKIApp {
	noInstallTrust := false
	return &PKIApp{
		CertificateAuthorities: map[string]*CA{
			"local": {InstallTrust: &noInstallTrust},
		},
	}
}
