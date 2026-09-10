// Package ingress implements the ingress controller: the
// reconcile.Controller that keeps Caddy's config (internal/ingress, ADR
// 005) in sync with every service that declares domains.
//
// ADR 005's Verified section is the load-bearing finding this controller
// is built around: caddy.Load (what Driver.Apply calls) replaces Caddy's
// entire process-wide config on every call, there is no per-app
// incremental update. So unlike the application controller
// (internal/reconcile/application), which reconciles one named service
// at a time, this controller reconciles all of them in a single pass:
// every Reconcile call lists every desired service, derives a complete
// desired ingress.Config from whichever of them currently have a running
// backend, and applies that whole document. This fits the reconciler
// contract's level-triggered philosophy directly (re-derive from current
// state every time, assume nothing about a previous call), it just means the
// unit of reconciliation for ingress is "all routable services," not one
// resource.
//
// A follow-up this package's own doc comment used to flag as open is now
// closed: real ACME (ADR 005's Verified section names this exact gap,
// "must be spot-checked against a real domain... not assumed to follow
// automatically") is available as an opt-in, platform-wide toggle
// (store.IngressSettings.ACMEEnabled, read fresh every Reconcile via
// ServiceStore.GetIngressSettings), not the default: Caddy's internal
// (self-signed, offline) issuer remains what every route gets unless an
// operator explicitly turns ACME on through PUT /api/v1/settings/ingress
// (internal/api), so an upgrading deployment's behavior is unchanged
// until it opts in. When enabled, every currently-routed host shares one
// automation policy pointed at real ACME instead of the internal issuer;
// see internal/ingress.BuildRoutesConfig's own doc comment on
// RoutesOptions.ACMEEnabled for why this pass deliberately does not mix
// ACME and internal issuers across different hosts in the same reconcile.
//
// The platform primary domain (store.IngressSettings.PrimaryDomain) is
// this same settings row's second, independent field: when set, this
// controller adds one more reverse-proxy route (WithDashboardDial) for
// the control plane's own dashboard, sharing the same claimedHosts dedup
// and the same TLS automation policy as every app/static-site route.
//
// Two further gaps this package's own doc comment used to flag as open
// here are also closed:
//
//   - Certificate storage no longer has to stay on Caddy's default
//     file-system storage module (internal/ingress.FileStorage).
//     WithCertStore points it at internal/ingress.SQLiteStorage instead,
//     a certmagic.Storage backed by internal/store's SQLite, so
//     multiple ingress-driving processes sharing that database share
//     cert state, per the ingress design's requirement that certificate
//     storage live in the database rather than each independently
//     re-obtaining a certificate for a domain another one already has.
//   - Domain uniqueness across separate deploys over time (not just
//     within a single app.yaml, which internal/spec's Validate() already
//     enforced) is now enforced where desired state is actually
//     written: store.SaveDesiredService rejects a save with
//     *store.ErrDomainTaken if any domain is already claimed by a
//     different service (see migrations/0011_service_domains.sql). This
//     controller also carries a defense-in-depth guard in Reconcile
//     (firstDuplicateHost) for any pre-existing data written before that
//     constraint existed: it never builds two routes for the same host
//     in one pass, logging and skipping whichever service loses the
//     race within this reconcile.
package ingress

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"golang.org/x/crypto/bcrypt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ServiceStore is the narrow surface this controller needs from
// internal/store, so tests can fake it without a real database.
// *store.DB satisfies this. Unlike application.ServiceStore (one named
// service), this controller needs every known service in one call: see
// the package doc comment for why ingress reconciles as a single whole
// pass rather than per-service.
//
// ListStaticSites is the static-site bypass of the
// container reconciler (migrations/0015_static_sites.sql,
// internal/store.StaticSite): a static site is never a
// store.DesiredService (no image, no port, nothing to converge to a
// running container), so it needs its own listing call, read fresh on
// every Reconcile exactly like ListDesiredServices, rather than being
// threaded through the container-service shape as a special case.
// GetIngressSettings is read fresh on every Reconcile, the same
// "never cache, re-derive from current state every call" principle
// ListDesiredServices/ListStaticSites already follow: an operator
// toggling ACME on or setting a primary domain through PUT
// /api/v1/settings/ingress (internal/api) must take effect on this
// controller's very next reconcile pass, not require a restart.
type ServiceStore interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListStaticSites(ctx context.Context) ([]store.StaticSite, error)
	GetIngressSettings(ctx context.Context) (store.IngressSettings, error)
	GetCloudflareDNSSettings(ctx context.Context) (store.CloudflareDNSSettings, error)
	// ListDomainBasicAuth returns every domain currently protected by
	// HTTP Basic Auth (store.DomainBasicAuth, migrations/0052), read
	// fresh every Reconcile like everything else on this interface: an
	// operator setting or clearing a domain's basic auth through PUT/
	// DELETE /api/v1/apps/{name}/domains/{domain}/auth (internal/api)
	// must take effect on this controller's very next pass.
	ListDomainBasicAuth(ctx context.Context) ([]store.DomainBasicAuth, error)
	// ListDomainMaintenance returns every domain currently in
	// maintenance mode (migrations/0080), read fresh every Reconcile
	// for the same reason ListDomainBasicAuth is: an operator setting
	// or clearing maintenance mode through PUT/DELETE
	// /api/v1/apps/{name}/domains/{domain}/maintenance (internal/api)
	// must take effect on this controller's very next pass.
	ListDomainMaintenance(ctx context.Context) ([]string, error)
	// ListDomainTLSCerts returns every domain currently configured with a
	// BYO TLS certificate (migrations/0084), read fresh every Reconcile
	// for the same reason ListDomainBasicAuth is: an operator setting or
	// clearing a domain's certificate through PUT/DELETE
	// /api/v1/apps/{name}/domains/{domain}/tls-cert (internal/api) must
	// take effect on this controller's very next pass.
	ListDomainTLSCerts(ctx context.Context) ([]store.DomainTLSCert, error)
	// GetRegistrySettings returns the built-in registry's single
	// platform-wide row (store.RegistrySettings), read fresh every
	// Reconcile like GetIngressSettings: an operator enabling the
	// registry or changing its Host through PUT /api/v1/settings/registry
	// (internal/api) must take effect on this controller's very next
	// pass.
	GetRegistrySettings(ctx context.Context) (store.RegistrySettings, error)
	// ListDomainWAF returns every domain with opt-in WAF and/or rate
	// limiting configured (migrations/0091), read fresh every Reconcile
	// for the same reason ListDomainBasicAuth is: an operator setting or
	// clearing a domain's WAF/rate-limit config through PUT/DELETE
	// /api/v1/apps/{name}/domains/{domain}/waf (internal/api) must take
	// effect on this controller's very next pass. Unlike basic auth or
	// BYO TLS certs, there is no secret material here, so no resolver
	// option is needed to actually enforce it.
	ListDomainWAF(ctx context.Context) ([]store.DomainWAF, error)
}

