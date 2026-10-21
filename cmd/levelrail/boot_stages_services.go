package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/functions"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// buildRunners builds the backup and scheduled task runners.
func (s *bootState) buildRunners() error {
	logger := s.logger
	agentRegistry := s.agentRegistry
	b := s.b
	client := s.client
	db := s.db
	secretsManager := s.secretsManager
	// Shared by the manual trigger and the scheduler; nil without a master key.
	var backupRunner *backup.Runner
	// backupVerifyRunner, like backupRunner above, is shared between the
	// manual verify trigger (api.WithBackupVerifier) and the scheduler's
	// automatic post-backup check below.
	var backupVerifyRunner *backup.VerifyRunner
	if secretsManager != nil {
		backupRunner = &backup.Runner{
			Store:   db,
			Secrets: secretsManager,
			Dumper:  &backup.ContainerDumper{Runtime: client},
			// Shared by database and service volume backups, so both use one upload, history, and scheduling pipeline.
			VolumeArchiver:    &backup.ContainerVolumeArchiver{Runtime: client},
			SqliteSnapshotter: &backup.ContainerSqliteSnapshotter{Runtime: client},
			Uploader:          routedStorage(agentRegistry),
		}
		backupVerifyRunner = &backup.VerifyRunner{
			Store:      db,
			Secrets:    secretsManager,
			Downloader: routedStorage(agentRegistry),
		}
		volumeBackupOpts, err := newVolumeBackupOptions(db, client, agentRegistry, logger)
		if err != nil {
			return err
		}
		backupRunner.Volume = volumeBackupOpts
		backupVerifyRunner.Identities = backupIdentities(backupRunner)
	}

	// Shared by the manual trigger and the cron loop; needs no master key since exec'd commands carry no credential.
	jobLimits := scheduledtask.LimitsFromEnv(os.LookupEnv)
	scheduledTaskRunner := &scheduledtask.Runner{
		Apps:       db,
		Runs:       db,
		History:    db,
		Limits:     jobLimits,
		NamePrefix: b.ShortName,
		Resolver: func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(client, agentRegistry, nodeID)
		},
		Logger: logger,
	}
	s.backupRunner = backupRunner
	s.backupVerifyRunner = backupVerifyRunner
	s.scheduledTaskRunner = scheduledTaskRunner
	s.functionQueue = &functions.Queue{Store: db, Apps: db, Config: functions.TriggerConfigFromEnv(os.LookupEnv), Logger: logger}
	return nil
}

// startBuildAndIngress builds the pipeline, reconcile engine, ingress driver and plugins.
func (s *bootState) startBuildAndIngress() {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	b := s.b
	client := s.client
	db := s.db
	deployDispatcher := s.deployDispatcher
	deployRecorder := s.deployRecorder
	guard := s.guard
	secretsManager := s.secretsManager
	telemetryDB := s.telemetryDB
	supplyChainSvc := newSupplyChainService(ctx, logger, db, client, agentDataDir, b.ShortName)

	builder, buildLimiter, closeBuilder, err := loadBuilder(ctx, logger, db, telemetryDB, secretsManager, agentRegistry, supplyChainSvc, guard)
	if err != nil {
		// Non-fatal: the control plane still starts, with build-dependent paths unavailable.
		logger.Warn("builder not configured: git webhook deploys and manual build trigger are both unavailable", slog.String("error", err.Error()))
	}
	if closeBuilder != nil {
		s.later(func() {
			if cerr := closeBuilder(); cerr != nil {
				logger.Error("closing build client", slog.String("error", cerr.Error()))
			}
		})
	}

	// Created before the router and webhook handler, which both need it; safe to call before Run starts.
	engine := reconcile.NewEngine(logger)

	webhookHandler, err := loadWebhookHandler(logger, b, db, deployRecorder, deployDispatcher, builder, engine)
	if err != nil {
		// Non-fatal, like the other optional wiring: only the git-push path is unavailable.
		logger.Warn("webhook not configured", slog.String("error", err.Error()))
	}

	ingressDriver := ingressdriver.New(logger)
	if _, err := ingressdriver.ImportFileCerts(ctx, db, ingressdriver.LegacyStorageDir(), logger); err != nil {
		logger.Warn("importing legacy certificate storage failed, domains may re-issue", slog.String("error", err.Error()))
	}
	purgeStaleIssuerCerts(ctx, db, logger)
	s.later(func() {
		if cerr := ingressDriver.Stop(context.Background()); cerr != nil {
			logger.Error("stopping ingress driver", slog.String("error", cerr.Error()))
		}
	})

	pluginMgr := setupPlugins(ctx, logger, b, db, client, agentDataDir)
	s.supplyChainSvc = supplyChainSvc
	s.builder = builder
	s.buildLimiter = buildLimiter
	s.engine = engine
	s.webhookHandler = webhookHandler
	s.ingressDriver = ingressDriver
	s.pluginMgr = pluginMgr
}

