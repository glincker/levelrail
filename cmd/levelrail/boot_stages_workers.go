package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/deployurls"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/imageupdate"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/scheduledeploy"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/tlsowner"
	"github.com/GLINCKER/levelrail/internal/updatecheck"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

// startTelemetry starts the metric and log collectors.
func (s *bootState) startTelemetry() {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	b := s.b
	client := s.client
	db := s.db
	logBroadcaster := s.logBroadcaster
	meshCfg := s.meshCfg
	notifyClient := s.notifyClient
	telemetryDB := s.telemetryDB
	collector := telemetry.NewCollector(&multiNodeStatsSource{local: client, registry: agentRegistry}, telemetryDB, metricsCollectionInterval, logger)
	go func() {
		targets := telemetryTargets(db, client, agentRegistry, meshCfg, logger)
		if err := collector.Run(ctx, withModelTelemetryTargets(targets, db, client, b.ShortName)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry collector stopped", slog.String("error", err.Error()))
		}
	}()
	startTelemetryMaintenance(ctx, telemetryDB, metricsCollectionInterval, logger)

	// Needs the local node row from setupMesh; with mesh off there is no node to attribute samples to.
	if meshCfg != nil {
		// "node:" + id matches internal/api/node_metrics.go's own
		// nodeResourceID exactly: that's the key handleQueryNodeMetrics
		// reads nodeHostMetrics samples back from.
		diskCollector := telemetry.NewHostDiskCollector(agentDataDir, "node:"+meshCfg.localNodeID, telemetryDB, metricsCollectionInterval, logger)
		go func() {
			if err := diskCollector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("telemetry host disk collector stopped", slog.String("error", err.Error()))
			}
		}()

		memoryCollector := telemetry.NewHostMemoryCollector("node:"+meshCfg.localNodeID, telemetryDB, metricsCollectionInterval, logger)
		go func() {
			if err := memoryCollector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("telemetry host memory collector stopped", slog.String("error", err.Error()))
			}
		}()

		patchCollector := telemetry.NewHostPatchCollector(nil, "node:"+meshCfg.localNodeID, telemetryDB, osPatchCheckInterval(), logger)
		go func() {
			if err := patchCollector.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("telemetry host patch collector stopped", slog.String("error", err.Error()))
			}
		}()
	}

	logCollector := telemetry.NewLogCollector(client, telemetryDB, logBroadcaster, logger)
	logCollector.SetNodeSource(&multiNodeLogSource{local: client, registry: agentRegistry})
	go func() {
		if err := logCollector.Run(ctx, logTargetsResyncInterval, logTargets(db, client, agentRegistry, meshCfg, logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry log collector stopped", slog.String("error", err.Error()))
		}
	}()
	go wakeLogCollectorOnContainerEvents(ctx, client, logCollector)
	go runLogsRetentionSweep(ctx, telemetryDB, logger)

	// A pure extra consumer of the log stream: never touches logCollector or its store writes.
	drainForwarder := telemetry.NewDrainForwarder(logBroadcaster, notifyClient, logger, b.ShortName)
	go func() {
		if err := drainForwarder.Run(ctx, logTargetsResyncInterval, drainTargets(db)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry log drain forwarder stopped", slog.String("error", err.Error()))
		}
	}()
}

