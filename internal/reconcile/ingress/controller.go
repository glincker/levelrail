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
// backend, and applies that whole document. This fits CLAUDE.md 4.2's
// level-triggered philosophy directly (re-derive from current state
// every time, assume nothing about a previous call), it just means the
// unit of reconciliation for ingress is "all routable services," not one
// resource.
//
// Two follow-ups this pass deliberately does not build, per ADR 005 and
// the Phase 1 task breakdown:
//
//   - Certificate storage stays on Caddy's default file-system storage
//     module (internal/ingress.FileStorage). The real requirement, a
//     certmagic.Storage backed by internal/store's SQLite so multi-node
//     deployments share cert state, is Phase 3 (CLAUDE.md 4.5, TASKS.md
//     1.6), not this pass.
//   - Only Caddy's internal (self-signed, offline) TLS issuer is used.
//     Real ACME against a public domain is explicitly unverified per ADR
//     005's Verified section and needs a manual spot-check against a real
//     domain later, not blind wiring here.
//
// A real, currently open gap this controller does not close: domain
// uniqueness within a single app.yaml is enforced by internal/spec's
// Validate() at deploy time, but nothing enforces uniqueness across
// separate DesiredService rows written by two different deploys over
// time. Two different services could each claim the same domain in the
// store, and this controller would build a Config with two routes
// matching the same host, whichever key sorts last in
// ingress.RoutesOptions.Routes silently wins inside Caddy's own matcher
// evaluation. Closing this needs either a store-level uniqueness
// constraint on domains across all services or a check in this
// controller before calling BuildRoutesConfig; neither exists yet.
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
type ServiceStore interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
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
// per CLAUDE.md 3's brand/path indirection rule.
func WithStorageDir(dir string) Option {
	return func(c *Controller) { c.storageDir = dir }
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
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "ingress" }

// Reconcile implements reconcile.Controller. It lists every desired
// service, builds one route per service that both declares domains and
// currently has a running, port-published container, and applies the
// resulting whole Config to Caddy in a single call (see the package doc
// comment for why this can't be done incrementally).
//
// A service with no valid backend right now (mid-deploy, crashed, never
// deployed) is not a reconcile failure: it's simply left out of this
// pass's config, logged at debug level, and picked up automatically once
// a later reconcile finds it running (this controller and the
// application controller both re-run on every Docker event and resync
// tick, per the shared Engine). Only a genuine failure to list desired
// state, build the config, or apply it to Caddy is reported as an error.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	services, err := c.store.ListDesiredServices(ctx)
	if err != nil {
		return notReady("StoreError", err), fmt.Errorf("ingress: list desired services: %w", err)
	}

	var routes []ingress.ProxyRoute
	for _, svc := range services {
		if len(svc.Domains) == 0 {
			continue
		}
		route, ok := c.routeFor(ctx, svc)
		if !ok {
			continue
		}
		routes = append(routes, route)
	}

	cfg, err := ingress.BuildRoutesConfig(ingress.RoutesOptions{
		ServerName:  c.serverName,
		ListenAddr:  c.listenAddr,
		Routes:      routes,
		TLS:         true,
		AdminListen: c.adminListen,
		StorageDir:  c.storageDir,
	})
	if err != nil {
		return notReady("BuildConfigFailed", err), fmt.Errorf("ingress: build config: %w", err)
	}

	if err := c.driver.Apply(ctx, cfg); err != nil {
		return notReady("ApplyFailed", err), fmt.Errorf("ingress: apply config: %w", err)
	}

	reason := fmt.Sprintf("Routed%dServices", len(routes))
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type:    "Ready",
		Status:  reconcile.ConditionTrue,
		Reason:  reason,
		Message: fmt.Sprintf("%d service(s) with domains have a running backend and are routed", len(routes)),
	}}}, nil
}

// routeFor derives svc's currently active container the same way the
// application controller does (application.ContainerName: a deterministic
// hash of the service name and image), inspects it, and builds a
// ProxyRoute from its host port binding. false means svc has no valid
// backend to route right now; the caller skips it for this pass rather
// than failing the whole reconcile.
func (c *Controller) routeFor(ctx context.Context, svc store.DesiredService) (ingress.ProxyRoute, bool) {
	target := application.ContainerName(svc.Name, svc.Image)

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		// A real Docker-level error inspecting one service's container is
		// still not worth failing every other service's routing over:
		// CLAUDE.md 4.2's "one broken resource should never block
		// convergence of everything else" applies within this single
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
