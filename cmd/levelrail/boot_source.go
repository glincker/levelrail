package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/canary"
	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/deployurls"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/privnet"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/cloudflaretunnel"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	exposurereconcile "github.com/GLINCKER/levelrail/internal/reconcile/exposure"
	firewallreconcile "github.com/GLINCKER/levelrail/internal/reconcile/firewall"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	meshreconcile "github.com/GLINCKER/levelrail/internal/reconcile/mesh"
	"github.com/GLINCKER/levelrail/internal/reconcile/nodehealth"
	proxyroutesreconcile "github.com/GLINCKER/levelrail/internal/reconcile/proxyroutes"
	registryreconcile "github.com/GLINCKER/levelrail/internal/reconcile/registry"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/sharedenv"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/vault"
	"github.com/GLINCKER/levelrail/kit/firewall"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// Re-lists desired state on every call, so it is level-triggered by construction.
// The ingress controller stays on the local runtime: Caddy is embedded here and cannot reach a remote container.
type dynamicSourceDeps struct {
	deployGate         deployGate
	previewNotifier    previewNotifier
	db                 *store.DB
	runtime            docker.Runtime
	driver             *ingressdriver.Driver
	logger             *slog.Logger
	telemetryDB        *telemetry.DB
	secretsManager     *secrets.Manager
	agentRegistry      *agent.Registry
	heartbeatTimeout   time.Duration
	meshCfg            *meshSetup
	meshDNSAddr        string
	dashboardDial      string
	gateSecret         string
	routeCheck         ingressdriver.RouteCheck
	networkPrefix      string
	firewallRulePrefix string
	// databaseSlowQueryThresholdMs is 0 (databaseSlowQueryThresholdMs's own
	// "use the default" signal) unless APP_DATABASE_SLOW_QUERY_THRESHOLD_MS
	// is set; see database.WithSlowQueryThreshold.
	databaseSlowQueryThresholdMs int
	// Lets two instances on one Docker daemon tell their own resources from each other's stale leftovers.
	instanceID string
	// livenessTracker outlives the per-pass controllers below, which is
	// the whole point: see application.WithLivenessTracker.
	livenessTracker *application.LivenessTracker
	imageIDs        *application.ImageIDCache
	// restartBackoff is shared for the same reason as livenessTracker.
	restartBackoff *application.RestartBackoff
	// retainedDeploys outlives the per-pass controllers so its failure counts survive them.
	retainedDeploys *deployurls.Guard
	// publicHost is APP_PUBLIC_HOST, threaded to the ingress controller
	// for the zero-config fallback domain feature; see
	// ingressreconcile.WithPublicHost's own doc comment.
	publicHost string
	// Lets a second instance on the same host run its ingress on other ports.
	ingressHTTPSAddr string
	ingressHTTPAddr  string
	// httpRedirect is httpRedirectEnabled: Caddy serves port 80 and redirects to https.
	httpRedirect bool
	models       *modelDeps
	// orphanReaper removes leftover containers and volumes on a schedule.
	orphanReaper *orphans.Reaper
	// janitor runs scheduled cleanup passes on the local node.
	janitor     *janitor.Engine
	ingressEdge ingressEdge
	// lbRegistry is shared with the API so it can report live upstream status.
	lbRegistry *loadbalancer.Registry
	// proxySyncer is shared with the API so passes on one directory serialise.
	proxySyncer *proxyroutes.Syncer
}