// startAlerting starts the alerting engine and its sweepers.
func (s *bootState) startAlerting() {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	alertingDB := s.alertingDB
	apiRouter := s.apiRouter
	b := s.b
	client := s.client
	cpDR := s.cpDR
	db := s.db
	deployDispatcher := s.deployDispatcher
	emailSender := s.emailSender
	engine := s.engine
	gate := s.gate
	meshCfg := s.meshCfg
	notifyClient := s.notifyClient
	pluginMgr := s.pluginMgr
	pushSender := s.pushSender
	telemetryDB := s.telemetryDB
	// WithRecorder also persists restarts as a container_restart_count sample for the metrics dashboard.
	restartTracker := alerting.NewRestartTracker().WithRecorder(telemetryDB, logger)
	go func() {
		if err := restartTracker.Run(ctx, client, db, restartTrackerResyncInterval, logger); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("alerting restart tracker stopped", slog.String("error", err.Error()))
		}
	}()

	alertingFederator := telemetry.NewLocalFederator(telemetryDB)
	pluginSender := pluginSenderFor(pluginMgr)
	alertingNewNotifier := func(r alerting.Rule) alerting.Notifier {
		return alerting.NewNotifierWithPlugins(notifyClient, alerting.TemplatedSender{Sender: emailSender, Render: alertMailRenderer(db, *b)}, pushSender, pluginSender, r)
	}
	// db and apiRouter satisfy the alerting source interfaces structurally, so no adapters are needed.
	tlsSuppress := alerting.CertSuppressor(func(c context.Context) func(string) bool {
		return tlsowner.Load(c, db, db, nil).Suppressed
	})
	alertingEngine := alerting.NewEngine(alertingDB, alertingFederator, alertingFederator, restartTracker, alerting.NewSuppressedCertSource(db, tlsSuppress), db,
		certExpiryWarningWindow(logger), certRenewalStalledThreshold(logger), db, patchStatusThreshold(logger), nodeDiskSpaceThreshold(logger),
		db, nodeCPUThreshold(logger), nodeMemoryThreshold(logger), db, apiRouter, domainHealthCheckInterval(logger),
		db, backupMissingGracePeriod(logger), alertingNewNotifier, logger)
	alertingEngine.SetLogArchive(objectstore.HealthSource{Store: db})
	alertingEngine.SetExportHealth(exportHealthSource{db: db})
	alertingEngine.SetBackupCopy(backupCopyAlertSource{db: db}, backup.CopyLagAlertFromEnv(os.LookupEnv))
	alertingEngine.SetRegistryRefresh(registryRefreshSource{db: db})
	alertingEngine.SetNodeCertThresholds(nodeCertThresholds())
	alertingEngine.SetTrafficSources(db, alerting.NewSuppressedACMESource(ingressdriver.DefaultACMEFailures(), tlsSuppress))
	certPolicy := alerting.CertPolicyFromEnv(os.Getenv)
	certPolicy.BaseURL = dashboardBaseURL()
	alertingEngine.SetCertExpiryPolicy(certPolicy)
	alertingEngine.SetVersionSkew(db, upgrade.DefaultFetchers(b.RepoSlug()))
	alertingEngine.SetBudgets(apiRouter)
	alertingEngine.SetEmailDelivery(db)
	alertingEngine.SetDatabaseHealth(apiRouter)
	if src := s.deployDispatcher.Outcomes(); src != nil {
		alertingEngine.SetDeployFailures(src)
	}
	routeDefaultAlerts(ctx, logger, alertingDB, db, emailSender)
	go func() {
		if err := apiRouter.RunRegistryRefresher(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("registry credential refresh stopped", slog.String("error", err.Error()))
		}
	}()
	go runUsageRollup(ctx, apiRouter)
	go footprint.RunTrim(ctx, logger, os.Getenv)
	go func() {
		if err := footprint.Every(ctx, "database teardown retry", time.Minute, logger, apiRouter.RetryDatabaseTeardowns); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("database teardown retry stopped", slog.String("error", err.Error()))
		}
	}()
	go func() {
		prune := func(c context.Context) error {
			_, err := pruneEmailDeliveries(c, db, emailLogRetention(c, apiRouter.RuntimeSettings()), time.Now())
			return err
		}
		if err := footprint.Every(ctx, "email delivery log prune", time.Hour, logger, prune); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("email delivery log prune stopped", slog.String("error", err.Error()))
		}
	}()
	go runSpendCaps(ctx, apiRouter)
	go runCostBudgets(ctx, apiRouter)
	go runAutoscaler(ctx, apiRouter, autoscale.IntervalFromEnv(autoscale.OSEnv))
	alertingEngine.SetChanges(withLogSignatures(changes.New(db, db, db, logger), alertingFederator), alertDashboardLink())
	alertingEngine.SetNoiseControl(alerting.NewNoiseControl(alertNoiseConfig(logger, apiRouter.RuntimeSettings()), alertingDB, db, logger))
	go func() {
		if err := apiRouter.RunStatusPageSampler(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("status page sampler stopped", slog.String("error", err.Error()))
		}
	}()
	if localScheduled := controlPlaneBackupInterval(logger) > 0; localScheduled || cpDR != nil {
		alertingEngine.SetControlPlaneBackups(cpbackup.AlertSource{Local: cpbackup.NewManager(db, agentDataDir), Svc: cpDR, LocalScheduled: localScheduled}, 0)
	}
	// Opt-in per app (AutoRollbackOnCrashloop, off by default), so wiring it unconditionally is safe.
	alertingEngine.SetAutoRollback(db, engine)
	// Opt-in per app (AutoRollbackOnSLOBurn, off by default), like the crashloop rollback.
	alertingEngine.SetSLOBurnAutoRollback(db, engine, alertingDB)
	startDeployWatch(ctx, gate, deployWatchDeps{db: db, local: client, registry: agentRegistry, localNodeID: meshLocalNodeID(meshCfg),
		restarts: restartTracker, metrics: alertingFederator, sampleEvery: metricsCollectionInterval, nudger: engine, dispatcher: deployDispatcher, logger: logger})
	go func() {
		if err := alertingEngine.Run(ctx, alertEvaluationInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("alerting engine stopped", slog.String("error", err.Error()))
		}
	}()
	s.alertingEngine = alertingEngine
}

