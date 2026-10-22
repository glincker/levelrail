package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/integrity"
	"github.com/GLINCKER/levelrail/internal/migrateagent"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/webpush"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// openStore opens the SQLite store and records upgrade history.
func (s *bootState) openStore() error {
	ctx, logger := s.ctx, s.logger
	obs := upgradehistory.Observed{SchemaBefore: -1}
	db, err := openStoreObserved(ctx, &obs)
	if errors.Is(err, store.ErrSchemaNewer) {
		return newDowngradeRefusal(ctx, dataDirFromEnv())
	}
	if err != nil {
		return classifyStoreError(err, 0)
	}
	s.later(func() {
		if cerr := db.Close(); cerr != nil {
			logger.Error("closing store", slog.String("error", cerr.Error()))
		}
	})
	recordUpgradeHistory(ctx, db, obs, logger)
	applyTakeoverBoot(ctx, db, logger)
	importSelfUpgradeJournal(ctx, db, logger)
	resolveServerProfile(ctx, db, logger)
	if err := api.LoadBetaFeatures(ctx, db); err != nil {
		logger.Warn("beta feature switches not loaded", slog.String("error", err.Error()))
	}

	if moved, merr := db.ReconcileInstanceHost(ctx, dataDirFromEnv()); merr != nil {
		logger.Warn("instance host check failed", slog.String("error", merr.Error()))
	} else if moved {
		logger.Warn("database restored onto a new host: instance identity rotated, CLI tokens bound to the old one are refused until an owner confirms it is the same instance")
	}
	s.db = db
	dataDir := dataDirFromEnv()
	s.integrity = integrity.NewService(db.DB, dataDir, cpbackup.NewManager(db, dataDir), logger)
	quick, full := integrity.IntervalsFromEnv(os.Getenv)
	go s.integrity.Run(ctx, quick, full)
	return nil
}

// openDocker creates the Docker client behind the privilege guard.
func (s *bootState) openDocker() error {
	ctx, logger := s.ctx, s.logger
	db := s.db
	// Resolved before the Docker client so every container created from the first pass carries it.
	instanceID, err := db.GetOrCreateInstanceID(ctx)
	if err != nil {
		logger.Warn("instance id not available: cross-instance Docker cleanup safety is disabled", slog.String("error", err.Error()))
	}

	guard, err := bootDockerGuard(ctx, logger, db)
	if err != nil {
		return err
	}
	clientOpts := guardClientOptions(guard)
	if instanceID != "" {
		clientOpts = append(clientOpts, docker.WithInstanceLabel(spec.InstanceLabelKey, instanceID))
	}
	client, err := docker.NewClient(clientOpts...)
	if err != nil {
		return err
	}
	s.later(func() {
		if cerr := client.Close(); cerr != nil {
			logger.Error("closing docker client", slog.String("error", cerr.Error()))
		}
	})
	dockerguard.LogPrivilege(ctx, logger, client)

	// Must run before anything below can create a new deploy attempt:
	// see FailOrphanedDeployAttempts' own doc comment.
	if n, err := db.FailOrphanedDeployAttempts(ctx, time.Now()); err != nil {
		logger.Error("fail orphaned deploy attempts", slog.String("error", err.Error()))
	} else if n > 0 {
		logger.Warn("marked orphaned deploy attempts as failed", slog.Int("count", n))
	}
	s.instanceID = instanceID
	s.guard = guard
	s.client = client
	return nil
}

// bootIdentity loads the brand and bootstraps the admin account.
func (s *bootState) bootIdentity() error {
	ctx, logger := s.ctx, s.logger
	db := s.db
	b, err := loadBrand()
	if err != nil {
		return err
	}

	if err := bootstrapAdmin(ctx, db); err != nil {
		logger.Warn("admin account not bootstrapped", slog.String("error", err.Error()))
	}

	// Runs after bootstrapAdmin so an operator-set admin credential always wins over the dev fallback.
	if err := api.MaybeBootstrapDevAdmin(ctx, db, logger); err != nil {
		logger.Warn("dev admin account not bootstrapped", slog.String("error", err.Error()))
	}

	if err := applyLocalMode(ctx, logger, db); err != nil {
		return err
	}

	ensureSetupToken(ctx, logger, db)

	// A missing dev-fixtures file is fine: it is an opt-in layer on dev mode.
	if err := api.MaybeSeedDevFixturesFromFile(ctx, db, logger, devFixturesFile()); err != nil {
		logger.Warn("dev fixtures not seeded", slog.String("error", err.Error()))
	}
	s.b = b
	return nil
}