func dynamicSource(deps dynamicSourceDeps) reconcile.Source {
	return func(ctx context.Context) ([]reconcile.Controller, error) {
		services, err := deps.db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}
		databases, err := deps.db.ListDesiredDatabases(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired databases: %w", err)
		}
		nodes, err := deps.db.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		modelRows, err := deps.db.ListModels(ctx)
		if err != nil {
			return nil, fmt.Errorf("list models: %w", err)
		}

		controllers := make([]reconcile.Controller, 0, len(services)+len(databases)+len(nodes)+2)
		controllers = append(controllers, appControllersFor(ctx, deps, services)...)
		controllers = append(controllers, databaseControllersFor(ctx, deps, databases)...)
		controllers = append(controllers, modelControllersFor(deps, modelRows)...)
		for _, n := range nodes {
			controllers = append(controllers, nodehealth.New(n.ID, deps.db, deps.heartbeatTimeout, nodehealth.WithPendingExpiry(nodePendingExpireAfter())))
		}

		ingressOpts := []ingressreconcile.Option{
			ingressreconcile.WithLogger(deps.logger),
			ingressreconcile.WithPublicHost(deps.publicHost),
			ingressreconcile.WithCertStore(deps.db),
			ingressreconcile.WithServedHostsRecorder(deps.db),
			ingressreconcile.WithDomainConditionsRecorder(deps.db),
			ingressreconcile.WithAuditRecorder(deps.db),
			ingressreconcile.WithListenAddr(deps.ingressHTTPSAddr),
			ingressreconcile.WithHTTPListenAddr(deps.ingressHTTPAddr),
			ingressreconcile.WithHTTPRedirect(deps.httpRedirect),
			ingressreconcile.WithRequestStats(),
			ingressreconcile.WithFirewall(),
			ingressreconcile.WithRouteCheck(deps.routeCheck),
			// Lets Reconcile flag a service placed on an unreachable node.
			ingressreconcile.WithLocalNodeID(localNodeIDOf(deps)),
			// Routes remote apps to their node's mesh address.
			ingressreconcile.WithMeshPaths(newMeshPathResolver(deps.meshCfg, deps.db)),
		}
		ingressOpts = append(ingressOpts, deps.ingressEdge.options(ctx, deps.db, deps.logger)...)
		if experimental.Enabled(experimental.AIModels) {
			ingressOpts = append(ingressOpts, ingressreconcile.WithModelHosts(models.HostLister{Store: deps.db, Hosts: deps.models.hosts}))
		}
		if deps.dashboardDial != "" {
			ingressOpts = append(ingressOpts, ingressreconcile.WithDashboardDial(deps.dashboardDial), ingressreconcile.WithGateSecret(deps.gateSecret))
		}
		// Cloudflare DNS-01 for wildcard domains: same nil-secretsManager
		// hazard as the Tunnel token below, and the same "skipped
		// entirely without a master key" fallback.
		if deps.secretsManager != nil {
			ingressOpts = append(ingressOpts, ingressreconcile.WithCloudflareDNSTokens(deps.secretsManager))
			// Prefers Cloudflare if both DNS-01 providers are enabled.
			ingressOpts = append(ingressOpts, ingressreconcile.WithRoute53DNSCredentials(deps.secretsManager))
			ingressOpts = append(ingressOpts, ingressreconcile.WithGoogleCloudDNSKey(deps.secretsManager))
			// Per-domain HTTP Basic Auth passwords: same nil-secretsManager
			// hazard as Cloudflare DNS-01 above, same "no domain gets a
			// basic_auth handler" fallback when absent.
			ingressOpts = append(ingressOpts, ingressreconcile.WithDomainBasicAuthSecrets(deps.secretsManager))
			// BYO TLS certificate uploads: same nil-secretsManager hazard
			// as basic auth above, same "domain falls back to automatic
			// issuance" fallback when absent.
			ingressOpts = append(ingressOpts, ingressreconcile.WithDomainTLSCertSecrets(deps.secretsManager))
		}
		// Unconditional: the dial target is a fixed loopback address, not a secret.
		ingressOpts = append(ingressOpts, ingressreconcile.WithRegistryDial(registryDialAddr()))
		ingressOpts = append(ingressOpts,
			ingressreconcile.WithImplicitLoadBalancing(implicitLoadBalancing(deps.logger)),
			ingressreconcile.WithNodeUpstreams(lbNodeUpstreams{db: deps.db, local: deps.runtime, registry: deps.agentRegistry, localNodeID: localNodeIDOf(deps)}),
		)
		if experimental.Enabled(experimental.LoadBalancer) {
			ingressOpts = append(ingressOpts, ingressreconcile.WithLoadBalancers(deps.db, deps.lbRegistry))
		}
		ingressOpts = append(ingressOpts, ingressreconcile.WithAppWake(deps.db, dashboardDialAddr(httpAddr()), appWakeToken()))
		ingressOpts = append(ingressOpts, ingressreconcile.WithCanaries(deps.db), ingressreconcile.WithCanaryDials(canary.DefaultDials()))
		controllers = append(controllers, ingressreconcile.New(deps.db, deps.runtime, deps.driver, ingressOpts...))
		if deps.proxySyncer != nil {
			controllers = append(controllers, proxyroutesreconcile.New(deps.proxySyncer))
		}

		// Local runtime unconditionally, same reasoning as the ingress
		// controller above: per-app networks are single-node scope until
		// the WireGuard mesh exists.
		controllers = append(controllers, application.NewNetworkCleanupController(deps.db, deps.runtime, deps.networkPrefix, deps.instanceID))
		for _, n := range nodes {
			if runtimeNodeID(deps, n.ID) == "" {
				continue
			}
			if nodeRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, n.ID); err == nil {
				controllers = append(controllers, application.NewNetworkCleanupController(deps.db, nodeRuntime, deps.networkPrefix, deps.instanceID).ForNode(n.ID))
			}
		}
		controllers = append(controllers, privnet.NewController(deps.db, deps.runtime, deps.networkPrefix, deps.instanceID))
		for _, n := range nodes {
			if runtimeNodeID(deps, n.ID) == "" {
				continue
			}
			if nodeRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, n.ID); err == nil {
				controllers = append(controllers, privnet.NewController(deps.db, nodeRuntime, deps.networkPrefix, deps.instanceID).ForNode(n.ID))
			}
		}
		if deps.orphanReaper != nil {
			controllers = append(controllers, orphans.NewController(deps.orphanReaper))
		}
		if deps.janitor != nil {
			controllers = append(controllers, janitor.NewController(deps.janitor))
		}

		// Platform-wide singleton on the local runtime; a nil secretsManager is handled inside cloudflaretunnel.New.
		var tunnelTokens cloudflaretunnel.TokenResolver
		if deps.secretsManager != nil {
			tunnelTokens = deps.secretsManager
		}
		if experimental.Enabled(experimental.CloudflareTunnel) {
			controllers = append(controllers, cloudflaretunnel.New(deps.db, tunnelTokens, deps.runtime, cloudflaretunnel.WithContainerPrefix(deps.networkPrefix)))
		}

		// The registry credential is generated when an operator enables the registry, so this only resolves an existing one.
		var registryCreds registryreconcile.CredentialResolver
		if deps.secretsManager != nil {
			registryCreds = deps.secretsManager
		}
		controllers = append(controllers, registryreconcile.New(deps.db, registryCreds, deps.runtime, registryreconcile.WithContainerPrefix(deps.networkPrefix)))

		// requiredPorts matches the API's list, so the write-time refusal and this skip never disagree.
		controllers = append(controllers, firewallreconcile.New(deps.db, firewall.New(deps.firewallRulePrefix), firewallreconcile.WithRequiredPorts(platformRequiredPorts()), firewallreconcile.WithLogger(deps.logger)))

		// Exposure restrictions re-assert DOCKER-USER rules after a reboot or Docker restart.
		controllers = append(controllers, exposurereconcile.New(deps.db, newExposureManager(deps.firewallRulePrefix), deps.logger, exposurereconcile.WithPodmanCheck(runtimeIsPodman(deps.runtime))))

		if deps.meshCfg != nil {
			controllers = append(controllers, meshreconcile.New(deps.meshCfg.localNodeID, deps.db, deps.meshCfg.coordinator, deps.meshCfg.resolver, meshreconcile.WithLogger(deps.logger), meshreconcile.WithHiddenApps(privateNetworkHiddenApps(deps.db))))
		}
		return controllers, nil
	}
}