// Applier is the narrow surface this controller needs from
// internal/ingress.Driver, so tests can fake it without starting a real
// Caddy instance. *ingress.Driver satisfies this.
type Applier interface {
	Apply(ctx context.Context, cfg *ingress.Config) error
}

// CloudflareDNSTokenResolver is the narrow surface this controller needs
// from internal/secrets.Manager to resolve the Cloudflare DNS-01 API
// token, mirroring cloudflaretunnel.TokenResolver's exact shape.
// *secrets.Manager satisfies this structurally.
type CloudflareDNSTokenResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// DomainBasicAuthPasswordResolver is the narrow surface this controller
// needs from internal/secrets.Manager to resolve a domain's basic-auth
// password, structurally identical to CloudflareDNSTokenResolver but
// named separately since the two resolve unrelated credentials under
// different serviceName namespaces.
type DomainBasicAuthPasswordResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// DomainTLSCertPEMResolver is the narrow surface this controller needs
// from internal/secrets.Manager to resolve a domain's BYO certificate
// and private key PEM text, structurally identical to
// DomainBasicAuthPasswordResolver but named separately since the two
// resolve unrelated material under different serviceName namespaces.
type DomainTLSCertPEMResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

const (
	defaultServerName = "levelrail-ingress"
	defaultListenAddr = ":443"

	// defaultAdminListen pins Caddy's admin API to loopback only. Caddy's
	// own default is already localhost:2019 (see internal/ingress's
	// AdminConfig doc comment), so this doesn't change behavior; it makes
	// the choice explicit in code rather than relying on an upstream
	// default that could change. The admin listener itself is left
	// enabled (not Disabled: true): local introspection into a running
	// Caddy instance is genuinely useful for debugging a stuck ingress
	// reconcile, and binding it to loopback only means it is never
	// reachable off the machine the control plane runs on, so there is
	// no real exposure to accept in exchange for that.
	defaultAdminListen = "localhost:2019"

	// dashboardRouteOwner is the pseudo-owner name the platform primary
	// domain's route claims in claimedHosts (firstDuplicateHost's
	// "name" parameter), so a conflict with an app or static site domain
	// logs something more useful than an empty string. Not a real
	// service or static site name and never looked up against either
	// table.
	dashboardRouteOwner = "platform dashboard"

	// registryRouteOwner is dashboardRouteOwner's exact counterpart for
	// the built-in registry's route.
	registryRouteOwner = "builtin registry"
)

