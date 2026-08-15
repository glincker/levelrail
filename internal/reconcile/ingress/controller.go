// Package ingress implements TASKS.md 1.6's ingress controller: the
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
// here are also closed, both TASKS.md 3.6:
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
}

// Applier is the narrow surface this controller needs from
// internal/ingress.Driver, so tests can fake it without starting a real
// Caddy instance. *ingress.Driver satisfies this.
type Applier interface {
	Apply(ctx context.Context, cfg *ingress.Config) error
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

	// certStore, if set via WithCertStore, is built into a
	// *ingress.SQLiteStorage and registered with ingress.SetActiveCertStorage
	// once, in New, rather than on every Reconcile. certStorage is the
	// resulting ingress.RoutesOptions.CertStorage value; nil means
	// "storageDir/file storage instead" (see WithCertStore).
	certStore   ingress.CertStore
	certStorage any
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
// OS-specific default location. Production wiring (TASKS.md 1.9/1.10, not
// yet built) should point this at a path under the control plane's data
// directory once that constant exists; this package does not invent one,
// keeping with the repo's brand/path indirection rule (no hardcoded
// paths outside the shared data-directory constant).
func WithStorageDir(dir string) Option {
	return func(c *Controller) { c.storageDir = dir }
}

// WithCertStore points Caddy's certificate/ACME-account storage at
// internal/store's SQLite (TASKS.md 3.6) instead of the local
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

	var routes []ingress.ProxyRoute
	claimedHosts := make(map[string]string, len(services)+len(staticSites)) // host -> owning service/static site, this pass only
	for _, svc := range services {
		if len(svc.Domains) == 0 {
			continue
		}
		if owner, host, dup := firstDuplicateHost(svc.Name, svc.Domains, claimedHosts); dup {
			// store.SaveDesiredService (TASKS.md 3.6) now rejects a save
			// that would create this situation for any service written
			// after that change landed, so reaching this branch means
			// either data written before the constraint existed, or a
			// direct write that bypassed SaveDesiredService. Either way,
			// this pass must still never build two routes for the same
			// host: skip the loser and say so loudly, rather than
			// silently letting Caddy's own last-match-wins matcher
			// evaluation decide.
			c.logger.WarnContext(ctx, "ingress: service claims a domain another service already routed this pass, skipping; internal/store's service_domains uniqueness constraint should prevent this for any service saved since TASKS.md 3.6, this is a defense-in-depth guard for pre-existing data",
				slog.String("service", svc.Name),
				slog.String("domain", host),
				slog.String("already_routed_to", owner),
			)
			continue
		}
		route, ok := c.routeFor(ctx, svc)
		if !ok {
			continue
		}
		for _, host := range svc.Domains {
			claimedHosts[host] = svc.Name
		}
		routes = append(routes, route)
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

	cfg, err := ingress.BuildRoutesConfig(ingress.RoutesOptions{
		ServerName:       c.serverName,
		ListenAddr:       c.listenAddr,
		Routes:           routes,
		StaticRoutes:     staticRoutes,
		TLS:              true,
		AdminListen:      c.adminListen,
		StorageDir:       c.storageDir,
		CertStorage:      c.certStorage,
		ACMEEnabled:      settings.ACMEEnabled,
		ACMEEmail:        settings.ACMEEmail,
		ACMEDirectoryURL: settings.ACMEDirectoryURL,
	})
	if err != nil {
		return notReady("BuildConfigFailed", err), fmt.Errorf("ingress: build config: %w", err)
	}

	if err := c.driver.Apply(ctx, cfg); err != nil {
		return notReady("ApplyFailed", err), fmt.Errorf("ingress: apply config: %w", err)
	}

	total := len(routes) + len(staticRoutes)
	reason := fmt.Sprintf("Routed%dServices", total)
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:    "Ready",
		Status:  reconcile.ConditionTrue,
		Reason:  reason,
		Message: fmt.Sprintf("%d service(s)/static site(s) with domains are routed (%d with a running backend, %d served directly)", total, len(routes), len(staticRoutes)),
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

// routeFor derives svc's currently active container the same way the
// application controller does (application.ContainerName: a deterministic
// hash of the service name and image), inspects it, and builds a
// ProxyRoute from its host port binding. false means svc has no valid
// backend to route right now; the caller skips it for this pass rather
// than failing the whole reconcile.
func (c *Controller) routeFor(ctx context.Context, svc store.DesiredService) (ingress.ProxyRoute, bool) {
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
		return ingress.ProxyRoute{}, false
	}
	if state == nil {
		c.logger.DebugContext(ctx, "ingress: no container found for service yet, skipping (likely mid-deploy or never deployed)",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return ingress.ProxyRoute{}, false
	}
	if !state.Running {
		c.logger.DebugContext(ctx, "ingress: service container found but not running, skipping",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return ingress.ProxyRoute{}, false
	}
	if len(state.Ports) == 0 {
		c.logger.DebugContext(ctx, "ingress: service container running but has no published ports, skipping",
			slog.String("service", svc.Name),
			slog.String("container", target),
		)
		return ingress.ProxyRoute{}, false
	}

	// See internal/docker.PortBinding's doc comment: routing to the
	// container's published host port, not its container-network IP, is
	// the deliberate choice this whole codebase makes, for the same
	// reason the application controller's readiness probe does (macOS
	// Docker Desktop's VM boundary makes container IPs unreachable from
	// this process, host ports are reachable everywhere this runs).
	dial := "127.0.0.1:" + strconv.Itoa(state.Ports[0].HostPort)

	return ingress.ProxyRoute{Hosts: svc.Domains, BackendDial: dial}, true
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