// openTelemetryStores opens the telemetry and alerting stores.
func (s *bootState) openTelemetryStores() error {
	ctx, logger := s.ctx, s.logger
	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	configureTelemetryTiers(telemetryDB)
	s.later(func() {
		if cerr := telemetryDB.Close(); cerr != nil {
			logger.Error("closing telemetry store", slog.String("error", cerr.Error()))
		}
	})

	// Shared by reference: a webhook build's live output must reach SSE viewers on the same subscriber set.
	deployRecorder := deploylog.NewRecorder(telemetryDB, logger)

	// Shared by reference so the publisher and the SSE subscribers see the same fan-out.
	logBroadcaster := telemetry.NewLogBroadcaster()

	alertingDB, err := openAlertingStore(ctx)
	if err != nil {
		return err
	}
	s.later(func() {
		if cerr := alertingDB.Close(); cerr != nil {
			logger.Error("closing alerting store", slog.String("error", cerr.Error()))
		}
	})
	s.telemetryDB = telemetryDB
	s.deployRecorder = deployRecorder
	s.logBroadcaster = logBroadcaster
	s.alertingDB = alertingDB
	seedDefaultAlerts(ctx, logger, alertingDB)
	return nil
}

// startAgentServer starts the node agent gRPC server.
func (s *bootState) startAgentServer() error {
	ctx, logger := s.ctx, s.logger
	client := s.client
	db := s.db
	// Built before node placement consumes it, so a control plane with no remote nodes still accepts agents.
	agentDataDir := os.Getenv("APP_DATA_DIR")
	if agentDataDir == "" {
		agentDataDir = defaultDataDir
	}
	agentCA, err := loadOrGenerateAgentCA(agentDataDir)
	if err != nil {
		return fmt.Errorf("load or generate agent CA: %w", err)
	}
	agentRegistry := agent.NewRegistry()
	if id := readLocalNodeID(agentDataDir); id != "" {
		agentRegistry.SetLocal(id, agent.NewLocal(client))
	}
	var agentServerRef atomic.Pointer[agent.Server]
	agentCreds, err := agent.NewServerCredentials(agentCA, []string{advertiseHostFromStore(ctx, db)}, agentServerCertValidity,
		agent.WithRejectObserver(func(r agent.RejectedHandshake) {
			if srv := agentServerRef.Load(); srv != nil {
				srv.ObserveRejected(r)
			}
		}))
	if err != nil {
		return fmt.Errorf("build agent grpc credentials: %w", err)
	}
	agentListener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", agentListenAddr(ctx, db, readLocalNodeID(agentDataDir)))
	if err != nil {
		return fmt.Errorf("start agent grpc listener: %w", err)
	}
	agentGRPCServer := grpc.NewServer(grpc.Creds(agentCreds),
		// Transport backstop for the Heartbeat frame: a frozen agent fails PING too, not only heartbeats.
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    nodeKeepaliveTime(),
			Timeout: nodeKeepaliveTimeout(),
		}),
		// MinTime rejects agents that ping faster than their own keepalive interval allows.
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             nodeKeepaliveTime() / 2,
			PermitWithoutStream: true,
		}),
	)
	meshEndpoints := newMeshEndpointBook(logger)
	hostReports := agent.NewHostReportStore()
	agentServer := agent.NewServer(agentCA, db, agentRegistry, logger, append(agentCertServerOptions(db), agent.WithGPUSink(db), agent.WithHostReportSink(hostReports), agent.WithSessionObserver(meshEndpoints.Observe))...)
	agentServerRef.Store(agentServer)
	agentpb.RegisterAgentServiceServer(agentGRPCServer, agentServer)
	migrationAgent := migrateagent.NewService(db, agentCA, migrateagent.ConfigFromEnv(), logger, nil)
	migrationpb.RegisterMigrationServiceServer(agentGRPCServer, migrationAgent)
	go func() {
		logger.Info("agent grpc listening", slog.String("addr", agentListener.Addr().String()))
		if err := agentGRPCServer.Serve(agentListener); err != nil {
			logger.Error("agent grpc server stopped", slog.String("error", err.Error()))
		}
	}()
	// Fires early on SIGTERM so a well-behaved agent's GoAway completes during the rest of shutdown.
	go func() {
		<-ctx.Done()
		agentServer.NotifyShutdown()
	}()
	s.later(func() { stopAgentGRPCServer(agentGRPCServer, logger, agentGRPCShutdownTimeout) })
	s.agentDataDir = agentDataDir
	s.agentCA = agentCA
	s.agentRegistry = agentRegistry
	s.meshEndpoints = meshEndpoints
	s.hostReports = hostReports
	s.agentServer = agentServer
	s.migrationAgent = migrationAgent
	return nil
}