// Controller converges Caddy's config to match every service in
// ServiceStore that declares Domains and currently has a running
// container. Desired state is read fresh from ServiceStore on every
// Reconcile, never cached.
type Controller struct {
	store   ServiceStore
	runtime docker.Runtime
	driver  Applier
	logger  *slog.Logger

	serverName  string
	listenAddr  string
	adminListen string
	storageDir  string

	// dashboardDial is the control plane's own dashboard bind address
	// (see WithDashboardDial), reverse-proxied to whenever
	// store.IngressSettings.PrimaryDomain is set. Empty means "no
	// dashboard route", the default, matching how every currently
	// existing deployment has no such route today.
	dashboardDial string

	// registryDial is the built-in registry container's loopback dial
	// address (see WithRegistryDial), reverse-proxied to whenever
	// store.RegistrySettings.Enabled and .Host are both set. Empty means
	// "no registry route", the default, the same shape dashboardDial's
	// own absence already has.
	registryDial string

	// certStore, if set via WithCertStore, is built into a
	// *ingress.SQLiteStorage and registered with ingress.SetActiveCertStorage
	// once, in New, rather than on every Reconcile. certStorage is the
	// resulting ingress.RoutesOptions.CertStorage value; nil means
	// "storageDir/file storage instead" (see WithCertStore).
	certStore   ingress.CertStore
	certStorage any

	// dnsTokens, if set via WithCloudflareDNSTokens, is resolved fresh
	// every Reconcile pass whenever store.CloudflareDNSSettings.Enabled
	// is true, and threaded through to
	// ingress.RoutesOptions.CloudflareDNSAPIToken. Nil (the default)
	// means no wildcard domain ever gets Cloudflare DNS-01, unchanged
	// from this controller's behavior before this field existed.
	dnsTokens CloudflareDNSTokenResolver

	// basicAuthSecrets, if set via WithDomainBasicAuthSecrets, is
	// resolved fresh every Reconcile pass for every domain returned by
	// ServiceStore.ListDomainBasicAuth. Nil (the default) means no
	// domain ever gets a basic_auth handler, unchanged from this
	// controller's behavior before this feature existed.
	basicAuthSecrets DomainBasicAuthPasswordResolver

	// tlsCertSecrets, if set via WithDomainTLSCertSecrets, is resolved
	// fresh every Reconcile pass for every domain returned by
	// ServiceStore.ListDomainTLSCerts. Nil (the default) means no domain
	// ever gets a BYO certificate loaded, unchanged from this
	// controller's behavior before this feature existed.
	tlsCertSecrets DomainTLSCertPEMResolver
}

// Option configures optional Controller behavior.
type Option func(*Controller)

// WithServerName overrides the name Caddy's config keys the shared server
// under (apps.http.servers.<name>). Defaults to "levelrail-ingress".
// Arbitrary, used only for logging/introspection.
func WithServerName(name string) Option {
	return func(c *Controller) { c.serverName = name }
}

// WithListenAddr overrides the Caddy network address the shared HTTPS
// listener binds. Defaults to ":443".
func WithListenAddr(addr string) Option {
	return func(c *Controller) { c.listenAddr = addr }
}

// WithAdminListen overrides Caddy's admin API bind address. Defaults to
// "localhost:2019"; see defaultAdminListen's doc comment for why that
// default is loopback-only rather than disabled outright.
func WithAdminListen(addr string) Option {
	return func(c *Controller) { c.adminListen = addr }
}

// WithStorageDir overrides Caddy's certificate/ACME-account storage root
// (internal/ingress.FileStorage). Empty (the default) keeps Caddy's own
// OS-specific default location. Production wiring (not
// yet built) should point this at a path under the control plane's data
// directory once that constant exists; this package does not invent one,
// keeping with the repo's brand/path indirection rule (no hardcoded
// paths outside the shared data-directory constant).
func WithStorageDir(dir string) Option {
	return func(c *Controller) { c.storageDir = dir }
}

// WithCertStore points Caddy's certificate/ACME-account storage at
// internal/store's SQLite instead of the local
// filesystem, so multi-node deployments share cert state instead of each
// node maintaining its own certificate storage. Takes precedence over
// WithStorageDir if both are set. certStore is typically
// the same *store.DB already passed to New as the ServiceStore; New
// builds it into an ingress.SQLiteStorage and registers it via
// ingress.SetActiveCertStorage exactly once, not on every Reconcile.
func WithCertStore(certStore ingress.CertStore) Option {
	return func(c *Controller) { c.certStore = certStore }
}

// WithDashboardDial enables routing the control plane's own dashboard
// through this controller's shared Caddy server whenever an operator
// sets a primary domain (store.IngressSettings.PrimaryDomain, PUT
// /api/v1/settings/ingress). dial is a host:port reverse-proxy target
// for the dashboard's own existing *http.Server (cmd/levelrail's
// httpAddr(), normalized to a loopback dial address the same way
// routeFor already normalizes a service container's published port: the
// dashboard's own bind address is often ":8080", listen-on-every-
// interface shorthand that isn't itself a valid dial target, so the
// caller is expected to pass the loopback-normalized form). Without this
// option (the default), a configured PrimaryDomain is silently not
// routed: this mirrors every other optional Controller capability in
// this file (WithCertStore, and so on) failing closed rather than
// erroring when its prerequisite wiring is absent.
func WithDashboardDial(dial string) Option {
	return func(c *Controller) { c.dashboardDial = dial }
}