// wireRouter builds the API router and attaches late-bound services.
func (s *bootState) wireRouter() {
	ctx, stop, logger := s.ctx, s.stop, s.logger
	agentCA := s.agentCA
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	agentServer := s.agentServer
	alertingDB := s.alertingDB
	b := s.b
	backupRunner := s.backupRunner
	backupVerifyRunner := s.backupVerifyRunner
	buildLimiter := s.buildLimiter
	builder := s.builder
	client := s.client
	db := s.db
	deployDispatcher := s.deployDispatcher
	deployRecorder := s.deployRecorder
	emailSender := s.emailSender
	engine := s.engine
	guard := s.guard
	hostReports := s.hostReports
	ingressDriver := s.ingressDriver
	logBroadcaster := s.logBroadcaster
	masterKeyFilePath := s.masterKeyFilePath
	pluginMgr := s.pluginMgr
	pushVAPIDPublicKey := s.pushVAPIDPublicKey
	scheduledTaskRunner := s.scheduledTaskRunner
	secretsManager := s.secretsManager
	telemetryDB := s.telemetryDB
	webhookHandler := s.webhookHandler
	apiHandler, apiRouter := rootHandler(rootDeps{
		logger: logger, b: b, db: db, telemetryDB: telemetryDB, alertingDB: alertingDB,
		secretsManager: secretsManager, masterKeyFilePath: masterKeyFilePath, webhookHandler: webhookHandler,
		client: client, builder: builder, deployRecorder: deployRecorder, logBroadcaster: logBroadcaster,
		deployDispatcher: deployDispatcher, backupRunner: backupRunner, backupVerifyRunner: backupVerifyRunner,
		agentRegistry: agentRegistry, agentCAFingerprint: agentCA.Fingerprint(), emailSender: emailSender,
		scheduledTaskRunner: scheduledTaskRunner, functionQueue: s.functionQueue, engine: engine, ingressDriver: ingressDriver,
		pushVAPIDPublicKey: pushVAPIDPublicKey,
	})
	apiRouter.WarmTrustRoot()
	if buildLimiter != nil {
		apiRouter.SetBuildLoad(buildLimiter)
	}
	if wh, ok := webhookHandler.(*webhook.Handler); ok {
		wh.UseSourceCache(apiRouter.SourceCache())
	}
	if pluginMgr != nil {
		apiRouter.SetPluginManager(pluginMgr)
		apiRouter.SetPluginMarketplace(setupPluginMarketplace(logger, db, pluginMgr))
	}
	apiRouter.SetRestarter(api.Restarter{
		Supervised: restartSupervised(os.LookupEnv, fileExists),
		Trigger:    func() { s.restartRequested.Store(true); stop() },
	})
	apiRouter.SetControlPlaneProbe(api.ControlPlaneProbe{DB: db, Reconcile: engine.Stats, ResyncInterval: resyncInterval, Telemetry: telemetryDB, MemoryLimit: footprint.CurrentMemoryLimit(), Integrity: s.integrity, Guard: engine.Guard()})
	go db.RunWALMaintenance(ctx)
	configureNodeCerts(apiRouter, agentServer, agentServer)
	apiRouter.SetMigrationAgent(s.migrationAgent)
	setupEdgeFirewall(ctx, logger, b, apiRouter, secretsManager)
	apiRouter.SetNodeHostReports(hostReports)
	gateSecret := setupBuiltinGate(ctx, logger, apiRouter)
	apiRouter.SetDockerGuard(guard.Controller)
	setupLogArchive(ctx, logger, db, telemetryDB, secretsManager, apiRouter)
	setupDataExport(ctx, logger, b, db, telemetryDB, secretsManager, logBroadcaster, apiRouter)
	setupEventWebhooks(ctx, logger, b, db, secretsManager, apiRouter)
	startScheduledMaintenance(ctx, logger, db, alertingDB, deployDispatcher, apiRouter)
	startKeyLifecycle(ctx, logger, db, secretsManager, alertingDB, deployDispatcher, apiRouter)
	startDiskGuard(ctx, logger, agentDataDir, alertingDB, deployDispatcher, apiRouter, db)
	setupCompliance(ctx, logger, db, secretsManager, agentDataDir, apiRouter)
	startHeldDeployReleaser(ctx, logger, db, apiRouter)
	startCanaryReconciler(ctx, logger, db, apiRouter)
	apiRouter.StartSplitTesting(ctx)
	cpDR := setupControlPlaneDR(ctx, logger, db, secretsManager, masterKeyFilePath, agentDataDir, apiRouter)
	startPipelines(ctx, logger, b, db, secretsManager, client, agentRegistry, builder, engine, deployDispatcher, apiRouter)
	startExternalDatabaseMonitor(ctx, logger, db, secretsManager, client, agentRegistry)
	httpServer := &http.Server{
		Addr:              httpAddr(),
		Handler:           apiHandler,
		ReadHeaderTimeout: 10 * time.Second,
		// ReadTimeout bounds request bodies, not responses, so SSE streams and exec terminals are unaffected.
		// WriteTimeout is omitted: it cannot exempt long-lived streams.
		ReadTimeout: 30 * time.Second,
		IdleTimeout: 120 * time.Second,
	}
	s.apiRouter = apiRouter
	s.gateSecret = gateSecret
	s.cpDR = cpDR
	s.httpServer = httpServer
}