// appOptionsFor builds the options every application.Controller shares:
// the reconcile path and scheduled job containers both resolve a service's
// env, volumes and network through them.
func appOptionsFor(deps dynamicSourceDeps) []application.Option {
	// Storage targets and database attachments need no master key; only their credentials do.
	sharedEnvResolver := sharedenv.NewResolver(deps.db, deps.secretsManager)
	appOpts := []application.Option{
		application.WithStopGrace(application.StopGraceFromEnv(os.Getenv)),
		application.WithDeployRecorder(deps.telemetryDB),
		application.WithHookRunRecorder(deps.db),
		application.WithProbeAttemptRecorder(deps.db),
		application.WithWakeProbe(deps.db, appsleep.ConfigFromEnv(os.Getenv).WakeProbeInterval),
		application.WithStorageTargets(deps.db),
		application.WithBucketBindings(deps.db),
		application.WithDatabaseAttachments(deps.db),
		application.WithDatabaseScope(deps.db),
		application.WithVaultSettings(deps.db),
		application.WithVaultResolver(vault.NewResolver()),
		application.WithRegistryCredentials(deps.db),
		application.WithProjectEnv(sharedEnvResolver),
		application.WithOrganizationEnv(sharedEnvResolver),
		application.WithEnvironmentEnv(sharedEnvResolver),
		application.WithAppIntegrations(deps.db),
		application.WithConditionReader(deps.db),
		application.WithNetworkPrefix(deps.networkPrefix),
		application.WithInstanceID(deps.instanceID),
		application.WithLivenessTracker(deps.livenessTracker),
		application.WithRestartBackoff(deps.restartBackoff),
		application.WithRolloutRecorder(rolloutRecorderFor(deps.db, deps.previewNotifier)),
		application.WithAppliedConfigRecorder(deps.db),
		application.WithPreviousReleaseHold(previousReleaseHold(deps.logger)),
		application.WithPinnedPortRetry(pinnedPortRetry(deps.logger)),
		application.WithStartupWatch(startupWatch(deps.logger)),
		application.WithExitRecorder(deps.db),
		application.WithProbeLimits(probe.LimitsFromEnv(os.LookupEnv)),
		application.WithNodeGPU(modelNodes{db: deps.db, localNodeID: localNodeIDOf(deps)}),
		application.WithMeshPaths(newMeshPathResolver(deps.meshCfg, deps.db)),
	}
	appOpts = append(appOpts, deps.deployGate.appOptions(deps.db)...)
	if reconcile.WorkersFromEnv(os.Getenv) > 1 {
		appOpts = append(appOpts, application.WithReadinessRequeue(pendingReadiness(), application.ReadinessRequeueFromEnv(os.Getenv)))
	}
	if deps.retainedDeploys != nil {
		appOpts = append(appOpts, application.WithRetainedDeploys(deps.retainedDeploys))
	}
	if deps.secretsManager != nil {
		// A retained deploy URL runs its app's config, so it reads the app's secrets.
		appOpts = append(appOpts, application.WithSecretResolver(deployurls.ParentSecrets{Inner: deps.secretsManager, Apps: deps.db}))
	}
	if deps.meshDNSAddr != "" {
		appOpts = append(appOpts, application.WithMeshDNSAddr(deps.meshDNSAddr))
	}
	if deps.meshDNSAddr != "" && deps.meshCfg != nil && deps.meshCfg.resolver != nil {
		appOpts = append(appOpts, application.WithMeshZone(deps.meshCfg.resolver.Zone()))
	}
	if deps.meshCfg != nil && deps.meshCfg.resolver != nil {
		appOpts = append(appOpts, application.WithMeshDatabases(meshDatabaseConfig(deps)))
	}
	return appOpts
}