// WithRegistryDial enables routing the built-in container registry
// (internal/reconcile/registry) through this controller's shared Caddy
// server whenever an operator enables it with a Host set (PUT
// /api/v1/settings/registry). dial is the registry container's own
// loopback dial address (127.0.0.1 plus registry.HostPort), the same
// "published port, dialed via loopback" shape WithDashboardDial's own
// doc comment establishes. No basic_auth handler is added here: the
// registry container enforces its own htpasswd auth
// (internal/reconcile/registry's own doc comment), so this route is a
// plain TLS-terminating reverse proxy, the same shape the dashboard
// route already has. Without this option (the default), an enabled
// registry with a Host set is silently not routed, matching
// WithDashboardDial's own "fails closed" absence behavior.
func WithRegistryDial(dial string) Option {
	return func(c *Controller) { c.registryDial = dial }
}

// WithCloudflareDNSTokens enables Cloudflare DNS-01 for wildcard domains
// (see internal/ingress.IsWildcardDomain) by resolving the API token
// internal/secrets stores under store.CloudflareDNSSecretsKey(), whenever
// store.CloudflareDNSSettings.Enabled is true. Without this option (the
// default), wildcard hosts reconcile exactly as before this feature
// existed.
func WithCloudflareDNSTokens(resolver CloudflareDNSTokenResolver) Option {
	return func(c *Controller) { c.dnsTokens = resolver }
}

// WithDomainBasicAuthSecrets enables HTTP Basic Auth on any domain
// present in store.DomainBasicAuth by resolving its password through
// internal/secrets.Manager under store.DomainBasicAuthSecretsKey(domain).
// Without this option (the default), domain_basic_auth rows exist in
// the store but are never enforced, the same "fails closed" shape
// WithCloudflareDNSTokens's own absence already has.
func WithDomainBasicAuthSecrets(resolver DomainBasicAuthPasswordResolver) Option {
	return func(c *Controller) { c.basicAuthSecrets = resolver }
}

// WithDomainTLSCertSecrets enables loading a BYO certificate for any
// domain present in store.DomainTLSCert by resolving its certificate and
// private key PEM through internal/secrets.Manager under
// store.DomainTLSCertSecretsKey(domain). Without this option (the
// default), domain_tls_cert rows exist in the store but are never
// loaded, the same "fails closed" shape WithDomainBasicAuthSecrets's own
// absence already has; unlike that case, failing to resolve a BYO
// certificate falls back to Caddy's normal automatic ACME/internal
// issuance for that host rather than leaving it unrouted, since an
// unloadable certificate is an availability problem, not a security
// control to fail closed on the way an unresolvable password is.
func WithDomainTLSCertSecrets(resolver DomainTLSCertPEMResolver) Option {
	return func(c *Controller) { c.tlsCertSecrets = resolver }
}

// WithLogger overrides the logger used for per-service skip decisions.
// Defaults to slog.Default().
func WithLogger(logger *slog.Logger) Option {
	return func(c *Controller) { c.logger = logger }
}