// loadSecretsAndNotifiers loads secrets and builds the notification senders.
func (s *bootState) loadSecretsAndNotifiers() {
	ctx, logger := s.ctx, s.logger
	agentDataDir := s.agentDataDir
	alertingDB := s.alertingDB
	b := s.b
	db := s.db
	telemetryDB := s.telemetryDB
	secretsManager, masterKeyFilePath, err := loadSecretsManager(db, agentDataDir, secretsManagerOptions(logger)...)
	if err != nil {
		// Non-fatal, like bootstrapAdmin: secrets-dependent routes fail specifically instead of refusing startup.
		logger.Warn("secrets not configured", slog.String("error", err.Error()))
	}
	installAuditChainKey(ctx, logger, secretsManager)
	setupNotificationChannelSecrets(ctx, logger, alertingDB, secretsManager)

	// Always non-nil: email settings resolve on every send, so 'not configured' surfaces at send time.
	emailSender := email.NewDynamicSender(emailConfigLoader(db, secretsManager, smtpConfigFromEnv())).
		WithRecorder(newEmailDeliveryRecorder(db)).
		WithRetry(emailEnvInt("APP_EMAIL_RETRY_ATTEMPTS"), time.Duration(emailEnvInt("APP_EMAIL_RETRY_BASE_MS"))*time.Millisecond)
	installGitNetguard()
	notifyClient := netguard.NewClient()

	// Nil until a master key exists: the VAPID private key can only be stored through secretsManager.
	var pushSender alerting.PushSender
	var pushVAPIDPublicKey string
	if secretsManager != nil {
		publicKey, privateKey, err := webpush.EnsureVAPIDKeys(ctx, secretsManager)
		if err != nil {
			logger.Error("webpush: ensure vapid keys failed", slog.String("error", err.Error()))
		} else {
			pushVAPIDPublicKey = publicKey
			subscriber := b.SupportEmail
			if subscriber == "" {
				subscriber = b.SupportURL
			}
			pushSender = webpush.NewSender(db, publicKey, privateKey, subscriber, notifyClient, logger)
		}
	}

	deployDispatcher := alerting.NewDeployDispatcher(alertingDB, notifyClient, alerting.TemplatedSender{Sender: emailSender, Render: alertMailRenderer(db, *b)}, pushSender, logger).WithLogSources(telemetryDB, telemetryDB).WithOutcomeTracker(alerting.NewDeployOutcomeTracker())
	s.secretsManager = secretsManager
	s.masterKeyFilePath = masterKeyFilePath
	s.emailSender = emailSender
	s.notifyClient = notifyClient
	s.pushSender = pushSender
	s.pushVAPIDPublicKey = pushVAPIDPublicKey
	s.deployDispatcher = deployDispatcher
}