// nodeOfflineControllerFor keeps an unreachable node's app from showing a stale Ready.
func nodeOfflineControllerFor(ctx context.Context, deps dynamicSourceDeps, svc store.DesiredService) reconcile.Controller {
	name, since := svc.NodeID, time.Time{}
	if n, err := deps.db.GetNode(ctx, svc.NodeID); err == nil && n != nil {
		name = n.Name
		if n.LastSeenAt != nil {
			since = *n.LastSeenAt
		}
	}
	return application.NewNodeOfflineController(svc.Name, name, since)
}

// nodeRecentlyRecovered is true when the node went back online within
// APP_NODE_RECOVERY_WINDOW (default 2m).
func nodeRecentlyRecovered(ctx context.Context, deps dynamicSourceDeps, nodeID string) bool {
	if nodeID == "" {
		return false
	}
	window := 2 * time.Minute
	if d, err := time.ParseDuration(os.Getenv("APP_NODE_RECOVERY_WINDOW")); err == nil && d > 0 {
		window = d
	}
	events, err := deps.db.ListNodeStatusEvents(ctx, nodeID, 1)
	return err == nil && len(events) == 1 && events[0].ToStatus == store.NodeStatusOnline && time.Since(events[0].CreatedAt) < window
}

// appControllersFor builds one application.Controller per service,
// skipping (with a warning) any whose node isn't currently reachable.
func appControllersFor(ctx context.Context, deps dynamicSourceDeps, services []store.DesiredService) []reconcile.Controller {
	appOpts := appOptionsFor(deps)
	snap := application.NewPassSnapshot(application.SnapshotMaxAgeFromEnv(os.Getenv))
	controllers := make([]reconcile.Controller, 0, len(services))
	targets := map[string]*prefetchNode{}
	for _, svc := range services {
		nodeID := runtimeNodeID(deps, svc.NodeID)
		svcRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, nodeID)
		if err != nil {
			deps.logger.Warn("skipping service for this reconcile pass: node transport unavailable",
				slog.String("service", svc.Name), slog.String("node_id", svc.NodeID), slog.String("error", err.Error()))
			controllers = append(controllers, nodeOfflineControllerFor(ctx, deps, svc))
			continue
		}
		svcOpts := append(appOpts[:len(appOpts):len(appOpts)], application.WithPassSnapshot(snap, nodeID), application.WithImageIDCache(deps.imageIDs, nodeID), application.WithNodeRecovering(nodeRecentlyRecovered(ctx, deps, svc.NodeID)))
		appCtl := reconcile.Controller(application.New(svc.Name, deps.db, svcRuntime, svcOpts...))
		if svc.NodeID != "" {
			// Only a placed app can have attempts queued for an offline node.
			appCtl = application.WithPendingAttemptResolution(appCtl, svc.Name, deps.db)
		}
		controllers = append(controllers, appCtl)
		if !svc.Suspended {
			n := targets[nodeID]
			if n == nil {
				n = &prefetchNode{rt: svcRuntime}
				targets[nodeID] = n
			}
			n.names = append(n.names, replicaTargets(svc)...)
		}
	}
	for nodeID, n := range targets {
		snap.Prefetch(ctx, nodeID, n.rt, n.names)
	}
	controllers = append(controllers, application.NewDeleteFinalizer(deps.db, func(nodeID string) (docker.Runtime, error) {
		return resolveNodeTransport(deps.runtime, deps.agentRegistry, runtimeNodeID(deps, nodeID))
	}, deps.logger, application.WithNetworkPrefix(deps.networkPrefix), application.WithInstanceID(deps.instanceID)))
	return controllers
}