// New builds a Controller.
func New(svcStore ServiceStore, runtime docker.Runtime, driver Applier, opts ...Option) *Controller {
	c := &Controller{
		store:       svcStore,
		runtime:     runtime,
		driver:      driver,
		logger:      slog.Default(),
		serverName:  defaultServerName,
		listenAddr:  defaultListenAddr,
		adminListen: defaultAdminListen,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.certStore != nil {
		// Built after every Option has run, not inside WithCertStore
		// itself, so the SQLiteStorage's logger reflects a later
		// WithLogger call regardless of Option ordering.
		storage := ingress.NewSQLiteStorage(c.certStore, c.logger)
		ingress.SetActiveCertStorage(storage)
		c.certStorage = ingress.NewSQLiteStorageRef()
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "ingress" }

// Reconcile implements reconcile.Controller. It lists every desired
// service and every static site (store.StaticSite, the
// container-free bypass, migrations/0015_static_sites.sql), builds one
// reverse-proxy route per service that both declares domains and
// currently has a running, port-published container, one file_server
// route per static site that declares domains, and applies the
// resulting whole Config to Caddy in a single call (see the package doc
// comment for why this can't be done incrementally). Both kinds of route
// share one claimedHosts pass so a container service and a static site
// can never both route the same host within one apply, the same
// defense-in-depth firstDuplicateHost already gave container services
// alone before static sites existed.
//
// A service with no valid backend right now (mid-deploy, crashed, never
// deployed) is not a reconcile failure: it's simply left out of this
// pass's config, logged at debug level, and picked up automatically once
// a later reconcile finds it running (this controller and the
// application controller both re-run on every Docker event and resync
// tick, per the shared Engine). A static site never has this problem:
// there is no container to wait for, so every static site with domains
// is routed the moment internal/deploy.Pipeline has saved it, on this
// controller's very next reconcile. Only a genuine failure to list
// desired state, build the config, or apply it to Caddy is reported as
// an error.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	services, err := c.store.ListDesiredServices(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list desired services: %w", err)
	}
	staticSites, err := c.store.ListStaticSites(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list static sites: %w", err)
	}
	settings, err := c.store.GetIngressSettings(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: get ingress settings: %w", err)
	}
	authByDomain, err := c.domainBasicAuthByDomain(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list domain basic auth: %w", err)
	}
	maintenanceByDomain, err := c.domainMaintenanceSet(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list domain maintenance: %w", err)
	}
	tlsCertOverrides, err := c.domainTLSCertOverrides(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list domain tls certs: %w", err)
	}
	registrySettings, err := c.store.GetRegistrySettings(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: get registry settings: %w", err)
	}
	wafByDomain, err := c.domainWAFByDomain(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list domain waf: %w", err)
	}

	var routes []ingress.ProxyRoute
	var maintenanceRoutes []ingress.MaintenanceRoute
	claimedHosts := make(map[string]string, len(services)+len(staticSites)) // host -> owning service/static site, this pass only
	for _, svc := range services {
		if len(svc.Domains) == 0 {
			continue
		}
		if owner, host, dup := firstDuplicateHost(svc.Name, svc.Domains, claimedHosts); dup {
			// store.SaveDesiredService now rejects a save
			// that would create this situation for any service written
			// after that change landed, so reaching this branch means
			// either data written before the constraint existed, or a
			// direct write that bypassed SaveDesiredService. Either way,
			// this pass must still never build two routes for the same
			// host: skip the loser and say so loudly, rather than
			// silently letting Caddy's own last-match-wins matcher
			// evaluation decide.
			c.logger.WarnContext(ctx, "ingress: service claims a domain another service already routed this pass, skipping; internal/store's service_domains uniqueness constraint should prevent this for any service saved since that constraint landed, this is a defense-in-depth guard for pre-existing data",
				slog.String("service", svc.Name),
				slog.String("domain", host),
				slog.String("already_routed_to", owner),
			)
			continue
		}

		// A domain in maintenance mode is routed unconditionally, with
		// no dependency on dialForService: "intentionally unavailable
		// right now" must hold even for a service with zero running
		// containers, unlike every other route kind here, which needs a
		// real backend to dial. Only the remaining, non-maintenance
		// hosts still go through the ordinary dial-required path below.
		maintenanceHosts, activeHosts := splitMaintenanceHosts(svc.Domains, maintenanceByDomain)
		if len(maintenanceHosts) > 0 {
			for _, host := range maintenanceHosts {
				claimedHosts[host] = svc.Name
			}
			maintenanceRoutes = append(maintenanceRoutes, ingress.MaintenanceRoute{Hosts: maintenanceHosts})
		}
		if len(activeHosts) == 0 {
			continue
		}

		dial, ok := c.dialForService(ctx, svc)
		if !ok {
			continue
		}
		for _, host := range activeHosts {
			claimedHosts[host] = svc.Name
		}
		routes = append(routes, c.routesForService(ctx, activeHosts, dial, authByDomain, wafByDomain)...)
	}

	var staticRoutes []ingress.StaticRoute
	for _, site := range staticSites {
		if len(site.Domains) == 0 {
			continue
		}
		if owner, host, dup := firstDuplicateHost(site.Name, site.Domains, claimedHosts); dup {
			// Mirrors the container-service guard above: store.SaveStaticSite
			// (migrations/0015) already rejects a save that would create
			// this situation, this is the same defense-in-depth for
			// pre-existing data, now covering the cross-table case too (a
			// static site and a container service claiming the same
			// host).
			c.logger.WarnContext(ctx, "ingress: static site claims a domain another service or static site already routed this pass, skipping",
				slog.String("static_site", site.Name),
				slog.String("domain", host),
				slog.String("already_routed_to", owner),
			)
			continue
		}
		for _, host := range site.Domains {
			claimedHosts[host] = site.Name
		}
		staticRoutes = append(staticRoutes, ingress.StaticRoute{Hosts: site.Domains, RootDir: site.RootDir})
	}

	// Platform primary domain (store.IngressSettings.PrimaryDomain): one
	// more reverse-proxy route to the control plane's own dashboard,
	// participating in the exact same claimedHosts dedup and the exact
	// same TLS automation policy set as every app/static-site route
	// above (ACME if enabled, internal issuer otherwise). Requires both
	// a configured PrimaryDomain and WithDashboardDial having been set;
	// either missing means no dashboard route this pass, the same
	// "fails closed, not an error" shape a service with no running
	// container already has.
	if settings.PrimaryDomain != "" && c.dashboardDial != "" {
		if owner, host, dup := firstDuplicateHost(dashboardRouteOwner, []string{settings.PrimaryDomain}, claimedHosts); dup {
			c.logger.WarnContext(ctx, "ingress: platform primary domain is already routed to a service or static site, skipping the dashboard route",
				slog.String("domain", host),
				slog.String("already_routed_to", owner),
			)
		} else {
			claimedHosts[settings.PrimaryDomain] = dashboardRouteOwner
			routes = append(routes, ingress.ProxyRoute{
				Hosts:       []string{settings.PrimaryDomain},
				BackendDial: c.dashboardDial,
			})
		}
	}

	// Built-in registry (store.RegistrySettings): one more reverse-proxy
	// route, the exact same shape the dashboard route above has (plain
	// TLS termination, no basic_auth handler, since the registry
	// container enforces its own htpasswd auth). Requires both a
	// configured Host and WithRegistryDial having been set; either
	// missing means no registry route this pass, the same "fails closed"
	// shape the dashboard route already establishes.
	if registrySettings.Enabled && registrySettings.Host != "" && c.registryDial != "" {
		if owner, host, dup := firstDuplicateHost(registryRouteOwner, []string{registrySettings.Host}, claimedHosts); dup {
			c.logger.WarnContext(ctx, "ingress: built-in registry host is already routed to a service or static site, skipping the registry route",
				slog.String("domain", host),
				slog.String("already_routed_to", owner),
			)
		} else {
			claimedHosts[registrySettings.Host] = registryRouteOwner
			routes = append(routes, ingress.ProxyRoute{
				Hosts:       []string{registrySettings.Host},
				BackendDial: c.registryDial,
			})
		}
	}

	cfg, err := ingress.BuildRoutesConfig(ingress.RoutesOptions{
		ServerName:            c.serverName,
		ListenAddr:            c.listenAddr,
		Routes:                routes,
		StaticRoutes:          staticRoutes,
		MaintenanceRoutes:     maintenanceRoutes,
		TLS:                   true,
		AdminListen:           c.adminListen,
		StorageDir:            c.storageDir,
		CertStorage:           c.certStorage,
		ACMEEnabled:           settings.ACMEEnabled,
		ACMEEmail:             settings.ACMEEmail,
		ACMEDirectoryURL:      settings.ACMEDirectoryURL,
		CloudflareDNSAPIToken: c.resolveCloudflareDNSAPIToken(ctx),
		TLSCertificates:       tlsCertOverrides,
	})
	if err != nil {
		return notReady("BuildConfigFailed", err), fmt.Errorf("ingress: build config: %w", err)
	}

	if err := c.driver.Apply(ctx, cfg); err != nil {
		return notReady("ApplyFailed", err), fmt.Errorf("ingress: apply config: %w", err)
	}

	total := len(routes) + len(staticRoutes) + len(maintenanceRoutes)
	reason := fmt.Sprintf("Routed%dServices", total)
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:    "Ready",
		Status:  reconcile.ConditionTrue,
		Reason:  reason,
		Message: fmt.Sprintf("%d service(s)/static site(s) with domains are routed (%d with a running backend, %d served directly, %d in maintenance mode)", total, len(routes), len(staticRoutes), len(maintenanceRoutes)),
	}}}, nil
}