// startMesh starts the mesh, previews, orphan reaper and ingress edge.
func (s *bootState) startMesh() error {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	apiRouter := s.apiRouter
	b := s.b
	client := s.client
	db := s.db
	ingressDriver := s.ingressDriver
	instanceID := s.instanceID
	meshEndpoints := s.meshEndpoints
	secretsManager := s.secretsManager
	supplyChainSvc := s.supplyChainSvc
	telemetryDB := s.telemetryDB
	lbRegistry := loadbalancer.NewRegistry()
	lbStats := loadbalancer.CaddyAdminStats{Addr: ingressreconcile.DefaultAdminListen}
	apiRouter.SetLoadBalancers(db, lbRegistry, lbStats)
	if experimental.Enabled(experimental.LoadBalancer) {
		go runLoadBalancerTelemetry(ctx, lbRegistry, lbStats, telemetryDB, metricsCollectionInterval, logger)
	}

	meshCfg, err := setupMesh(ctx, db, b, agentDataDir, agentRegistry, logger)
	if err != nil {
		// Non-fatal: a misconfigured mesh should not stop a control plane that runs fine without it.
		logger.Warn("mesh not configured", slog.String("error", err.Error()))
		apiRouter.SetMeshStartError(err)
	}
	if meshCfg != nil {
		meshEndpoints.Attach(meshCfg.coordinator, meshCfg.localNodeID)
	}
	apiRouter.SetAppTeardownOptions(application.WithNetworkPrefix(b.ShortName), application.WithInstanceID(instanceID))
	apiRouter.SetPrivateNetworks(privateNetworkConfig(db, client, b.ShortName, instanceID))
	if meshCfg != nil {
		s.later(func() { meshCfg.close() })
		apiRouter.SetLocalNodeID(meshCfg.localNodeID)
		agentRegistry.SetLocal(meshCfg.localNodeID, agent.NewLocal(client))
		apiRouter.SetMesh(meshCfg.device, meshCfg.coordinator)
	}
	apiRouter.SetMeshPaths(newMeshPathResolver(meshCfg, db))
	apiRouter.SetMeshPeerStatus(agent.NewGRPCSink(agentRegistry))
	go apiRouter.RunNodeMoves(ctx)
	go apiRouter.RunNodeFailure(ctx)
	// Resolved once: the bridge gateway IP does not change while the process runs.
	meshDNSAddr := containerDNSAddr(ctx, client, meshCfg, logger)
	if meshDNSAddr != "" && meshCfg != nil && meshCfg.resolver != nil {
		apiRouter.SetMeshZone(meshCfg.resolver.Zone())
	}

	previewLocalNodeID := ""
	if meshCfg != nil {
		previewLocalNodeID = meshCfg.localNodeID
	}
	previewManager := newPreviewManager(ctx, logger, db, client, agentDataDir, b.ShortName, previewLocalNodeID)
	apiRouter.SetPreview(previewManager)
	apiRouter.SetSupplyChain(supplyChainSvc)
	apiRouter.SetSecretScan(newSecretScanService(db, logger))

	orphanReaper := newOrphanReaper(db, client, agentRegistry, previewLocalNodeID, instanceID, logger)
	apiRouter.SetOrphanReaper(orphanReaper)
	janitorEngine := apiRouter.NewJanitor(db, client)

	edge, err := loadIngressEdge(logger, b)
	if err != nil {
		return fmt.Errorf("ingress edge settings: %w", err)
	}
	edgeInfo := edge.doctorInfo()
	edgeInfo.Snapshot = setupIngressSnapshot(ctx, logger, ingressDriver, db, secretsManager, edge)
	apiRouter.SetDoctorIngressEdge(edgeInfo)
	apiRouter.SetIngressResilience(api.IngressResilience{
		Status:  ingressDriver.SnapshotStatus,
		Verify:  ingressDriver.VerifySnapshot,
		SaveNow: ingressDriver.SaveSnapshotNow,
		CertKeys: func(ctx context.Context) (int, error) {
			keys, err := db.ListCertStorageKeys(ctx, "certificates", true)
			return len(keys), err
		},
	})
	proxySyncer := newProxySyncer(b, db, logger, proxyContainerExec{rt: client})
	apiRouter.SetProxyIntegration(proxySyncer, db, proxyVerifyTimeout(logger))
	routeCheck := newRouteCheck(logger)
	apiRouter.SetProxyRouteCheck(routeCheck)
	apiRouter.ResumeCutoverRuns(ctx)
	go apiRouter.ResumeTakeover(ctx)
	s.lbRegistry = lbRegistry
	s.meshCfg = meshCfg
	s.meshDNSAddr = meshDNSAddr
	s.previewManager = previewManager
	s.orphanReaper = orphanReaper
	s.janitorEngine = janitorEngine
	s.edge = edge
	s.proxySyncer = proxySyncer
	s.routeCheck = routeCheck
	return nil
}