// A database whose credentials fail to resolve still gets a controller, and is retried on the next pass.
func databaseControllersFor(ctx context.Context, deps dynamicSourceDeps, databases []store.DesiredDatabase) []reconcile.Controller {
	controllers := make([]reconcile.Controller, 0, len(databases))
	for _, desired := range databases {
		dbRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, runtimeNodeID(deps, desired.NodeID))
		if err != nil {
			deps.logger.Warn("skipping database for this reconcile pass: node transport unavailable",
				slog.String("database", desired.Name), slog.String("node_id", desired.NodeID), slog.String("error", err.Error()))
			continue
		}
		dbOpts := databaseCredentialOpts(ctx, deps, desired)
		if deps.meshDNSAddr != "" {
			dbOpts = append(dbOpts, database.WithMeshDNSAddr(deps.meshDNSAddr))
		}
		if deps.meshCfg != nil {
			dbOpts = append(dbOpts, database.WithMeshEndpoint(newMeshPathResolver(deps.meshCfg, deps.db), deps.db))
		}
		dbOpts = append(dbOpts, database.WithProvisionRecorder(provisionRecorder(deps.db)))
		dbOpts = append(dbOpts, database.WithAgentVersion(agentVersionLookup(deps)))
		controllers = append(controllers, database.New(desired.Name, deps.db, dbRuntime, dbOpts...))
	}
	return controllers
}

var (
	pendingReadinessOnce sync.Once
	pendingReadinessInst *application.PendingReadiness
)

// pendingReadiness is shared so a candidate's readiness wait survives the per-pass controller rebuild.
func pendingReadiness() *application.PendingReadiness {
	pendingReadinessOnce.Do(func() { pendingReadinessInst = application.NewPendingReadiness() })
	return pendingReadinessInst
}

var (
	provisionRecorderOnce sync.Once
	provisionRecorderInst *dbprovision.Recorder
)

// provisionRecorder is shared so its duplicate-event suppression survives reconcile passes.
func provisionRecorder(s dbprovision.Store) *dbprovision.Recorder {
	provisionRecorderOnce.Do(func() { provisionRecorderInst = dbprovision.NewRecorder(s) })
	return provisionRecorderInst
}

// databaseCredentialOpts resolves desired's engine-specific credential
// option, logging and skipping (not failing) on a transient error.
func databaseCredentialOpts(ctx context.Context, deps dynamicSourceDeps, desired store.DesiredDatabase) []database.Option {
	var opts []database.Option
	switch desired.Engine {
	case store.EnginePostgres:
		creds, err := postgresCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping postgres credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithPostgresCredentials(creds))
		}
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name, desired.Engine)...)
		opts = append(opts, database.WithSlowQueryThreshold(deps.databaseSlowQueryThresholdMs))
	case store.EngineRedis:
		// Redis needs no credentials to reconcile at all (this package's
		// own doc comment), so TLS is the only per-engine option it ever
		// gets here.
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name, desired.Engine)...)
	case store.EngineMySQL:
		creds, err := mysqlCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mysql credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMySQLCredentials(creds))
		}
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name, desired.Engine)...)
		opts = append(opts, database.WithSlowQueryThreshold(deps.databaseSlowQueryThresholdMs))
	case store.EngineMongoDB:
		creds, err := mongoCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mongodb credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMongoDBCredentials(creds))
		}
	case store.EngineMariaDB:
		creds, err := mariadbCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mariadb credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMariaDBCredentials(creds))
		}
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name, desired.Engine)...)
	case store.EngineClickHouse:
		creds, err := clickhouseCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping clickhouse credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithClickHouseCredentials(creds))
		}
	case store.EngineMeilisearch, store.EngineQdrant, store.EngineTypesense:
		creds, err := apiKeyCredentialsFor(ctx, deps.secretsManager, desired.Name, desired.Engine)
		if err != nil {
			deps.logger.Warn("skipping api key for this reconcile pass",
				slog.String("database", desired.Name), slog.String("engine", desired.Engine), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithAPIKeyCredentials(creds))
		}
	}
	return opts
}