// firstDuplicateHost reports the first of domains already present in
// claimed (owned by a resource other than name), if any. Shared by both
// container services and static sites (Reconcile calls it once per
// resource kind against the same claimedHosts map), since the conflict
// it guards against, migrations/0015's own comment explains, is no
// longer confined to one table: a static site and a container service
// must never both route the same host either. See Reconcile's own
// comment for why this check exists as a defense-in-depth guard on top
// of store.SaveDesiredService's and store.SaveStaticSite's own
// domain-uniqueness enforcement.
func firstDuplicateHost(name string, domains []string, claimed map[string]string) (owner, host string, dup bool) {
	for _, h := range domains {
		if o, ok := claimed[h]; ok && o != name {
			return o, h, true
		}
	}
	return "", "", false
}

// resolveCloudflareDNSAPIToken returns the Cloudflare DNS-01 API token
// for this reconcile pass, or "" if WithCloudflareDNSTokens was never
// set, the operator hasn't enabled it (store.CloudflareDNSSettings.
// Enabled), or resolving it fails. Failing to "" rather than returning
// an error keeps a token problem from blocking the whole ingress
// reconcile: it only means wildcard hosts fall back to whatever the
// non-wildcard policy already does with them this pass, exactly as if
// this feature were never configured.
func (c *Controller) resolveCloudflareDNSAPIToken(ctx context.Context) string {
	if c.dnsTokens == nil {
		return ""
	}
	dnsSettings, err := c.store.GetCloudflareDNSSettings(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: get cloudflare dns settings failed, wildcard domains will not get DNS-01 this pass", slog.String("error", err.Error()))
		return ""
	}
	if !dnsSettings.Enabled {
		return ""
	}
	token, err := c.dnsTokens.Resolve(ctx, store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: resolve cloudflare dns token failed, wildcard domains will not get DNS-01 this pass", slog.String("error", err.Error()))
		return ""
	}
	return token
}