// startSchedulers starts the backup, task, sleep and housekeeping schedulers.
func (s *bootState) startSchedulers() {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	agentRegistry := s.agentRegistry
	alertingDB := s.alertingDB
	alertingEngine := s.alertingEngine
	apiRouter := s.apiRouter
	b := s.b
	backupRunner := s.backupRunner
	backupVerifyRunner := s.backupVerifyRunner
	client := s.client
	db := s.db
	deployDispatcher := s.deployDispatcher
	engine := s.engine
	meshCfg := s.meshCfg
	scheduledTaskRunner := s.scheduledTaskRunner
	secretsManager := s.secretsManager
	telemetryDB := s.telemetryDB
	// Gated on backupRunner: a scheduled backup needs the same master-key-backed credentials as a manual one.
	if backupRunner != nil {
		scheduler := backup.NewScheduler(db, backupRunner, routedStorage(agentRegistry), logger)
		// Every scheduled backup is auto-verified right after it
		// completes, no opt-in toggle: same "just works" default the
		// manual verify button already gives an operator on demand.
		scheduler.Verifier = backupVerifyRunner
		scheduler.Policies, scheduler.GFS = db, db
		scheduler.Tombstones = db
		scheduler.BaseBackups = newBaseBackupRunner(db, secretsManager, client, backupRunner, logger)
		walShip := newWALShipScheduler(db, client, backupRunner, func(nodeID string) bool {
			return nodeID == "" || (meshCfg != nil && nodeID == meshCfg.localNodeID)
		}, logger)
		apiRouter.SetWALShipStatus(walShip)
		startWhenWork(ctx, logger, "backups", hasScheduledBackups(db), func() {
			startBackupDrills(ctx, logger, db, newBackupProtection(db, secretsManager, client, backupRunner, logger), alertingEngine)
			startBackupCopies(ctx, logger, db, secretsManager, routedStorage(agentRegistry))
			go func() {
				if err := scheduler.Run(ctx, backupSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("backup scheduler stopped", slog.String("error", err.Error()))
				}
			}()
			go func() {
				if err := walShip.Run(ctx, walShipInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
					logger.Error("wal ship scheduler stopped", slog.String("error", err.Error()))
				}
			}()
		})
	}

	// Always runs: scheduledTaskRunner needs no master key, so this loop is never gated.
	localNodeID := ""
	if meshCfg != nil {
		localNodeID = meshCfg.localNodeID
	}
	scheduledTaskRunner.HTTPConfigs = db
	scheduledTaskRunner.Target = jobHTTPTarget{
		db: db, nudger: engine, local: client,
		nodes: lbNodeUpstreams{db: db, local: client, registry: agentRegistry, localNodeID: localNodeID},
	}
	s.functionQueue.Target = scheduledTaskRunner.Target
	go func() {
		if err := s.functionQueue.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("function queue stopped", slog.String("error", err.Error()))
		}
	}()
	scheduledTaskScheduler := scheduledtask.NewScheduler(db, scheduledTaskRunner, logger)
	scheduledTaskScheduler.Lookback = scheduledTaskRunner.Limits.CatchUpLookback
	go func() {
		if err := scheduledTaskScheduler.Run(ctx, scheduledTaskSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduled task scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	// Fires through the same git-source deploy path a webhook push uses.
	appScheduleScheduler := scheduledeploy.NewScheduler(db, db, apiRouter, logger)
	appScheduleScheduler.OneShotCatchUp = appScheduleOneShotCatchUp(logger)
	go func() {
		if err := appScheduleScheduler.Run(ctx, appScheduleSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduled deploy scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	oneShotRunner := &scheduledeploy.OneShotRunner{
		Store: db, Freeze: db, Exec: apiRouter, Notify: apiRouter, Logger: logger,
		Grace: scheduledDeployGrace(logger), StaleRun: scheduledDeployStaleRun(logger),
	}
	go func() {
		if err := oneShotRunner.Run(ctx, appScheduleSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("one-shot scheduled deploy runner stopped", slog.String("error", err.Error()))
		}
	}()

	sleepCfg := appsleep.ConfigFromEnv(os.Getenv)
	portWake := &appsleep.PortWakeManager{
		Store: db, Apps: db, Nudger: engine, Events: db, Logger: logger, Cfg: sleepCfg, Ready: apiRouter.AppReadyAfter,
		IsLocal: func(nodeID string) bool { return nodeID == "" || (meshCfg != nil && nodeID == meshCfg.localNodeID) },
	}
	appSleepScheduler := &appsleep.Scheduler{
		Sleep: db, Apps: db, Traffic: telemetryDB, Nudger: engine, Events: db, Logger: logger, Cfg: sleepCfg,
		Metrics: telemetryDB, Breakdowns: telemetryDB, Conns: portWake, Ops: db, Fleet: db, PortWake: portWake, PortSync: portWake,
	}
	apiRouter.SetSleepRuntime(portWake, appSleepScheduler)
	go func() {
		if err := portWake.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("app sleep port wake stopped", slog.String("error", err.Error()))
		}
	}()
	go func() {
		if err := appSleepScheduler.Run(ctx, appsleep.DefaultInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("app sleep scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	deployURLSyncer := &deployurls.Syncer{
		Store: db, Config: deployurls.FromProcessEnv(), HostFor: apiRouter.DeployURLHost,
		Remove: apiRouter.RemoveDeployURLService, Nudge: apiRouter.NudgeReconcilers, Logger: logger,
	}
	go func() {
		if err := deployURLSyncer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("deploy url syncer stopped", slog.String("error", err.Error()))
		}
	}()

	dbAccessSweeper := &dbaccess.Sweeper{Store: dbaccess.StoreAdapter{DB: db}, Revoker: apiRouter, Auditor: apiRouter, Logger: logger}
	go func() {
		if err := dbAccessSweeper.Run(ctx, dbaccess.SweepIntervalFromEnv(os.Getenv)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("database temporary credential sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	digestRunner := &alerting.DigestRunner{DB: alertingDB, Sender: deployDispatcher, Logger: logger}
	go func() {
		if err := digestRunner.Run(ctx, 10*time.Minute); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("inbox digest stopped", slog.String("error", err.Error()))
		}
	}()

	inboxTidy := &alerting.TidyRunner{DB: alertingDB, Cfg: alerting.TidyConfigFromEnv(os.Getenv), Logger: logger}
	go func() {
		if err := inboxTidy.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("inbox tidy stopped", slog.String("error", err.Error()))
		}
	}()

	startSecretRotation(ctx, logger, db, apiRouter, secretsManager != nil)

	startDatabaseUpgrades(ctx, logger, db, secretsManager, client, engine.Nudge, backupRunner, backupVerifyRunner, alertingDB, deployDispatcher,
		func(nodeID string) bool { return nodeID == "" || (meshCfg != nil && nodeID == meshCfg.localNodeID) }, apiRouter)

	go func() {
		if err := apiRouter.RunDatabasePasswordRotation(ctx, databasePasswordRotationInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("database password rotation stopped", slog.String("error", err.Error()))
		}
	}()

	imageUpdateScheduler := &imageupdate.Scheduler{Store: db, Checker: apiRouter, Logger: logger}
	go func() {
		if err := imageUpdateScheduler.Run(ctx, imageupdate.IntervalFromEnv(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("image update scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	go apiRouter.RunCertWatchdog(ctx)

	// Only checks when an operator has opted into auto_update_enabled.
	startUpgradeScheduler(ctx, logger, apiRouter)
	updateCheckScheduler := updatecheck.NewScheduler(db, logger, b.RepoSlug())
	go func() {
		if err := updateCheckScheduler.Run(ctx, updateCheckInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("update check scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	if interval := controlPlaneBackupInterval(logger); interval > 0 {
		cpBackups := cpbackup.NewManager(db, agentDataDir)
		retain := controlPlaneBackupRetain(logger)
		go cpBackups.Run(ctx, interval, retain, logger)
	} else {
		logger.Info("scheduled control plane backups disabled (APP_CONTROL_PLANE_BACKUP_INTERVAL=0)")
	}

	// Fallback for a PR-closed webhook that never arrived; needs no master key.
	go sweepOrphanPreviewsOnce(ctx, logger, apiRouter)
	go runStaticOrphanSweep(ctx, logger, apiRouter)
	go func() {
		if err := apiRouter.RunPreviewSweeper(ctx, previewSweepInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("preview sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	// Enforces the operator's audit retention window so audit_log does not grow forever.
	go func() {
		if err := apiRouter.RunAuditLogSweeper(ctx, auditLogSweepInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("audit log sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	// Moves cold audit, deploy and pipeline rows into archive bundles when the policy is on.
	go func() {
		if err := apiRouter.RunArchiveSweeper(ctx, 0); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("archive sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	// Device login expiry sweep (api.Router.RunDeviceLoginExpirySweeper):
	// audits each lapsed CLI login exactly once, even when no CLI polls.
	go func() {
		if err := apiRouter.RunDeviceLoginExpirySweeper(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("device login expiry sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	// Same reasoning as the audit log sweep, for the only inbound-webhook history table.
	go func() {
		if err := apiRouter.RunWebhookDeliverySweeper(ctx, webhookDeliverySweepInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("webhook delivery sweeper stopped", slog.String("error", err.Error()))
		}
	}()

	// The per-request expiry check already blocks stale approvals; this sweep only updates UI state.
	go func() {
		if err := apiRouter.RunDeployApprovalExpirySweep(ctx, deployApprovalSweepInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("deploy approval expiry sweeper stopped", slog.String("error", err.Error()))
		}
	}()
}

// serve serves HTTP and runs the reconcile engine until shutdown.
func (s *bootState) serve() error {
	ctx, logger := s.ctx, s.logger
	client := s.client
	engine := s.engine
	httpServer := s.httpServer
	events := resilientEvents(ctx, client.Events, logger)

	serveErrs := make(chan error, 1)
	go func() {
		logger.Info("http api listening", slog.String("addr", httpServer.Addr))
		exposedHTTPAddrWarning.warn(logger, httpServer.Addr, runningInContainer())
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrs <- err
			return
		}
		serveErrs <- nil
	}()

	healthy := time.AfterFunc(s.bootPolicy.HealthyAfter, s.ledger.MarkHealthy)
	defer healthy.Stop()

	engine.SetDrainTimeout(envDurationOr(logger, shutdownDrainEnv, defaultShutdownDrain))
	go func() {
		<-ctx.Done()
		if n := s.apiRouter.BeginShutdown(); n > 0 {
			logger.Info("closed live streams for shutdown", slog.Int("streams", n))
		}
	}()
	engineErr := engine.Run(ctx, events, resyncInterval)

	drainCtx, drainCancel := context.WithTimeout(context.Background(), envDurationOr(logger, shutdownDrainEnv, defaultShutdownDrain))
	if !s.apiRouter.WaitDeploys(drainCtx) {
		logger.Warn("shutdown drain limit reached with deploys still running, they resume as failed on next boot")
	}
	drainCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", slog.String("error", err.Error()))
	}
	if serveErr := <-serveErrs; serveErr != nil {
		logger.Error("http server error", slog.String("error", serveErr.Error()))
	}

	if errors.Is(engineErr, context.Canceled) {
		if s.restartRequested.Load() {
			return errRestartRequested
		}
		return nil // Ctrl+C / SIGTERM is a clean shutdown, not a failure
	}
	return engineErr
}