// Logs and skips on a transient error, like the other credential options.
func databaseTLSOpt(ctx context.Context, deps dynamicSourceDeps, dbName, engine string) []database.Option {
	if engine != store.EnginePostgres {
		mode, err := deps.db.GetDatabaseTLSMode(ctx, dbName)
		if err != nil {
			deps.logger.Warn("skipping tls mode for this reconcile pass", slog.String("database", dbName), slog.String("error", err.Error()))
			return nil
		}
		if !store.DatabaseTLSEffective(engine, mode) {
			return []database.Option{database.WithTLSDisabled()}
		}
	}
	material, err := tlsMaterialFor(ctx, deps.secretsManager, dbName)
	if err != nil {
		deps.logger.Warn("skipping tls material for this reconcile pass",
			slog.String("database", dbName), slog.String("error", err.Error()))
		return nil
	}
	if material == nil {
		return nil
	}
	return []database.Option{database.WithTLS(material)}
}

// An empty nodeID means local. A disconnected node returns agent.ErrNodeNotRegistered, which callers skip for this pass.
func resolveNodeTransport(local docker.Runtime, registry *agent.Registry, nodeID string) (docker.Runtime, error) {
	if nodeID == "" {
		return local, nil
	}
	return registry.Resolve(nodeID)
}

// resolveNodeTransport treats only the empty string as local.
func runtimeNodeID(deps dynamicSourceDeps, nodeID string) string {
	if nodeID == "" || nodeID == localNodeIDOf(deps) {
		return ""
	}
	return nodeID
}

// nodePendingExpireAfter reads APP_NODE_PENDING_EXPIRE_AFTER: how long an enrolled node may stay unconnected before it is removed. 0 disables.
func nodePendingExpireAfter() time.Duration {
	raw := os.Getenv("APP_NODE_PENDING_EXPIRE_AFTER")
	if raw == "" {
		return time.Hour
	}
	if d, err := time.ParseDuration(raw); err == nil && d >= 0 {
		return d
	}
	return time.Hour
}

type prefetchNode struct {
	rt    docker.Runtime
	names []string
}

// replicaTargets are the container names the application controller will look up for svc this pass.
func replicaTargets(svc store.DesiredService) []string {
	replicas := svc.Replicas
	if replicas <= 0 {
		replicas = store.DefaultReplicas
	}
	names := make([]string, replicas)
	for i := range names {
		names[i] = application.ReplicaContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce, i)
	}
	return names
}

// runtimeIsPodman reports whether the live engine identifies as Podman.
func runtimeIsPodman(rt docker.Runtime) func(context.Context) bool {
	return func(ctx context.Context) bool {
		c, ok := rt.(interface {
			Runtime() docker.RuntimeInfo
			EngineIdentity(context.Context) (docker.EngineIdentity, error)
		})
		if !ok {
			return false
		}
		kind := c.Runtime().Kind
		if id, err := c.EngineIdentity(ctx); err == nil {
			kind = docker.RefineKind(kind, id)
		}
		return kind == docker.RuntimeKindPodman
	}
}

// agentVersionLookup reports the agent build of a database's node, so the
// controller can hold back destructive drift decisions on an outdated agent.
func agentVersionLookup(deps dynamicSourceDeps) database.AgentVersionLookup {
	return func(ctx context.Context, nodeID string) (string, bool, error) {
		id := runtimeNodeID(deps, nodeID)
		if id == "" {
			return "", false, nil
		}
		n, err := deps.db.GetNode(ctx, id)
		if err != nil {
			return "", true, err
		}
		return n.AgentVersion, true, nil
	}
}