// dialForService derives svc's currently active container the same way
// the application controller does (application.ContainerName: a
// deterministic hash of the service name and image), inspects it, and
// returns its host port binding as a dial address. false means svc has
// no valid backend to route right now; the caller skips it for this
// pass rather than failing the whole reconcile.
func (c *Controller) dialForService(ctx context.Context, svc store.DesiredService) (string, bool) {
	target := application.ContainerName(svc.Name, svc.Image, svc.RestartNonce)

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		// A real Docker-level error inspecting one service's container is
		// still not worth failing every other service's routing over: the
		// principle that one broken resource should never block
		// convergence of everything else applies within this single
		// pass too, not just across controllers. Logged at a level above
		// debug since, unlike "not found" or "not running," this is a
		// genuine anomaly worth noticing.
		c.logger.WarnContext(ctx, "ingress: inspecting service container failed, skipping for this pass",
			slog.String("service", svc.Name),
			slog.String("container", target),
			slog.String("error", err.Error()),
		)
		return "", false
	}
	if state == nil {
		c.logger.DebugContext(ctx, "ingress: no container found for service yet, skipping (likely mid-deploy or never deployed)",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return "", false
	}
	if !state.Running {
		c.logger.DebugContext(ctx, "ingress: service container found but not running, skipping",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return "", false
	}
	if len(state.Ports) == 0 {
		c.logger.DebugContext(ctx, "ingress: service container running but has no published ports, skipping",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return "", false
	}

	// See internal/docker.PortBinding's doc comment: routing to the
	// container's published host port, not its container-network IP, is
	// the deliberate choice this whole codebase makes, for the same
	// reason the application controller's readiness probe does (macOS
	// Docker Desktop's VM boundary makes container IPs unreachable from
	// this process, host ports are reachable everywhere this runs).
	return "127.0.0.1:" + strconv.Itoa(state.Ports[0].HostPort), true
}

// domainBasicAuthByDomain returns every store.DomainBasicAuth row keyed
// by domain, for routesForService to look up in O(1) per host.
func (c *Controller) domainBasicAuthByDomain(ctx context.Context) (map[string]store.DomainBasicAuth, error) {
	rows, err := c.store.ListDomainBasicAuth(ctx)
	if err != nil {
		return nil, err
	}
	byDomain := make(map[string]store.DomainBasicAuth, len(rows))
	for _, row := range rows {
		byDomain[row.Domain] = row
	}
	return byDomain, nil
}

// domainMaintenanceSet returns the set of domains currently in
// maintenance mode, for the services loop to check per host in O(1),
// mirroring domainBasicAuthByDomain's identical shape for a different
// per-domain toggle.
func (c *Controller) domainMaintenanceSet(ctx context.Context) (map[string]bool, error) {
	domains, err := c.store.ListDomainMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(domains))
	for _, d := range domains {
		set[d] = true
	}
	return set, nil
}

// splitMaintenanceHosts partitions domains into the ones currently in
// maintenance mode and the ones that still need a real backend routed
// to them.
func splitMaintenanceHosts(domains []string, maintenanceByDomain map[string]bool) (maintenance, active []string) {
	for _, host := range domains {
		if maintenanceByDomain[host] {
			maintenance = append(maintenance, host)
		} else {
			active = append(active, host)
		}
	}
	return maintenance, active
}

// routesForService builds one ProxyRoute per entry in hosts that has
// basic auth and/or WAF/rate-limit configured (each needs its own Handle
// chain, since Caddy has no notion of "this host within a shared route
// is exempt"), plus one shared ProxyRoute carrying every host that has
// neither. A service with no customized domains reproduces this
// controller's behavior before either feature existed exactly: a single
// route with every host. hosts is the caller's already-filtered subset
// of the service's own domains (Reconcile excludes any domain in
// maintenance mode before calling this), not necessarily svc.Domains
// verbatim.
func (c *Controller) routesForService(ctx context.Context, hosts []string, dial string, authByDomain map[string]store.DomainBasicAuth, wafByDomain map[string]store.DomainWAF) []ingress.ProxyRoute {
	var open []string
	var routes []ingress.ProxyRoute
	for _, host := range hosts {
		var account *ingress.BasicAuthAccount
		if auth, protected := authByDomain[host]; protected {
			resolved, ok := c.resolveBasicAuthAccount(ctx, host, auth)
			if !ok {
				// A domain configured for basic auth that this pass cannot
				// resolve a password for (no resolver wired, or the secret
				// itself failed to resolve) is left unrouted this pass
				// rather than served unprotected: failing a security
				// control closed is worse to leave silent than a domain
				// being briefly unreachable, unlike dialForService's own
				// "no backend yet" cases above, which fail open to "just
				// not routed yet."
				continue
			}
			account = resolved
		}

		waf := domainWAFConfig(wafByDomain[host])
		if account == nil && waf == nil {
			open = append(open, host)
			continue
		}
		routes = append(routes, ingress.ProxyRoute{Hosts: []string{host}, BackendDial: dial, BasicAuth: account, WAF: waf})
	}
	if len(open) > 0 {
		routes = append(routes, ingress.ProxyRoute{Hosts: open, BackendDial: dial})
	}
	return routes
}

// domainWAFConfig converts row into an ingress.WAFConfig, or nil when
// row has neither the WAF nor rate limiting turned on: the zero value of
// store.DomainWAF (what wafByDomain[host] returns for any host with no
// row at all) always takes this nil branch, reproducing this
// controller's behavior before this feature existed exactly.
func domainWAFConfig(row store.DomainWAF) *ingress.WAFConfig {
	if !row.WAFEnabled && row.RateLimitRPS <= 0 {
		return nil
	}
	return &ingress.WAFConfig{
		Enabled:        row.WAFEnabled,
		Blocking:       row.WAFMode == store.DomainWAFModeBlock,
		RateLimitRPS:   row.RateLimitRPS,
		RateLimitBurst: row.RateLimitBurst,
	}
}

// domainWAFByDomain returns every store.DomainWAF row keyed by domain,
// mirroring domainBasicAuthByDomain's identical shape for a different
// per-domain toggle.
func (c *Controller) domainWAFByDomain(ctx context.Context) (map[string]store.DomainWAF, error) {
	rows, err := c.store.ListDomainWAF(ctx)
	if err != nil {
		return nil, err
	}
	byDomain := make(map[string]store.DomainWAF, len(rows))
	for _, row := range rows {
		byDomain[row.Domain] = row
	}
	return byDomain, nil
}

// resolveBasicAuthAccount resolves domain's plaintext password through
// c.basicAuthSecrets and hashes it with bcrypt (the same algorithm
// internal/api already uses for user login passwords), fresh every
// call: this controller never persists a hash across reconcile passes,
// the same "never cache, re-derive from current state" principle every
// other Reconcile input already follows.
func (c *Controller) resolveBasicAuthAccount(ctx context.Context, domain string, auth store.DomainBasicAuth) (*ingress.BasicAuthAccount, bool) {
	if c.basicAuthSecrets == nil {
		return nil, false
	}
	password, err := c.basicAuthSecrets.Resolve(ctx, store.DomainBasicAuthSecretsKey(domain), store.DomainBasicAuthPasswordEnvKey)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: resolve domain basic auth password failed, domain will not be routed this pass",
			slog.String("domain", domain), slog.String("error", err.Error()))
		return nil, false
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: hash domain basic auth password failed, domain will not be routed this pass",
			slog.String("domain", domain), slog.String("error", err.Error()))
		return nil, false
	}
	return &ingress.BasicAuthAccount{Username: auth.Username, Password: string(hash)}, true
}