// startReconciler attaches the desired-state source to the reconcile engine.
func (s *bootState) startReconciler() {
	ctx, logger := s.ctx, s.logger
	agentRegistry := s.agentRegistry
	b := s.b
	client := s.client
	db := s.db
	edge := s.edge
	engine := s.engine
	gateSecret := s.gateSecret
	ingressDriver := s.ingressDriver
	instanceID := s.instanceID
	janitorEngine := s.janitorEngine
	lbRegistry := s.lbRegistry
	meshCfg := s.meshCfg
	meshDNSAddr := s.meshDNSAddr
	orphanReaper := s.orphanReaper
	previewManager := s.previewManager
	proxySyncer := s.proxySyncer
	routeCheck := s.routeCheck
	scheduledTaskRunner := s.scheduledTaskRunner
	secretsManager := s.secretsManager
	telemetryDB := s.telemetryDB
	engine.SetStore(db)
	gate := newDeployGate(logger, db, client, b.ShortName, meshLocalNodeID(meshCfg))
	sourceDeps := dynamicSourceDeps{
		deployGate:                   gate,
		orphanReaper:                 orphanReaper,
		janitor:                      janitorEngine,
		ingressEdge:                  edge,
		db:                           db,
		runtime:                      client,
		driver:                       ingressDriver,
		logger:                       logger,
		telemetryDB:                  telemetryDB,
		secretsManager:               secretsManager,
		agentRegistry:                agentRegistry,
		heartbeatTimeout:             nodeHeartbeatTimeout(),
		meshCfg:                      meshCfg,
		meshDNSAddr:                  meshDNSAddr,
		databaseSlowQueryThresholdMs: databaseSlowQueryThresholdMs(logger),
		dashboardDial:                dashboardDialAddr(httpAddr()),
		gateSecret:                   gateSecret,
		routeCheck:                   routeCheck,
		networkPrefix:                b.ShortName,
		firewallRulePrefix:           b.RuleCommentPrefix(),
		instanceID:                   instanceID,
		livenessTracker:              application.NewLivenessTracker(),
		restartBackoff:               newRestartBackoff(logger),
		publicHost:                   publicHost(),
		ingressHTTPSAddr:             ingressHTTPSAddr(),
		ingressHTTPAddr:              ingressHTTPAddr(),
		httpRedirect:                 httpRedirectEnabled(ingressHTTPAddr(), edge.inherited, logger),
		models:                       newModelDeps(),
		lbRegistry:                   lbRegistry,
		proxySyncer:                  proxySyncer,
		previewNotifier:              previewManager,
	}
	engine.SetSource(dynamicSource(sourceDeps))
	scheduledTaskRunner.Specs = newJobSpecBuilder(sourceDeps)
	scheduledTaskRunner.Resolver = func(nodeID string) (docker.Runtime, error) {
		return resolveNodeTransport(client, agentRegistry, runtimeNodeID(sourceDeps, nodeID))
	}
	if experimental.Enabled(experimental.AIModels) {
		startLocalGPUCollector(ctx, db, client, logger)
		go models.NewEngineMetricsCollector(db, telemetryDB, nil, logger).Run(ctx, models.EngineMetricsInterval())
	}
	s.gate = gate
}