// domainTLSCertOverrides resolves every store.DomainTLSCert row into an
// ingress.TLSCertificateOverride, fresh every call: this controller
// never persists decrypted certificate material across reconcile
// passes, the same "never cache, re-derive from current state" principle
// resolveBasicAuthAccount already follows. A domain whose certificate or
// key this pass cannot resolve (no resolver wired, or the secret itself
// failed to resolve) is simply omitted, not an error: see
// WithDomainTLSCertSecrets's doc comment for why that's a fail-open
// choice here, unlike basic auth's fail-closed one.
func (c *Controller) domainTLSCertOverrides(ctx context.Context) ([]ingress.TLSCertificateOverride, error) {
	rows, err := c.store.ListDomainTLSCerts(ctx)
	if err != nil {
		return nil, err
	}
	if c.tlsCertSecrets == nil || len(rows) == 0 {
		return nil, nil
	}

	overrides := make([]ingress.TLSCertificateOverride, 0, len(rows))
	for _, row := range rows {
		key := store.DomainTLSCertSecretsKey(row.Domain)
		certPEM, err := c.tlsCertSecrets.Resolve(ctx, key, store.DomainTLSCertCertificateEnvKey)
		if err != nil {
			c.logger.WarnContext(ctx, "ingress: resolve domain tls certificate failed, domain falls back to automatic issuance this pass",
				slog.String("domain", row.Domain), slog.String("error", err.Error()))
			continue
		}
		keyPEM, err := c.tlsCertSecrets.Resolve(ctx, key, store.DomainTLSCertPrivateKeyEnvKey)
		if err != nil {
			c.logger.WarnContext(ctx, "ingress: resolve domain tls private key failed, domain falls back to automatic issuance this pass",
				slog.String("domain", row.Domain), slog.String("error", err.Error()))
			continue
		}
		overrides = append(overrides, ingress.TLSCertificateOverride{Host: row.Domain, CertPEM: certPEM, KeyPEM: keyPEM})
	}
	return overrides, nil
}

func notReady(reason string, err error) reconcile.Result {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}
