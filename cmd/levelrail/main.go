// Command levelrail is the control plane binary.
// Controllers are derived from desired state on every reconcile pass (see dynamicSource).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/appsleep"
	"github.com/GLINCKER/levelrail/internal/autoscale"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/canary"
	"github.com/GLINCKER/levelrail/internal/changelog"
	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/deployurls"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/imageupdate"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/orphans"
	"github.com/GLINCKER/levelrail/internal/pkgregistry"
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
	"github.com/GLINCKER/levelrail/internal/scheduledeploy"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/sharedenv"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/tlsowner"
	"github.com/GLINCKER/levelrail/internal/updatecheck"
	"github.com/GLINCKER/levelrail/internal/upgradehistory"
	"github.com/GLINCKER/levelrail/internal/vault"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/GLINCKER/levelrail/internal/webhook"
	"github.com/GLINCKER/levelrail/internal/webpush"
	"github.com/GLINCKER/levelrail/kit/firewall"
	"github.com/GLINCKER/levelrail/kit/netguard"
	"github.com/GLINCKER/levelrail/kit/probe"
	"github.com/GLINCKER/levelrail/kit/upgrade"
	"github.com/GLINCKER/levelrail/web"
)

const (
	resyncInterval = 30 * time.Second
	// Relative, not /var/lib/levelrail-data: Phase 0 runs locally, not as an installed service.
	defaultDataDir = "./data"
	// defaultBrandFile is the same "relative default, env override"
	// pattern as defaultDataDir.
	defaultBrandFile = "./brand.yaml"
	// Absence is not an error: the What's new panel just shows no entries.
	defaultChangelogFile = "./CHANGELOG.md"
	// Absence is not an error: the built-in manifest config is used instead.
	defaultGitHubAppManifestFile = "./github-app-manifest.yaml"
	// Only read when APP_DEV_MODE=1.
	defaultDevFixturesFile = "./dev-fixtures.yml"
	// defaultHTTPAddr is where the HTTP API listens.
	defaultHTTPAddr = ":8080"

	// Overridable so a second control plane on the same host does not fail to bind :443.
	defaultIngressHTTPSAddr = ":443"
	// Overridable via APP_INGRESS_HTTP_ADDR.
	defaultIngressHTTPAddr = ":80"

	// A separate port: agent gRPC uses mTLS with its own CA, unlike the HTTP API's session auth.
	defaultAgentAddr = ":9443"
	// Regenerated on each start; only the CA persists, so agents keep trusting it.
	agentServerCertValidity = 365 * 24 * time.Hour

	httpShutdownTimeout = 10 * time.Second

	// Bounded so a live agent Session stream cannot stall exit until systemd SIGKILLs the process.
	agentGRPCShutdownTimeout = 5 * time.Second

	// Not env-configurable: resolution changes the stored sample shape, retention does not.
	metricsCollectionInterval = 15 * time.Second

	// Separate from resyncInterval: log stream subscriptions are a different concern.
	logTargetsResyncInterval = 30 * time.Second
	// defaultLogsRetention matches the observability design's "default
	// 15 days," the same default metrics uses, applied to the sibling
	// log store.
	defaultLogsRetention = 15 * 24 * time.Hour
	// logsRetentionSweepInterval mirrors metricsRetentionSweepInterval's
	// own reasoning: frequent enough that log_entries never grows much
	// past its steady-state size, infrequent enough to be a non-event.
	logsRetentionSweepInterval = 1 * time.Hour

	// Coarser than metrics collection: a rule's ForDuration already debounces noisy samples.
	alertEvaluationInterval = 30 * time.Second

	// Resync only: start events are handled continuously by the event stream.
	restartTrackerResyncInterval = 30 * time.Second

	// Duplicated rather than imported so this file's env-var convention stays self-contained.
	defaultNodeHeartbeatInterval = 15 * time.Second
	// Three missed heartbeats: long enough to ride out a GC pause, short enough to catch a hang.
	defaultNodeHeartbeatTimeout = 45 * time.Second

	// Must stay under defaultNodeHeartbeatTimeout so the transport notices before nodehealth does.
	defaultNodeKeepaliveTime    = 10 * time.Second
	defaultNodeKeepaliveTimeout = 10 * time.Second

	// Shells out to a package manager, so an hour is a conservative default.
	defaultOSPatchCheckInterval = 1 * time.Hour

	// One minute: standard cron has no finer granularity, and a coarser check would miss minute schedules.
	defaultBackupSchedulerInterval = 1 * time.Minute

	// Same one-minute value as the backup scheduler, for the same cron granularity reason.
	defaultScheduledTaskSchedulerInterval = 1 * time.Minute

	// Same one-minute value as the backup scheduler, for the same cron granularity reason.
	defaultAppScheduleSchedulerInterval = 1 * time.Minute

	// An hour: a days-scale TTL needs no minute-granularity checks.
	defaultPreviewSweepInterval = 1 * time.Hour

	// An hour: a release check is outbound network cost with no benefit from polling faster.
	defaultUpdateCheckInterval = 1 * time.Hour

	// An hour: a days-scale retention window needs no finer checks.
	defaultAuditLogSweepInterval = 1 * time.Hour

	// Hourly, like the audit log sweep, for a days-scale retention window.
	defaultWebhookDeliverySweepInterval = 1 * time.Hour

	// Minutes, not hourly: a stale pending approval should show as expired promptly.
	defaultDeployApprovalSweepInterval = 5 * time.Minute

	// Budgets sit well above normal dashboard, CLI, and deploy cadence; only runaway loops hit them.
	defaultAPIRateLimitReadRPM  = 600
	defaultAPIRateLimitWriteRPM = 120

	// Well above any real git provider's delivery cadence; only a client hammering this route hits it.
	defaultWebhookRateLimitRPM = 60

	// defaultTokenRedeemRateLimitRPM is the per-IP budget for reset-password
	// and invite-accept when APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM is unset.
	defaultTokenRedeemRateLimitRPM = 10
)

func main() {
	if code, handled := bootArgsExit(os.Args[1:], os.Args[0], os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if len(os.Args) > 1 && runReleaseCommand(os.Args[1], os.Args[2:]) {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "demo" {
		if err := runDemo(context.Background(), logger, os.Args[2:], os.Stdout); err != nil {
			logger.Error("demo failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "recover-admin" {
		if err := runRecoverAdmin(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("recover-admin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "approve-signin" {
		if err := runApproveSignIn(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("approve-signin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "emergency-token" {
		if err := runEmergencyToken(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("emergency-token failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "access-policy-reset" {
		if err := runAccessPolicyReset(context.Background(), os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("access-policy-reset failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore-db" {
		if err := runRestoreDB(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout); err != nil {
			logger.Error("restore-db failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "wrap-master-key" {
		if err := runWrapMasterKey(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout, os.Getenv); err != nil {
			logger.Error("wrap-master-key failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore-snapshot" {
		if err := runRestoreSnapshot(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdin, os.Stdout); err != nil {
			logger.Error("restore-snapshot failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "restore" {
		if err := runRestore(context.Background(), os.Args[2:], dataDirFromEnv(), os.Stdout, os.LookupEnv); err != nil {
			logger.Error("restore failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "setup-token" {
		if err := runSetupToken(context.Background(), os.Stdout, openStore); err != nil {
			logger.Error("setup-token failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		if err := runHealthcheck(context.Background(), os.Stdout, len(os.Args) > 2 && os.Args[2] == "--ready"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := run(logger); err != nil {
		var refusal *downgradeRefusal
		if errors.As(err, &refusal) {
			fmt.Fprint(os.Stderr, refusal.msg)
			os.Exit(selfupgrade.ExitDowngradeRefused)
		}
		if errors.Is(err, errRestartRequested) {
			logger.Info("exiting for restart", slog.Int("code", exitRestart))
			os.Exit(exitRestart)
		}
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	logger.Info("starting", slog.String("version", version.Version))
	if demo.Enabled(os.Getenv) {
		logger.Warn("ignoring " + demo.EnvDemoMode + ": demo mode only exists behind the demo subcommand")
	}
	if err := experimental.Validate(); err != nil {
		logger.Warn("ignoring unknown experimental feature", slog.String("error", err.Error()))
	}
	logger.Info("experimental features", slog.Any("enabled", experimental.EnabledList()))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	obs := upgradehistory.Observed{SchemaBefore: -1}
	db, err := openStoreObserved(ctx, &obs)
	if errors.Is(err, store.ErrSchemaNewer) {
		return newDowngradeRefusal(ctx, dataDirFromEnv())
	}
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logger.Error("closing store", slog.String("error", cerr.Error()))
		}
	}()
	recordUpgradeHistory(ctx, db, obs, logger)
	importSelfUpgradeJournal(ctx, db, logger)
	if err := api.LoadBetaFeatures(ctx, db); err != nil {
		logger.Warn("beta feature switches not loaded", slog.String("error", err.Error()))
	}

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
	defer func() {
		if cerr := client.Close(); cerr != nil {
			logger.Error("closing docker client", slog.String("error", cerr.Error()))
		}
	}()
	dockerguard.LogPrivilege(ctx, logger, client)

	// Must run before anything below can create a new deploy attempt:
	// see FailOrphanedDeployAttempts' own doc comment.
	if n, err := db.FailOrphanedDeployAttempts(ctx, time.Now()); err != nil {
		logger.Error("fail orphaned deploy attempts", slog.String("error", err.Error()))
	} else if n > 0 {
		logger.Warn("marked orphaned deploy attempts as failed", slog.Int("count", n))
	}

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

	ensureSetupToken(ctx, logger, db)

	// A missing dev-fixtures file is fine: it is an opt-in layer on dev mode.
	if err := api.MaybeSeedDevFixturesFromFile(ctx, db, logger, devFixturesFile()); err != nil {
		logger.Warn("dev fixtures not seeded", slog.String("error", err.Error()))
	}

	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	configureTelemetryTiers(telemetryDB)
	defer func() {
		if cerr := telemetryDB.Close(); cerr != nil {
			logger.Error("closing telemetry store", slog.String("error", cerr.Error()))
		}
	}()

	// Shared by reference: a webhook build's live output must reach SSE viewers on the same subscriber set.
	deployRecorder := deploylog.NewRecorder(telemetryDB, logger)

	// Shared by reference so the publisher and the SSE subscribers see the same fan-out.
	logBroadcaster := telemetry.NewLogBroadcaster()

	alertingDB, err := openAlertingStore(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := alertingDB.Close(); cerr != nil {
			logger.Error("closing alerting store", slog.String("error", cerr.Error()))
		}
	}()

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
	agentListener, err := net.Listen("tcp", agentAddr())
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
	defer stopAgentGRPCServer(agentGRPCServer, logger, agentGRPCShutdownTimeout)

	secretsManager, masterKeyFilePath, err := loadSecretsManager(db, agentDataDir, secretsManagerOptions(logger)...)
	if err != nil {
		// Non-fatal, like bootstrapAdmin: secrets-dependent routes fail specifically instead of refusing startup.
		logger.Warn("secrets not configured", slog.String("error", err.Error()))
	}
	setupNotificationChannelSecrets(ctx, logger, alertingDB, secretsManager)

	// Always non-nil: email settings resolve on every send, so 'not configured' surfaces at send time.
	emailSender := email.NewDynamicSender(emailConfigLoader(db, secretsManager, smtpConfigFromEnv()))
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

	deployDispatcher := alerting.NewDeployDispatcher(alertingDB, notifyClient, emailSender, pushSender, logger).WithLogSources(telemetryDB, telemetryDB)

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
			Uploader:          backup.S3Uploader{},
		}
		backupVerifyRunner = &backup.VerifyRunner{
			Store:      db,
			Secrets:    secretsManager,
			Downloader: backup.S3Downloader{},
		}
		volumeBackupOpts, err := newVolumeBackupOptions(db, client, agentRegistry, logger)
		if err != nil {
			return err
		}
		backupRunner.Volume = volumeBackupOpts
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

	supplyChainSvc := newSupplyChainService(ctx, logger, db, client, agentDataDir, b.ShortName)

	builder, closeBuilder, err := loadBuilder(ctx, logger, db, telemetryDB, secretsManager, agentRegistry, supplyChainSvc, guard)
	if err != nil {
		// Non-fatal: the control plane still starts, with build-dependent paths unavailable.
		logger.Warn("builder not configured: git webhook deploys and manual build trigger are both unavailable", slog.String("error", err.Error()))
	}
	if closeBuilder != nil {
		defer func() {
			if cerr := closeBuilder(); cerr != nil {
				logger.Error("closing build client", slog.String("error", cerr.Error()))
			}
		}()
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
	defer func() {
		if cerr := ingressDriver.Stop(context.Background()); cerr != nil {
			logger.Error("stopping ingress driver", slog.String("error", cerr.Error()))
		}
	}()

	pluginMgr := setupPlugins(ctx, logger, b, db, client, agentDataDir)
	var restartRequested atomic.Bool
	apiHandler, apiRouter := rootHandler(logger, b, db, telemetryDB, alertingDB, secretsManager, masterKeyFilePath, webhookHandler, client, builder, deployRecorder, logBroadcaster, deployDispatcher, backupRunner, backupVerifyRunner, agentRegistry, agentCA.Fingerprint(), emailSender, scheduledTaskRunner, engine, ingressDriver, pushVAPIDPublicKey)
	apiRouter.WarmTrustRoot()
	if wh, ok := webhookHandler.(*webhook.Handler); ok {
		wh.UseSourceCache(apiRouter.SourceCache())
	}
	if pluginMgr != nil {
		apiRouter.SetPluginManager(pluginMgr)
	}
	apiRouter.SetRestarter(api.Restarter{
		Supervised: restartSupervised(os.LookupEnv, fileExists),
		Trigger:    func() { restartRequested.Store(true); stop() },
	})
	configureNodeCerts(apiRouter, agentServer, agentServer)
	setupEdgeFirewall(ctx, logger, b, apiRouter, secretsManager)
	apiRouter.SetNodeHostReports(hostReports)
	gateSecret := setupBuiltinGate(ctx, logger, apiRouter)
	apiRouter.SetDockerGuard(guard.Controller)
	setupLogArchive(ctx, logger, db, telemetryDB, secretsManager, apiRouter)
	setupDataExport(ctx, logger, b, db, telemetryDB, secretsManager, logBroadcaster, apiRouter)
	setupEventWebhooks(ctx, logger, b, db, secretsManager, apiRouter)
	startScheduledMaintenance(ctx, logger, db, alertingDB, deployDispatcher, apiRouter)
	setupCompliance(ctx, logger, db, secretsManager, agentDataDir, apiRouter)
	startHeldDeployReleaser(ctx, logger, db, apiRouter)
	startCanaryReconciler(ctx, logger, db, apiRouter)
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
		defer meshCfg.close()
		apiRouter.SetLocalNodeID(meshCfg.localNodeID)
		agentRegistry.SetLocal(meshCfg.localNodeID, agent.NewLocal(client))
		apiRouter.SetMesh(meshCfg.device, meshCfg.coordinator)
	}
	apiRouter.SetMeshPaths(newMeshPathResolver(meshCfg, db))
	apiRouter.SetMeshPeerStatus(agent.NewGRPCSink(agentRegistry))
	go apiRouter.RunNodeMoves(ctx)
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

	edge, err := loadIngressEdge(logger)
	if err != nil {
		return fmt.Errorf("ingress edge settings: %w", err)
	}
	edgeInfo := edge.doctorInfo()
	edgeInfo.Snapshot = setupIngressSnapshot(ctx, logger, ingressDriver, db, secretsManager, edge)
	apiRouter.SetDoctorIngressEdge(edgeInfo)
	proxySyncer := newProxySyncer(b, db, logger)
	apiRouter.SetProxyIntegration(proxySyncer, db, proxyVerifyTimeout(logger))
	routeCheck := newRouteCheck(logger)
	apiRouter.SetProxyRouteCheck(routeCheck)
	apiRouter.ResumeCutoverRuns(ctx)

	engine.SetStore(db)
	gate := newDeployGate(logger, db, client, b.ShortName, meshLocalNodeID(meshCfg))
	sourceDeps := dynamicSourceDeps{
		deployGate:                   gate,
		orphanReaper:                 orphanReaper,
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
	go func() {
		if err := logCollector.Run(ctx, logTargetsResyncInterval, logTargets(db, client)); err != nil && !errors.Is(err, context.Canceled) {
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
		return alerting.NewNotifierWithPlugins(notifyClient, emailSender, pushSender, pluginSender, r)
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
	alertingEngine.SetNodeCertThresholds(nodeCertThresholds())
	alertingEngine.SetTrafficSources(db, alerting.NewSuppressedACMESource(ingressdriver.DefaultACMEFailures(), tlsSuppress))
	alertingEngine.SetVersionSkew(db, upgrade.DefaultFetchers(b.RepoSlug()))
	alertingEngine.SetBudgets(apiRouter)
	go runUsageRollup(ctx, apiRouter)
	go runSpendCaps(ctx, apiRouter)
	go runAutoscaler(ctx, apiRouter, autoscale.IntervalFromEnv(autoscale.OSEnv))
	alertingEngine.SetChanges(withLogSignatures(changes.New(db, db, db, logger), alertingFederator), alertDashboardLink())
	alertingEngine.SetNoiseControl(alerting.NewNoiseControl(alertNoiseConfig(logger), alertingDB, db, logger))
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

	// Gated on backupRunner: a scheduled backup needs the same master-key-backed credentials as a manual one.
	if backupRunner != nil {
		scheduler := backup.NewScheduler(db, backupRunner, backup.S3Deleter{}, logger)
		// Every scheduled backup is auto-verified right after it
		// completes, no opt-in toggle: same "just works" default the
		// manual verify button already gives an operator on demand.
		scheduler.Verifier = backupVerifyRunner
		scheduler.Policies, scheduler.GFS = db, db
		startBackupDrills(ctx, logger, db, newBackupProtection(db, secretsManager, client, backupRunner, logger), alertingEngine)
		scheduler.BaseBackups = newBaseBackupRunner(db, secretsManager, client, backupRunner, logger)
		go func() {
			if err := scheduler.Run(ctx, backupSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("backup scheduler stopped", slog.String("error", err.Error()))
			}
		}()
		walShip := newWALShipScheduler(db, client, backupRunner, func(nodeID string) bool {
			return nodeID == "" || (meshCfg != nil && nodeID == meshCfg.localNodeID)
		}, logger)
		apiRouter.SetWALShipStatus(walShip)
		go func() {
			if err := walShip.Run(ctx, walShipInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("wal ship scheduler stopped", slog.String("error", err.Error()))
			}
		}()
	}

	// Always runs: scheduledTaskRunner needs no master key, so this loop is never gated.
	scheduledTaskScheduler := scheduledtask.NewScheduler(db, scheduledTaskRunner, logger)
	scheduledTaskScheduler.Lookback = scheduledTaskRunner.Limits.CatchUpLookback
	go func() {
		if err := scheduledTaskScheduler.Run(ctx, scheduledTaskSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduled task scheduler stopped", slog.String("error", err.Error()))
		}
	}()

	// Fires through the same git-source deploy path a webhook push uses.
	appScheduleScheduler := scheduledeploy.NewScheduler(db, db, apiRouter, logger)
	go func() {
		if err := appScheduleScheduler.Run(ctx, appScheduleSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("scheduled deploy scheduler stopped", slog.String("error", err.Error()))
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

	startSecretRotation(ctx, logger, db, apiRouter, secretsManager != nil)

	startDatabaseUpgrades(ctx, logger, db, secretsManager, client, engine.Nudge, backupRunner, backupVerifyRunner, alertingDB, deployDispatcher,
		func(nodeID string) bool { return nodeID == "" || (meshCfg != nil && nodeID == meshCfg.localNodeID) }, apiRouter)

	imageUpdateScheduler := &imageupdate.Scheduler{Store: db, Checker: apiRouter, Logger: logger}
	go func() {
		if err := imageUpdateScheduler.Run(ctx, imageupdate.IntervalFromEnv(logger)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("image update scheduler stopped", slog.String("error", err.Error()))
		}
	}()

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

	events, errs := client.Events(ctx)
	go func() {
		if err, ok := <-errs; ok && err != nil {
			logger.Error("docker event stream error", slog.String("error", err.Error()))
		}
	}()

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

	engineErr := engine.Run(ctx, events, resyncInterval)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), httpShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("http server shutdown failed", slog.String("error", err.Error()))
	}
	if serveErr := <-serveErrs; serveErr != nil {
		logger.Error("http server error", slog.String("error", serveErr.Error()))
	}

	if errors.Is(engineErr, context.Canceled) {
		if restartRequested.Load() {
			return errRestartRequested
		}
		return nil // Ctrl+C / SIGTERM is a clean shutdown, not a failure
	}
	return engineErr
}

// gracefulStopper is the subset of *grpc.Server stopAgentGRPCServer
// needs, narrowed so a test can exercise the timeout/fallback path
// without a real listener and a real stuck stream.
type gracefulStopper interface {
	GracefulStop()
	Stop()
}

// Falls back to hard Stop so a live agent Session stream cannot stall process exit.
func stopAgentGRPCServer(server gracefulStopper, logger *slog.Logger, timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		server.GracefulStop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logger.Warn("agent grpc server did not drain in time, forcing stop",
			slog.Duration("timeout", timeout))
		server.Stop()
		<-done
	}
}

func openStore(ctx context.Context) (*store.DB, error) {
	return openStoreObserved(ctx, nil)
}

// openStoreObserved is openStore that reports pre-migration facts into obs.
func openStoreObserved(ctx context.Context, obs *upgradehistory.Observed) (*store.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	// dataDir is operator-supplied at process startup (env var or the
	// fixed default), not attacker-controlled request input, so gosec's
	// path-traversal concern doesn't apply here.
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return store.Open(ctx, filepath.Join(dataDir, storeFilename), store.WithPreMigrateHook(observedPreMigrateSnapshotHook(dataDir, obs)))
}

// Separate SQLite file so its collection-tick writes never contend with levelrail.db's WAL.
func openTelemetryStore(ctx context.Context) (*telemetry.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return telemetry.Open(ctx, filepath.Join(dataDir, "telemetry.db"))
}

// Separate SQLite file for the same write-isolation reason as telemetry.db.
func openAlertingStore(ctx context.Context) (*alerting.DB, error) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	return alerting.Open(ctx, filepath.Join(dataDir, "alerting.db"))
}

// Env-only SMTP fallback; nil is a valid result meaning not configured.
func smtpConfigFromEnv() *email.SMTPConfig {
	host := os.Getenv("APP_SMTP_HOST")
	from := os.Getenv("APP_SMTP_FROM")
	if host == "" || from == "" {
		return nil
	}
	port := os.Getenv("APP_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return &email.SMTPConfig{
		Addr:     host + ":" + port,
		Host:     host,
		Username: os.Getenv("APP_SMTP_USERNAME"),
		Password: os.Getenv("APP_SMTP_PASSWORD"),
		From:     from,
	}
}

// emailConfigLoader resolves email_settings fresh on every send,
// decrypting its stored credential through secretsManager, falling back
// to envFallback when the row's backend is unset.
func emailConfigLoader(db *store.DB, secretsManager *secrets.Manager, envFallback *email.SMTPConfig) email.ConfigLoader {
	return func(ctx context.Context) (email.Config, error) {
		settings, err := db.GetEmailSettings(ctx)
		if err != nil {
			return email.Config{}, err
		}

		switch settings.Backend {
		case store.EmailBackendSMTP:
			if secretsManager == nil {
				return email.Config{}, fmt.Errorf("email settings: smtp backend selected but no master key is configured")
			}
			password, err := resolveEmailSecret(ctx, secretsManager, "smtp_password")
			if err != nil {
				return email.Config{}, err
			}
			return email.Config{Backend: email.BackendSMTP, SMTP: &email.SMTPConfig{
				Addr:     fmt.Sprintf("%s:%d", settings.SMTPHost, settings.SMTPPort),
				Host:     settings.SMTPHost,
				Username: settings.SMTPUsername,
				Password: password,
				From:     settings.SMTPFrom,
			}}, nil
		case store.EmailBackendSES:
			if secretsManager == nil {
				return email.Config{}, fmt.Errorf("email settings: ses backend selected but no master key is configured")
			}
			secret, err := resolveEmailSecret(ctx, secretsManager, "ses_secret_access_key")
			if err != nil {
				return email.Config{}, err
			}
			return email.Config{Backend: email.BackendSES, SES: &email.SESConfig{
				Region:          settings.SESRegion,
				AccessKeyID:     settings.SESAccessKeyID,
				SecretAccessKey: secret,
				From:            settings.SESFrom,
			}}, nil
		case store.EmailBackendResend:
			if secretsManager == nil {
				return email.Config{}, fmt.Errorf("email settings: resend backend selected but no master key is configured")
			}
			apiKey, err := resolveEmailSecret(ctx, secretsManager, "resend_api_key")
			if err != nil {
				return email.Config{}, err
			}
			return email.Config{Backend: email.BackendResend, Resend: &email.ResendConfig{
				APIKey: apiKey,
				From:   settings.ResendFrom,
			}}, nil
		default:
			if envFallback == nil {
				return email.Config{}, nil // NewSender turns this into email.ErrNotConfigured
			}
			return email.Config{Backend: email.BackendSMTP, SMTP: envFallback}, nil
		}
	}
}

// resolveEmailSecret treats "never set" as an empty string rather than
// an error: a backend can be selected before its credential is saved.
func resolveEmailSecret(ctx context.Context, secretsManager *secrets.Manager, envKey string) (string, error) {
	value, err := secretsManager.Resolve(ctx, store.EmailSettingsSecretsKey(), envKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

// Persisted beside the other SQLite files; unlike the server certificate, the CA survives restarts.
const (
	agentCACertFilename = "agent-ca.crt.pem"
	agentCAKeyFilename  = "agent-ca.key.pem"

	// masterKeyFilename is the persisted secrets.MasterKey fallback used
	// when APP_MASTER_KEY isn't set, same data directory as the agent CA
	// above.
	masterKeyFilename = "master.key"

	// envMasterKeyFile points at a key file outside the data directory, so a
	// copy of the data directory alone cannot decrypt stored secrets.
	envMasterKeyFile = "APP_MASTER_KEY_FILE"
)

// A regenerated CA would invalidate every enrolled agent's certificate.
func loadOrGenerateAgentCA(dataDir string) (*agent.CA, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, err
	}
	certPath := filepath.Join(dataDir, agentCACertFilename)
	keyPath := filepath.Join(dataDir, agentCAKeyFilename)

	certPEM, certErr := os.ReadFile(certPath) //nolint:gosec // operator-controlled data directory path, not user input
	keyPEM, keyErr := os.ReadFile(keyPath)    //nolint:gosec // same
	if certErr == nil && keyErr == nil {
		return agent.LoadCA(certPEM, keyPEM)
	}

	ca, err := agent.GenerateCA()
	if err != nil {
		return nil, fmt.Errorf("generate agent CA: %w", err)
	}
	if err := os.WriteFile(certPath, ca.CertPEM(), 0o644); err != nil { //nolint:gosec // certificate is not secret
		return nil, fmt.Errorf("persist agent CA certificate: %w", err)
	}
	if err := os.WriteFile(keyPath, ca.KeyPEM(), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return nil, fmt.Errorf("persist agent CA private key: %w", err)
	}
	return ca, nil
}

// agentAddr reads APP_AGENT_ADDR, defaulting to defaultAgentAddr.
func agentAddr() string {
	addr := os.Getenv("APP_AGENT_ADDR")
	if addr == "" {
		addr = defaultAgentAddr
	}
	return addr
}

// Must match the gRPC server's TLS certificate or every agent's verification fails.
// Defaults to 127.0.0.1, correct only for single-machine testing.
func agentAdvertiseHost() string {
	host := os.Getenv("APP_AGENT_ADVERTISE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

// Re-derived on every call from the store and runtime, so it never serves stale targets.
func telemetryTargets(db *store.DB, local docker.Runtime, registry *agent.Registry, meshCfg *meshSetup, logger *slog.Logger) func(context.Context) ([]telemetry.Target, error) {
	deps := dynamicSourceDeps{meshCfg: meshCfg}
	return func(ctx context.Context) ([]telemetry.Target, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}

		var targets []telemetry.Target
		for _, svc := range services {
			nodeID := runtimeNodeID(deps, svc.NodeID)
			nodeRuntime, err := resolveNodeTransport(local, registry, nodeID)
			if err != nil {
				// Skipping a disconnected node's services is deliberate: metrics resume on reconnect.
				logger.Warn("telemetry: service's node unreachable, skipping this tick",
					slog.String("service", svc.Name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
				continue
			}
			containers, err := nodeRuntime.ListByPrefix(ctx, svc.Name+"-")
			if err != nil {
				logger.Warn("telemetry: list containers failed, skipping this tick",
					slog.String("service", svc.Name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
				continue
			}
			for _, c := range containers {
				if !c.Running {
					continue
				}
				targets = append(targets, telemetry.Target{
					ResourceID:  "service:" + svc.Name,
					ContainerID: c.ID,
					NodeID:      nodeID,
				})
			}
		}

		dbTargets, err := databaseTelemetryTargets(ctx, db, local, registry, meshCfg, logger)
		if err != nil {
			return nil, err
		}
		targets = append(targets, dbTargets...)
		return targets, nil
	}
}

// Exact container lookup: a database's name has no image-hash suffix, so prefix matching would also hit db-foobar.
func databaseTelemetryTargets(ctx context.Context, db *store.DB, local docker.Runtime, registry *agent.Registry, meshCfg *meshSetup, logger *slog.Logger) ([]telemetry.Target, error) {
	deps := dynamicSourceDeps{meshCfg: meshCfg}
	databases, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired databases: %w", err)
	}

	var targets []telemetry.Target
	for _, d := range databases {
		nodeID := runtimeNodeID(deps, d.NodeID)
		nodeRuntime, err := resolveNodeTransport(local, registry, nodeID)
		if err != nil {
			logger.Warn("telemetry: database's node unreachable, skipping this tick",
				slog.String("database", d.Name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
			continue
		}
		cs, err := nodeRuntime.InspectByName(ctx, "db-"+d.Name)
		if err != nil {
			logger.Warn("telemetry: inspect database container failed, skipping this tick",
				slog.String("database", d.Name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
			continue
		}
		if cs == nil || !cs.Running {
			continue
		}
		targets = append(targets, telemetry.Target{
			ResourceID:  "database:" + d.Name,
			ContainerID: cs.ID,
			NodeID:      nodeID,
		})
	}
	return targets, nil
}

// Samples land centrally rather than per node, a narrower scope than the design's ideal.
type multiNodeStatsSource struct {
	local    docker.Runtime
	registry *agent.Registry
}

func (s *multiNodeStatsSource) Stats(ctx context.Context, nodeID, containerID string) (docker.ContainerStats, error) {
	nodeRuntime, err := resolveNodeTransport(s.local, s.registry, nodeID)
	if err != nil {
		return docker.ContainerStats{}, err
	}
	inspector, ok := nodeRuntime.(docker.StatsInspector)
	if !ok {
		return docker.ContainerStats{}, fmt.Errorf("telemetry: node %q's runtime cannot report container stats", nodeID)
	}
	return inspector.Stats(ctx, containerID)
}

// Deliberately duplicates telemetryTargets: streaming and polling collectors use distinct target types.
func logTargets(db *store.DB, runtime docker.Runtime) func(context.Context) ([]telemetry.LogTarget, error) {
	return func(ctx context.Context) ([]telemetry.LogTarget, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}

		var targets []telemetry.LogTarget
		for _, svc := range services {
			containers, err := runtime.ListByPrefix(ctx, svc.Name+"-")
			if err != nil {
				return nil, fmt.Errorf("list containers for %s: %w", svc.Name, err)
			}
			for _, c := range containers {
				targets = append(targets, telemetry.LogTarget{
					ResourceID:  "service:" + svc.Name,
					ContainerID: c.ID,
					Stopped:     !c.Running,
				})
			}
		}

		dbTargets, err := databaseLogTargets(ctx, db, runtime)
		if err != nil {
			return nil, err
		}
		targets = append(targets, dbTargets...)
		return targets, nil
	}
}

// databaseLogTargets is logTargets' database-kind counterpart, the same
// InspectByName-not-ListByPrefix reasoning databaseTelemetryTargets'
// own doc comment explains.
func databaseLogTargets(ctx context.Context, db *store.DB, runtime docker.Runtime) ([]telemetry.LogTarget, error) {
	databases, err := db.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list desired databases: %w", err)
	}

	var targets []telemetry.LogTarget
	for _, d := range databases {
		cs, err := runtime.InspectByName(ctx, "db-"+d.Name)
		if err != nil {
			return nil, fmt.Errorf("inspect container for database %s: %w", d.Name, err)
		}
		if cs == nil {
			continue
		}
		targets = append(targets, telemetry.LogTarget{
			ResourceID:  "database:" + d.Name,
			ContainerID: cs.ID,
			Stopped:     !cs.Running,
		})
	}
	return targets, nil
}

// Only enabled drains are returned; DrainForwarder spawns a goroutine only for these.
func drainTargets(db *store.DB) func(context.Context) ([]telemetry.DrainConfig, error) {
	return func(ctx context.Context) ([]telemetry.DrainConfig, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}

		var configs []telemetry.DrainConfig
		for _, svc := range services {
			if svc.LogDrain == nil || !svc.LogDrain.Enabled {
				continue
			}
			configs = append(configs, telemetry.DrainConfig{
				ResourceID: "service:" + svc.Name,
				Type:       telemetry.DrainSinkType(svc.LogDrain.Type),
				Target:     svc.LogDrain.Target,
				Enabled:    true,
			})
		}
		return configs, nil
	}
}

// Mirrors runMetricsRetentionSweep; retention is env-configurable, the interval is not.
func runLogsRetentionSweep(ctx context.Context, db *telemetry.DB, logger *slog.Logger) {
	ticker := time.NewTicker(logsRetentionSweepInterval)
	defer ticker.Stop()

	sweep := func() {
		cutoff := time.Now().Add(-logsRetention())
		deleted, err := db.RetainLogs(ctx, cutoff)
		if err != nil {
			logger.Error("logs retention sweep failed", slog.String("error", err.Error()))
			return
		}
		if deleted > 0 {
			logger.Info("logs retention sweep", slog.Int64("deleted", deleted), slog.Time("cutoff", cutoff))
		}
		if trims, berr := db.EnforceLogBudgets(ctx, telemetry.DefaultLogBudgetBytes()); berr != nil {
			logger.Error("log budget sweep failed", slog.String("error", berr.Error()))
		} else {
			for _, t := range trims {
				logger.Info("log budget trim", slog.String("resource", t.ResourceID), slog.Int64("lines", t.Lines), slog.Int64("bytes", t.Bytes))
			}
		}
		// Traces age out on the same window as logs.
		if n, serr := db.RetainSpans(ctx, cutoff); serr != nil {
			logger.Error("trace retention sweep failed", slog.String("error", serr.Error()))
		} else if n > 0 {
			logger.Info("trace retention sweep", slog.Int64("deleted", n), slog.Time("cutoff", cutoff))
		}
		if n, eerr := db.RetainErrors(ctx, cutoff); eerr != nil {
			logger.Error("error tracking retention sweep failed", slog.String("error", eerr.Error()))
		} else if n > 0 {
			logger.Info("error tracking retention sweep", slog.Int64("deleted", n), slog.Time("cutoff", cutoff))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func logsRetention() time.Duration {
	raw := os.Getenv("APP_LOGS_RETENTION")
	if raw == "" {
		return defaultLogsRetention
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultLogsRetention
	}
	return d
}

// Shared with cmd/levelrail-agent through the same env var, so one setting governs both cadences by default.
func nodeHeartbeatInterval() time.Duration {
	raw := os.Getenv("APP_NODE_HEARTBEAT_INTERVAL")
	if raw == "" {
		return defaultNodeHeartbeatInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeHeartbeatInterval
	}
	return d
}

// nodeHeartbeatTimeout reads APP_NODE_HEARTBEAT_TIMEOUT,
// how long since last_seen_at internal/reconcile/nodehealth treats as
// stale.
func nodeHeartbeatTimeout() time.Duration {
	raw := os.Getenv("APP_NODE_HEARTBEAT_TIMEOUT")
	if raw == "" {
		return defaultNodeHeartbeatTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeHeartbeatTimeout
	}
	return d
}

// nodeKeepaliveTime reads APP_NODE_KEEPALIVE_TIME, how often the agent
// gRPC server sends an HTTP/2 PING on an otherwise-idle connection.
func nodeKeepaliveTime() time.Duration {
	raw := os.Getenv("APP_NODE_KEEPALIVE_TIME")
	if raw == "" {
		return defaultNodeKeepaliveTime
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeKeepaliveTime
	}
	return d
}

// nodeKeepaliveTimeout reads APP_NODE_KEEPALIVE_TIMEOUT, how long the
// agent gRPC server waits for a PING ack before considering the
// connection dead.
func nodeKeepaliveTimeout() time.Duration {
	raw := os.Getenv("APP_NODE_KEEPALIVE_TIMEOUT")
	if raw == "" {
		return defaultNodeKeepaliveTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultNodeKeepaliveTimeout
	}
	return d
}

// osPatchCheckInterval reads APP_OS_PATCH_CHECK_INTERVAL, the same
// env-var-with-default shape as nodeHeartbeatInterval above.
func osPatchCheckInterval() time.Duration {
	raw := os.Getenv("APP_OS_PATCH_CHECK_INTERVAL")
	if raw == "" {
		return defaultOSPatchCheckInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultOSPatchCheckInterval
	}
	return d
}

// Turns a missing brand.yaml into an actionable message, since it would otherwise read as an opaque crash.
// brand.Load itself stays strict about its input.
func loadBrand() (*brand.Brand, error) {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = defaultBrandFile
	}
	b, err := brand.Load(path)
	if err != nil && os.IsNotExist(errors.Unwrap(err)) {
		return nil, fmt.Errorf("%w (running via install.sh sets this up automatically; running the binary directly needs either a brand.yaml file at %q or APP_BRAND_FILE pointing at one)", err, path)
	}
	return b, err
}

// A missing CHANGELOG.md is not fatal: the what's-new panel is simply empty.
func loadChangelog(logger *slog.Logger) []changelog.Entry {
	path := os.Getenv("APP_CHANGELOG_FILE")
	content, found, err := changelog.ReadFile(path, defaultChangelogFile)
	if err != nil {
		logger.Warn("read changelog file failed", slog.String("error", err.Error()))
		return nil
	}
	if !found {
		return nil
	}
	return changelog.Parse(content)
}

func loadGitHubAppManifestConfig() (githubapp.ManifestConfig, error) {
	path := os.Getenv("APP_GITHUB_APP_MANIFEST_FILE")
	if path == "" {
		path = defaultGitHubAppManifestFile
	}
	return githubapp.LoadManifestConfig(path)
}

// devFixturesFile resolves the path MaybeSeedDevFixturesFromFile reads,
// the same default-plus-env-override shape as loadBrand above.
func devFixturesFile() string {
	path := os.Getenv("APP_DEV_FIXTURES_FILE")
	if path == "" {
		path = defaultDevFixturesFile
	}
	return path
}

// loadSecretsManager builds a secrets.Manager from APP_MASTER_KEY, or a
// key persisted in (or first generated into) dataDir. masterKeyFilePath
// is "" for an env-sourced key, so rotation knows it cannot persist it.
func loadSecretsManager(db *store.DB, dataDir string, opts ...secrets.ManagerOption) (mgr *secrets.Manager, masterKeyFilePath string, err error) {
	if serialized := os.Getenv("APP_MASTER_KEY"); serialized != "" {
		plain, err := openMasterKeyMaterial(context.Background(), serialized, os.Getenv)
		if err != nil {
			return nil, "", fmt.Errorf("open master key from APP_MASTER_KEY: %w", err)
		}
		mk, err := secrets.LoadMasterKey(plain)
		if err != nil {
			return nil, "", fmt.Errorf("load master key from APP_MASTER_KEY: %w", err)
		}
		return secrets.NewManager(db, mk, opts...), "", nil
	}

	mk, keyPath, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		return nil, "", err
	}
	return secrets.NewManager(db, mk, opts...), keyPath, nil
}

// loadOrGenerateMasterKey loads a previously persisted master key from
// dataDir, or generates and persists a fresh one if none exists yet.
func loadOrGenerateMasterKey(dataDir string) (mk *secrets.MasterKey, keyPath string, err error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, "", fmt.Errorf("create data dir for master key: %w", err)
	}
	keyPath = filepath.Join(dataDir, masterKeyFilename)
	if custom := strings.TrimSpace(os.Getenv(envMasterKeyFile)); custom != "" {
		if !filepath.IsAbs(custom) {
			return nil, "", fmt.Errorf("%s must be an absolute path, got %q", envMasterKeyFile, custom)
		}
		keyPath = custom
		if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil { //nolint:gosec // operator-set absolute path from startup config
			return nil, "", fmt.Errorf("create directory for %s: %w", envMasterKeyFile, err)
		}
		if st, err := os.Stat(keyPath); err == nil && st.Mode().Perm()&0o077 != 0 { //nolint:gosec // operator-set absolute path from startup config
			return nil, "", fmt.Errorf("%s %s has mode %o, want 0600: run chmod 600 on it", envMasterKeyFile, keyPath, st.Mode().Perm())
		}
	}

	serialized, readErr := os.ReadFile(keyPath) //nolint:gosec // operator-controlled data directory path, not user input
	switch {
	case readErr == nil:
		plain, err := openMasterKeyMaterial(context.Background(), string(serialized), os.Getenv)
		if err != nil {
			return nil, "", fmt.Errorf("open persisted master key: %w", err)
		}
		mk, err := secrets.LoadMasterKey(plain)
		if err != nil {
			return nil, "", fmt.Errorf("load persisted master key: %w", err)
		}
		return mk, keyPath, nil
	case !os.IsNotExist(readErr):
		// Never treat an I/O failure as 'no key yet': regenerating would orphan every secret already wrapped.
		return nil, "", fmt.Errorf("read persisted master key at %s: %w", keyPath, readErr)
	}

	mk, err = secrets.GenerateMasterKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate master key: %w", err)
	}
	sealed, err := sealMasterKeyMaterial(context.Background(), mk.String(), os.Getenv)
	if err != nil {
		return nil, "", fmt.Errorf("wrap new master key: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(sealed), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return nil, "", fmt.Errorf("persist master key: %w", err)
	}
	return mk, keyPath, nil
}

// Shared by the git webhook and the manual build trigger, so both use one BuildKit connection.
// Returns an error rather than a partially wired builder; the caller treats it as non-fatal.
func loadBuilder(ctx context.Context, logger *slog.Logger, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, agentRegistry *agent.Registry, supplyChain *supplychain.Service, guard dockerguard.Booted) (*deploy.Pipeline, func() error, error) {
	rawDockerCli, err := rawDockerClient(guard.Host)
	if err != nil {
		return nil, nil, fmt.Errorf("new docker client for buildkit: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawDockerCli, buildCacheOptions(ctx, db, logger)...)
	cancel()
	if err != nil {
		_ = rawDockerCli.Close()
		return nil, nil, fmt.Errorf("connect to buildkit: %w", err)
	}

	closer := func() error {
		buildErr := buildClient.Close()
		dockerErr := rawDockerCli.Close()
		if buildErr != nil {
			return buildErr
		}
		return dockerErr
	}

	// Static sites are copied here, not served from the checkout, which the webhook removes on return.
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	staticSitesDir := filepath.Join(dataDir, "static")

	deployOpts := []deploy.Option{
		deploy.WithBuildMetricsRecorder(telemetryDB),
		deploy.WithLogger(logger),
		deploy.WithStaticSiteStore(db),
		deploy.WithStaticRootDir(staticSitesDir),
		deploy.WithStaticPublishRecorder(db),
		deploy.WithAppStore(db),
		deploy.WithVaultConfigChecker(db),
		deploy.WithLoadBalancerStore(db),
		deploy.WithSpendCapStore(db),
		deploy.WithAutoscaleStore(db, autoscale.CeilingFromEnv(autoscale.OSEnv)),
		deploy.WithCanaryPolicyStore(db),
		deploy.WithJobStore(db),
		deploy.WithOrderedStore(db),
		deploy.WithAttemptRecorder(db),
	}
	deployOpts = append(deployOpts, digestResolverOptions(logger, db, secretsManager, guard)...)
	deployOpts = append(deployOpts, supplyChainDeployOptions(supplyChain)...)
	deployOpts = append(deployOpts, secretScanDeployOptions(db, logger)...)
	if secretsManager != nil {
		deployOpts = append(deployOpts, deploy.WithSecretChecker(secretsManager), deploy.WithBuildCache(newBuildCache(logger, db, secretsManager)),
			deploy.WithBuildSecretSource(&pkgregistry.Resolver{Store: db, Secrets: secretsManager}))
	}

	router := build.NewRouter(buildClient, buildNodeSource(db, agentRegistry), agent.NewBuildDispatcher(agentRegistry), build.WithRouterLogger(logger))
	limiter := build.NewLimiter(router, api.BuildLimitsSource(db, build.LimitsFromEnv(os.LookupEnv)),
		build.WithRunRecorder(api.BuildRunRecorder(db)), build.WithLimiterLogger(logger))
	return deploy.New(limiter, db, deployOpts...), closer, nil
}

// Requires every input, so any missing one returns an error rather than a partially wired handler.
func loadWebhookHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, recorder *deploylog.Recorder, notifier *alerting.DeployDispatcher, pipeline *deploy.Pipeline, nudger *reconcile.Engine) (http.Handler, error) {
	if pipeline == nil {
		return nil, fmt.Errorf("no builder available (see the earlier \"builder not configured\" warning)")
	}

	repoURL := os.Getenv("APP_GIT_REPO_URL")
	webhookSecret := os.Getenv("APP_WEBHOOK_SECRET")
	imageRepo := os.Getenv("APP_IMAGE_REPO")
	if repoURL == "" || webhookSecret == "" || imageRepo == "" {
		return nil, fmt.Errorf("APP_GIT_REPO_URL, APP_WEBHOOK_SECRET, and APP_IMAGE_REPO must all be set")
	}

	specDir := os.Getenv("APP_SPEC_DIR")
	if specDir == "" {
		specDir = "."
	}
	specPath, err := spec.DiscoverPath(specDir, strings.ToLower(b.BinaryName))
	if err != nil {
		return nil, fmt.Errorf("discover app spec: %w", err)
	}
	// Operator-controlled path, not request input, so the gosec file-path warning does not apply.
	data, err := os.ReadFile(specPath) //nolint:gosec // operator-controlled startup config, not user input
	if err != nil {
		return nil, fmt.Errorf("read app spec %s: %w", specPath, err)
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parse app spec %s: %w", specPath, err)
	}

	serviceName := os.Getenv("APP_SERVICE_NAME")
	if serviceName == "" {
		if len(parsed.Services) != 1 {
			return nil, fmt.Errorf("APP_SERVICE_NAME must be set: app spec %s declares %d services, not exactly 1", specPath, len(parsed.Services))
		}
		for name := range parsed.Services {
			serviceName = name
		}
	}
	svc, ok := parsed.Services[serviceName]
	if !ok {
		return nil, fmt.Errorf("service %q not found in app spec %s", serviceName, specPath)
	}

	cfg := webhook.Config{
		Secret:      []byte(webhookSecret),
		RepoURL:     repoURL,
		Branch:      os.Getenv("APP_GIT_BRANCH"),
		ServiceName: serviceName,
		Service:     svc,
		ImageRepo:   imageRepo,
	}
	return webhook.New(cfg, pipeline, db, recorder, notifier, nudger, logger), nil
}

// The built-in registry is used as the cache automatically unless APP_BUILD_CACHE_REGISTRY is set.
// Changing either takes a restart, since these are read once at startup.
func buildCacheOptions(ctx context.Context, db *store.DB, logger *slog.Logger) []build.Option {
	var opts []build.Option
	if dir := os.Getenv("APP_BUILD_CACHE_DIR"); dir != "" {
		opts = append(opts, build.WithCacheDir(dir))
	}
	if ref := os.Getenv("APP_BUILD_CACHE_REGISTRY"); ref != "" {
		opts = append(opts, build.WithCacheRegistry(ref))
		if os.Getenv("APP_BUILD_CACHE_REGISTRY_INSECURE") == "true" {
			opts = append(opts, build.WithCacheRegistryInsecure())
		}
		return opts
	}

	settings, err := db.GetRegistrySettings(ctx)
	if err != nil {
		logger.Warn("build cache: get registry settings failed, continuing without a cache backend", slog.String("error", err.Error()))
		return opts
	}
	if settings.Enabled && settings.Host != "" {
		opts = append(opts, build.WithCacheRegistry(settings.Host+"/buildcache"), build.WithCacheRegistryInsecure())
	}
	return opts
}

// Fixed loopback port: registryreconcile.HostPort is fixed, so there is nothing to inspect.
func registryDialAddr() string {
	return fmt.Sprintf("127.0.0.1:%d", registryreconcile.HostPort)
}

// Checked per build, so marking a node build-capable takes effect without a restart.
func buildNodeSource(db *store.DB, agentRegistry *agent.Registry) build.NodeSource {
	return func(ctx context.Context) ([]build.NodeInfo, error) {
		nodes, err := db.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}

		infos := make([]build.NodeInfo, 0, len(nodes))
		for _, n := range nodes {
			online := false
			if _, err := agentRegistry.Get(n.ID); err == nil {
				online = true
			}
			infos = append(infos, build.NodeInfo{
				ID:                    n.ID,
				AcceptsBuildWorkloads: n.AcceptsBuildWorkloads,
				Online:                online,
			})
		}
		return infos, nil
	}
}

// rootHandler mounts the API under /api/ and the embedded frontend everywhere else, on one server.
// Optional dependencies (secrets, webhook, builder) are applied only when non-nil, to avoid nil-interface panics.
func rootHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, alertingDB *alerting.DB, secretsManager *secrets.Manager, masterKeyFilePath string, webhookHandler http.Handler, client *docker.Client, builder *deploy.Pipeline, deployRecorder *deploylog.Recorder, logBroadcaster *telemetry.LogBroadcaster, deployDispatcher *alerting.DeployDispatcher, backupRunner *backup.Runner, backupVerifyRunner *backup.VerifyRunner, agentRegistry *agent.Registry, agentCAFingerprint string, emailSender email.Sender, scheduledTaskRunner *scheduledtask.Runner, engine *reconcile.Engine, ingressDriver *ingressdriver.Driver, pushVAPIDPublicKey string) (http.Handler, *api.Router) {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	trustRoot := selfupgrade.NewTrustRoot(selfupgrade.TUFCacheDir(dataDir), logger)
	opts := []api.Option{
		api.WithReconcileNudger(engine),
		api.WithStaticRootDir(filepath.Join(dataDir, "static")),
		api.WithDeployGate(db, deployGateDefaults()),
		api.WithReadinessProbes(api.ReadinessProbes{Database: db.PingContext, Migrations: db.MigrationsCurrent, EngineStarted: engine.Started}),
		api.WithTelemetryQuerier(telemetry.NewLocalFederator(telemetryDB)),
		api.WithTraceStore(telemetryDB),
		api.WithErrorStore(telemetryDB),
		api.WithLogPipelineStore(telemetryDB),
		api.WithLogBudgetStore(telemetryDB),
		api.WithAccessSource(api.EdgeAccess{Stats: ingressdriver.DefaultRequestStats(), Log: ingressdriver.DefaultAccessLog()}),
		api.WithRequestSummaryWindow(requestSummaryWindow()),
		api.WithAlertRules(alertingDB),
		api.WithAlertNoise(alertingDB),
		api.WithStatusPage(db, statusPageConfig(logger), statusPageRateLimit(logger)),
		api.WithDeployNotifyTargets(alertingDB),
		api.WithDeployNotifier(deployDispatcher),
		api.WithNotificationChannels(alertingDB),
		upgradeSchedulingOption(db, deployDispatcher),
		api.WithNotificationChannelTester(deployDispatcher),
		api.WithNotificationDeliveries(alertingDB),
		api.WithApprovalChatNotifier(deployDispatcher),
		api.WithDeviceLoginNotifier(deployDispatcher),
		api.WithDashboardURL(dashboardBaseURL()),
		api.WithFirewallRequiredPorts(platformRequiredPorts()),
		api.WithExposure(newExposureManager(b.RuleCommentPrefix()), db),
		api.WithDatabaseAccess(db, dbaccess.TTLLimitsFromEnv(os.Getenv)),
		api.WithSessionTTL(sessionTTL(logger)),
		api.WithRequestLogThresholds(slowRequestThreshold(logger), criticalRequestThreshold(logger)),
		api.WithDashboardCache(envDurationOr(logger, "APP_DASHBOARD_CACHE_TTL", 5*time.Second), envDurationOr(logger, "APP_DASHBOARD_CACHE_STALE", time.Minute)),
		api.WithAutoPlacement(autoPlacementEnabled(logger)),
		api.WithPlacementPolicy(placementPolicyFromEnv(logger)),
		api.WithHSTS(hstsEnabled(logger)),
		api.WithAllowInsecureLogin(allowInsecureLogin(logger)),
		api.WithNewDeviceApproval(api.NewDeviceApprovalFromEnv()),
		api.WithAPIRateLimit(apiRateLimitReadRPM(logger), apiRateLimitWriteRPM(logger)),
		api.WithWebhookRateLimit(webhookRateLimitRPM(logger)),
		api.WithTokenRedeemRateLimit(apiRateLimitRPM(logger, "APP_API_RATE_LIMIT_TOKEN_REDEEM_RPM", defaultTokenRedeemRateLimitRPM)),
		api.WithDataDir(dataDir),
		api.WithTrustRoot(trustRoot),
		api.WithSourceCache(sourcecache.New(sourcecache.ConfigFromEnv(filepath.Join(dataDir, "source-cache")), logger)),
		api.WithControlPlaneBackups(cpbackup.NewManager(db, dataDir)),
		multiNodeOption(db),
		api.WithNodeJoinTokenTTL(nodeJoinTokenTTL(logger)),
		api.WithControlPlaneBackupScheduleDisabled(controlPlaneBackupInterval(logger) == 0),
		api.WithDockerPinger(client),
		api.WithImageLister(client),
		api.WithContainerLister(client),
		api.WithDockerDiskUsager(client),
		api.WithDockerPruner(client),
		api.WithOrphanedVolumeManager(client),
		api.WithOrphanedContainerManager(client),
		api.WithRegistryAuthTester(client),
		api.WithDBPinger(db),
		api.WithAgentCAFingerprint(agentCAFingerprint),
		api.WithSecretRotationWarnAge(secretRotationWarnAge(logger)),
		api.WithDoctorDiskWarningBytes(doctorDiskWarningBytes(logger)),
		api.WithIngressPortOwner(ingressDriver),
		api.WithDoctorIngressPorts(ingressPortFromAddr(ingressHTTPAddr()), ingressPortFromAddr(ingressHTTPSAddr())),
		api.WithDashboardListenAddr(httpAddr()),
		api.WithDoctorNetworkTimeout(doctorNetworkTimeout(logger)),
		api.WithDoctorPublicIPEndpoint(os.Getenv("APP_DOCTOR_PUBLIC_IP_ENDPOINT")),
		api.WithDoctorClockSkewWarnAge(doctorClockSkewWarnAge(logger)),
		api.WithDoctorMinRAMBytes(doctorMinRAMBytes(logger)),
		api.WithDoctorMinCPUCount(doctorMinCPUCount(logger)),
		api.WithDoctorAgentAdvertise(agentAdvertiseHost(), ingressPortFromAddr(agentAddr())),
		api.WithDoctorDiskIOWarnLatency(doctorDiskIOWarnLatency(logger)),
		api.WithExecRuntime(func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(client, agentRegistry, nodeID)
		}),
		imageInventoryOption(func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(client, agentRegistry, nodeID)
		}),
		imageVerifyOption(logger),
		// Applied unconditionally: scheduledTaskRunner is always non-nil.
		api.WithScheduledTaskRunner(scheduledTaskRunner),
		api.WithJobLimits(jobsPerAppCap(scheduledTaskRunner)),
		api.WithCertExpiryWarningWindow(certExpiryWarningWindow(logger)),
		api.WithCertRenewalTracking(alertingDB, certRenewalStalledThreshold(logger)),
		api.WithNodeAlertThresholds(
			patchStatusThreshold(logger), nodeDiskSpaceThreshold(logger),
			nodeCPUThreshold(logger), nodeMemoryThreshold(logger),
		),
		api.WithResourceRecommendationLookback(resourceRecommendationLookback(logger)),
		api.WithCapacityForecastLookback(capacityForecastLookback(logger)),
		api.WithPreviewTTL(previewTTL(logger)),
		api.WithPreviewLimits(previewLimits(logger)),
		api.WithPreviewStuckAfter(previewStuckAfter(logger)),
		api.WithStaticOrphanGrace(staticOrphanGrace(logger)),
		api.WithInviteTTL(inviteTTL(logger)),
		api.WithAuditLogRetention(auditLogRetention(logger)),
		api.WithWebhookDeliveryRetention(webhookDeliveryRetention(logger)),
		api.WithWebhookFailedDeliveryLimits(
			positiveIntEnv(logger, "APP_WEBHOOK_FAILED_DELIVERY_KEEP"),
			positiveIntEnv(logger, "APP_WEBHOOK_FAILED_BODY_BYTES"),
		),
		api.WithDeployApprovalTTL(deployApprovalTTL(logger)),
		api.WithPublicHost(publicHost()),
		api.WithPublicHostSource(publicHostSource()),
		api.WithStandaloneSafety(standaloneSafety()),
		api.WithHTTPSEnableLimits(apiRateLimitRPM(logger, "APP_ACME_MAX_ATTEMPTS_PER_HOUR", 0), acmePendingTimeout(logger)),
		api.WithACMEDefaultDirectory(acmeDefaultDirectory()),
		api.WithDeployLogQuerier(telemetryDB),
		api.WithDeployRecorder(deployRecorder),
		api.WithDeploySafety(db, client),
		api.WithDeployMaxConcurrent(deployMaxConcurrent(logger)),
		api.WithWakeToken(appWakeToken()),
		api.WithLogBroadcaster(logBroadcaster),
		// emailSender is always non-nil (run() builds it unconditionally):
		// forgot-password always exists, it just fails clearly at send
		// time when nothing is actually configured.
		api.WithEmailSender(emailSender),
		// Applied unconditionally, like WithEmailSender; the 501 covers the no-key case.
		api.WithPushVAPIDPublicKey(pushVAPIDPublicKey),
	}
	authOpts, err := authEngineOptions(context.Background(), logger, b, db.DB, secretsManager)
	if err != nil {
		logger.Error("auth engine setup failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	opts = append(opts, authOpts...)
	if secretsManager != nil {
		opts = append(opts, api.WithSecretSetter(secretsManager))
		opts = append(opts, api.WithMasterKeyRotation(secretsManager, masterKeyFilePath))
		if w, werr := keyWrapperFromEnv(os.Getenv); werr == nil && w != nil {
			opts = append(opts, api.WithMasterKeyWrapper(w))
		}
		opts = append(opts, api.WithSecretBinding(secretsManager))
		opts = append(opts, api.WithDoctorMasterKeyRotationWarnAge(doctorMasterKeyRotationWarnAge(logger)))
		// Gated on the master key: every secret write path requires one, so the count is meaningless without it.
		opts = append(opts, api.WithStaleSecretCounter(db))
		// Email settings' SMTP password / SES secret access key go
		// through the same secretsManager as everything else here.
		opts = append(opts, api.WithEmailSecrets(secretsManager))
		// Cloudflare Tunnel's token goes through the same secretsManager,
		// same nil-interface hazard as everything else in this block.
		opts = append(opts, api.WithCloudflareTunnelSecrets(secretsManager))
		// Cloudflare DNS-01's API token (a distinct credential from the
		// Tunnel connector token above) goes through the same
		// secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithCloudflareDNSSecrets(secretsManager))
		// Route53 DNS-01's access key pair (a second, independent ACME
		// DNS-01 provider from Cloudflare DNS-01 above) goes through the
		// same secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithRoute53DNSSecrets(secretsManager))
		// DNS records management reads back the same two credentials
		// above (it needs the plaintext value, not just presence), still
		// through secretsManager.
		opts = append(opts, api.WithCloudflareDNSTokenResolver(secretsManager))
		opts = append(opts, api.WithRoute53DNSCredentialResolver(secretsManager))
		// A cloud node provider's API token (Hetzner, DigitalOcean) goes
		// through the same secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithNodeProviderSecrets(secretsManager))
		// The built-in registry's generated password goes through the
		// same secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithRegistrySecrets(secretsManager))
		// The built-in registry catalog picker resolves the same
		// generated password server-side to authenticate its own upstream
		// query, same nil-interface hazard.
		opts = append(opts, api.WithRegistryCatalogSecrets(secretsManager))
		// The external Vault integration's own token/AppRole secret ID
		// goes through the same secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithVaultSecrets(secretsManager))
		// Per-domain HTTP Basic Auth passwords go through the same
		// secretsManager, same nil-interface hazard.
		opts = append(opts, api.WithDomainBasicAuthSecrets(secretsManager))
		// BYO TLS certificate uploads go through the same secretsManager,
		// same nil-interface hazard.
		opts = append(opts, api.WithDomainTLSCertSecrets(secretsManager))
		// Same master-key gate as the other secret-backed options.
		opts = append(opts, api.WithGitSourceSecrets(secretsManager))
		// Compose ingestion's generated template secrets (SERVICE_PASSWORD_*
		// etc, internal/compose.ResolveMagicVars): same secretsManager,
		// same nil-interface hazard as everything else in this block.
		opts = append(opts, api.WithComposeSecrets(secretsManager))
		// OAuth provider client secrets (Google/GitHub sign-in): same
		// secretsManager, same nil-interface hazard as everything else in
		// this block.
		opts = append(opts, api.WithOAuthSecrets(secretsManager))
		// Skipped without a master key: backup targets store and read credentials through secretsManager.
		restoreRunner := &backup.RestoreRunner{
			Store:          db,
			Secrets:        secretsManager,
			Downloader:     backup.S3Downloader{},
			Restorer:       &backup.ContainerRestorer{Runtime: client},
			VolumeRestorer: &backup.ContainerVolumeRestorer{Runtime: client},
			Identities:     backupIdentities(backupRunner),
		}
		// pitrRunner's Nudge is the engine, so a PITR wipe's suspend and unsuspend are picked up immediately.
		baseBackupRunner := newBaseBackupRunner(db, secretsManager, client, backupRunner, logger)
		pitrRunner := &backup.PITRRunner{
			Store:      db,
			Secrets:    secretsManager,
			Downloader: backup.S3Downloader{},
			Restorer:   &backup.ContainerPITRRestorer{Runtime: client},
			WAL:        &backup.WALFetcher{Runtime: client, Lister: backup.S3Lister{}, Downloader: backup.S3Downloader{}},
			WALVolume:  databaseWALArchiveVolume,
			Runtime:    client,
			Nudge:      engine.Nudge,
		}
		opts = append(opts,
			api.WithBackupSecrets(secretsManager),
			// Registry credentials (build.type: image's optional
			// registryCredential field): same secretsManager, same
			// nil-interface hazard as everything else in this block.
			api.WithRegistryCredentialSecrets(secretsManager),
			// Network shares (CIFS password only; NFS shares write no
			// secret): same secretsManager, same nil-interface hazard.
			api.WithNetworkShareSecrets(secretsManager),
			api.WithBackupRunner(backupRunner),
			// Volume backups reuse the database runner: one implementation serves both resource kinds.
			api.WithServiceVolumeBackupRunner(backupRunner),
			// Needs ExecWithInput on the local runtime, the same way ContainerDumper needs Exec.
			api.WithRestoreRunner(restoreRunner),
			api.WithServiceVolumeRestoreRunner(restoreRunner),
			// Waits for the new database to come up, unlike the restore it sits beside.
			api.WithCloneRestoreRunner(&backup.CloneRestoreRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: backup.S3Downloader{},
				Restorer:   &backup.ContainerRestorer{Runtime: client},
			}),
			// Creates the volume synchronously: a bare volume has no reconciler to wait on.
			api.WithVolumeCloneRestoreRunner(&backup.VolumeCloneRestoreRunner{
				Store:          db,
				Secrets:        secretsManager,
				Downloader:     backup.S3Downloader{},
				VolumeRestorer: &backup.ContainerVolumeRestorer{Runtime: client},
				Volumes:        client,
				Identities:     backupIdentities(backupRunner),
			}),
			api.WithBackupProtection(newBackupProtection(db, secretsManager, client, backupRunner, logger), db),
			// Streams the object straight back rather than into a container's stdin.
			api.WithBackupDownloader(&backup.DownloadRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: backup.S3Downloader{},
			}),
			// Shares the retention scheduler's deleter, so manual and automatic deletes clean up the same way.
			api.WithBackupDeleter(&backup.DeleteRunner{
				Store:   db,
				Secrets: secretsManager,
				Deleter: backup.S3Deleter{},
				Logger:  logger,
			}),
			// Re-downloads and re-checks a succeeded backup's own stored
			// object for corruption, never attempting a live restore: the
			// same instance scheduler.Verifier below uses automatically.
			api.WithBackupVerifier(backupVerifyRunner),
			// Probes a target's bucket over its stored credentials with a
			// cheap HeadBucket call: same secretsManager dependency, same
			// nil-interface hazard as everything else in this block.
			api.WithBackupTargetTester(&backup.TargetTester{
				Store:   db,
				Secrets: secretsManager,
				Tester:  backup.S3Tester{},
			}),
			// Reads and writes through secretsManager directly, so no separate runner type is needed.
			api.WithGitHubAppSecrets(secretsManager),
			// GitLab App connection: same secretsManager, same
			// nil-interface hazard, the OAuth-Application counterpart of
			// the GitHub App connection just above.
			api.WithGitLabAppSecrets(secretsManager),
			// Gitea App connection: same secretsManager, same
			// nil-interface hazard, the GitLab-shaped self-hosted
			// counterpart just above.
			api.WithGiteaAppSecrets(secretsManager),
			// Bitbucket App connection: same secretsManager, same
			// nil-interface hazard, the third OAuth-consumer counterpart
			// alongside the GitHub and GitLab App connections above.
			api.WithBitbucketAppSecrets(secretsManager),
			api.WithAIVault(ai.NewVault(db, secretsManager, logger)),
			api.WithAIUsage(db),
			// Point-in-time restore (baseBackupRunner/pitrRunner just
			// above): manual base-backup trigger and the actual PITR
			// restore endpoint.
			api.WithBaseBackupRunner(baseBackupRunner),
			api.WithPITRRestoreRunner(pitrRunner),
			api.WithMajorUpgrader(newMajorUpgradeRunner(db, client, engine.Nudge, logger)),
		)
		// Optional: without a dial address or on setup failure the
		// operator loses the assistant, not the control plane.
		if dial := dashboardDialAddr(httpAddr()); dial != "" {
			engine, err := setupAIAssistantEngine(context.Background(), db, secretsManager, dial, b.Name, logger)
			if err != nil {
				logger.Error("ai assistant: setup failed, chat routes stay disabled", slog.String("error", err.Error()))
			} else {
				opts = append(opts, api.WithAIEngine(engine))
			}
		}
	}
	if builder != nil {
		opts = append(opts, api.WithBuilder(builder))
	}
	if manifestCfg, err := loadGitHubAppManifestConfig(); err != nil {
		logger.Error("load github app manifest config failed, using defaults", slog.String("error", err.Error()))
	} else {
		opts = append(opts, api.WithGitHubAppManifestConfig(manifestCfg))
	}
	opts = append(opts, api.WithChangelog(loadChangelog(logger)), api.WithUpgradeHistory(db), api.WithSelfUpgrade(db, nil))

	modelSvc, modelGateway, _ := modelWiring(db, secretsManager)
	wireModelPreflight(modelSvc, client, b.ShortName, logger)
	modelSvc.SetEngineMetricsReader(telemetryDB)
	if engine != nil {
		modelGateway.SetWakeHook(engine.Nudge)
	}
	opts = append(opts, api.WithModels(modelSvc))
	opts = append(opts, api.WithMeshSettings(db), api.WithFixit(db), api.WithBuildPerformance(db))
	if client != nil {
		opts = append(opts, api.WithGPUHostDiagnoser(func(ctx context.Context) gpu.HostDiagnosis {
			return gpu.DiagnoseHost(ctx, gpu.ExecRunner{}, client, nil)
		}))
	}
	rt := api.NewRouter(logger, b, db, opts...)
	return rt.StatusHostHandler(modelGateway.Middleware(composeMux(rt.Handler(), webhookHandler, rt.WithHSTS(web.Handler(b))))), rt
}

// Registers /healthz explicitly: otherwise it falls through to the SPA and probers get HTML instead of JSON.
func composeMux(apiHandler http.Handler, webhookHandler http.Handler, webHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/healthz", apiHandler)
	mux.Handle("/readyz", apiHandler)
	mux.Handle("/metrics", apiHandler)
	mux.Handle("/.well-known/jwks.json", apiHandler)
	mux.Handle("/api/", apiHandler)
	mux.Handle("/public/", apiHandler)
	mux.Handle("/gate/", apiHandler)
	if webhookHandler != nil {
		mux.Handle("POST /webhook", webhookHandler)
	}
	mux.Handle("/", webHandler)
	return mux
}

// Resolved here, not in the scheduler, which takes an interval with no fallback of its own.
func backupSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_BACKUP_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultBackupSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_BACKUP_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultBackupSchedulerInterval
	}
	return d
}

func scheduledTaskSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SCHEDULED_TASK_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultScheduledTaskSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SCHEDULED_TASK_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultScheduledTaskSchedulerInterval
	}
	return d
}

func appScheduleSchedulerInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL")
	if raw == "" {
		return defaultAppScheduleSchedulerInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SCHEDULE_DEPLOY_SCHEDULER_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultAppScheduleSchedulerInterval
	}
	return d
}

func httpAddr() string {
	return resolveHTTPAddr(os.Getenv, runningInContainer())
}

// ingressHTTPSAddr reads APP_INGRESS_HTTPS_ADDR, defaulting to
// defaultIngressHTTPSAddr, and is passed to
// ingressreconcile.WithListenAddr.
func ingressHTTPSAddr() string {
	addr := os.Getenv("APP_INGRESS_HTTPS_ADDR")
	if addr == "" {
		addr = defaultIngressHTTPSAddr
	}
	return addr
}

// ingressHTTPAddr reads APP_INGRESS_HTTP_ADDR, defaulting to
// defaultIngressHTTPAddr, and is passed to
// ingressreconcile.WithHTTPListenAddr.
func ingressHTTPAddr() string {
	addr := os.Getenv("APP_INGRESS_HTTP_ADDR")
	if addr == "" {
		addr = defaultIngressHTTPAddr
	}
	return addr
}

// An unparseable address degrades to 0, which means the default ports.
func ingressPortFromAddr(addr string) int {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	return port
}

// Passed to both the API and the firewall reconciler so no firewall rule can close a platform port.
func newExposureManager(prefix string) *exposure.Manager {
	protected := append(platformRequiredPorts(), api.HostFirewallSSHPorts()...)
	return exposure.NewManager(prefix, protected)
}

func platformRequiredPorts() []int {
	return []int{
		ingressPortFromAddr(httpAddr()),
		ingressPortFromAddr(agentAddr()),
		ingressPortFromAddr(ingressHTTPAddr()),
		ingressPortFromAddr(ingressHTTPSAddr()),
	}
}

// Caddy and the control plane share a loopback interface, so a wildcard bind address is not a dial target.
// An explicit host set by the operator is passed through unchanged.
func dashboardDialAddr(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// Returns 0 when unset or invalid so api.NewRouter applies its default.
func sessionTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SESSION_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SESSION_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func slowRequestThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SLOW_REQUEST_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_SLOW_REQUEST_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// criticalRequestThreshold reads APP_CRITICAL_REQUEST_THRESHOLD, the
// same shape as slowRequestThreshold but for the Error band.
func criticalRequestThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CRITICAL_REQUEST_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CRITICAL_REQUEST_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Defaults to enabled; an omitted node_id is auto-placed onto the least-loaded node.
func autoPlacementEnabled(logger *slog.Logger) bool {
	raw := os.Getenv("APP_AUTO_PLACEMENT")
	if raw == "" {
		return true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_AUTO_PLACEMENT, defaulting to enabled", slog.String("value", raw), slog.String("error", err.Error()))
		return true
	}
	return v
}

// Opt-in: HSTS is only safe with browser-trusted certificates, which this process cannot verify.
func hstsEnabled(logger *slog.Logger) bool {
	raw := os.Getenv("APP_ENABLE_HSTS")
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		logger.Warn("invalid APP_ENABLE_HSTS, defaulting to disabled", slog.String("value", raw), slog.String("error", err.Error()))
		return false
	}
	return v
}

// Shared by the API and the cert-expiry alert rule so they agree on when a certificate becomes a concern.
func certExpiryWarningWindow(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CERT_EXPIRY_WARNING_WINDOW")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CERT_EXPIRY_WARNING_WINDOW, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func certRenewalStalledThreshold(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CERT_RENEWAL_STALLED_THRESHOLD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CERT_RENEWAL_STALLED_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func patchStatusThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_PATCH_STATUS_THRESHOLD")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_PATCH_STATUS_THRESHOLD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Returns 0 when unset or invalid so alerting applies its default.
func nodeDiskSpaceThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_DISK_SPACE_THRESHOLD_PERCENT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Returns 0 when unset or invalid so alerting applies its default.
func nodeCPUThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_CPU_THRESHOLD_PERCENT")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_CPU_THRESHOLD_PERCENT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Bytes, not percent: memory has no honest node-capacity percentage. Returns 0 when unset or invalid.
func nodeMemoryThreshold(logger *slog.Logger) float64 {
	raw := os.Getenv("APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES")
	if raw == "" {
		return 0
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return v
}

// Each check is a real DNS lookup, so this runs on its own interval, not the evaluation tick.
func domainHealthCheckInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ALERT_DOMAIN_HEALTH_CHECK_INTERVAL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ALERT_DOMAIN_HEALTH_CHECK_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so alerting applies its default.
func backupMissingGracePeriod(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ALERT_BACKUP_MISSING_GRACE_PERIOD")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ALERT_BACKUP_MISSING_GRACE_PERIOD, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func previewTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PREVIEW_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_PREVIEW_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func inviteTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_INVITE_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_INVITE_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// updateCheckInterval reads APP_UPDATE_CHECK_INTERVAL as a Go duration
// string, the same env-var-with-default shape backupSchedulerInterval
// already uses.
func updateCheckInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_UPDATE_CHECK_INTERVAL")
	if raw == "" {
		return defaultUpdateCheckInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_UPDATE_CHECK_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultUpdateCheckInterval
	}
	return d
}

// Resolved here: RunPreviewSweeper takes its interval with no fallback of its own.
func previewSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_PREVIEW_SWEEP_INTERVAL")
	if raw == "" {
		return defaultPreviewSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_PREVIEW_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultPreviewSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func auditLogRetention(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_AUDIT_LOG_RETENTION_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_AUDIT_LOG_RETENTION_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so api applies its default.
func webhookDeliveryRetention(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_WEBHOOK_DELIVERY_RETENTION_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_WEBHOOK_DELIVERY_RETENTION_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// positiveIntEnv returns the env var as a positive int, or 0 (the api
// package's signal to use its own default) when unset or invalid.
func positiveIntEnv(logger *slog.Logger, name string) int {
	raw := os.Getenv(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		logger.Warn("invalid "+name+", using the default", slog.String("value", raw))
		return 0
	}
	return n
}

// Resolved here: RunWebhookDeliverySweeper takes its interval with no fallback of its own.
func webhookDeliverySweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_WEBHOOK_DELIVERY_SWEEP_INTERVAL")
	if raw == "" {
		return defaultWebhookDeliverySweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_WEBHOOK_DELIVERY_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultWebhookDeliverySweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func secretRotationWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_SECRET_ROTATION_WARN_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_SECRET_ROTATION_WARN_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so database applies its default.
func databaseSlowQueryThresholdMs(logger *slog.Logger) int {
	raw := os.Getenv("APP_DATABASE_SLOW_QUERY_THRESHOLD_MS")
	if raw == "" {
		return 0
	}
	ms, err := strconv.Atoi(raw)
	if err != nil {
		logger.Warn("invalid APP_DATABASE_SLOW_QUERY_THRESHOLD_MS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return ms
}

// Resolved here: RunAuditLogSweeper takes its interval with no fallback of its own.
func auditLogSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_AUDIT_LOG_SWEEP_INTERVAL")
	if raw == "" {
		return defaultAuditLogSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_AUDIT_LOG_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultAuditLogSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func deployApprovalTTL(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DEPLOY_APPROVAL_TTL")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DEPLOY_APPROVAL_TTL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Resolved here: RunDeployApprovalExpirySweep takes its interval with no fallback of its own.
func deployApprovalSweepInterval(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DEPLOY_APPROVAL_SWEEP_INTERVAL")
	if raw == "" {
		return defaultDeployApprovalSweepInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DEPLOY_APPROVAL_SWEEP_INTERVAL, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return defaultDeployApprovalSweepInterval
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func resourceRecommendationLookback(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_RESOURCE_RECOMMENDATION_LOOKBACK")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_RESOURCE_RECOMMENDATION_LOOKBACK, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func capacityForecastLookback(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_CAPACITY_FORECAST_LOOKBACK")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_CAPACITY_FORECAST_LOOKBACK, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorDiskWarningBytes(logger *slog.Logger) int64 {
	raw := os.Getenv("APP_DOCTOR_DISK_WARNING_BYTES")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_DISK_WARNING_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// Returns 0 when unset or invalid so api applies its default.
func doctorMasterKeyRotationWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS")
	if raw == "" {
		return 0
	}
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MASTER_KEY_ROTATION_WARN_DAYS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(days) * 24 * time.Hour
}

// Returns 0 when unset or invalid so api applies its default.
func doctorNetworkTimeout(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_NETWORK_TIMEOUT")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_NETWORK_TIMEOUT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorClockSkewWarnAge(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_CLOCK_SKEW_WARN")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_CLOCK_SKEW_WARN, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return d
}

// Returns 0 when unset or invalid so api applies its default.
func doctorDiskIOWarnLatency(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_DOCTOR_DISK_IO_WARN_MS")
	if raw == "" {
		return 0
	}
	ms, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_DISK_IO_WARN_MS, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// Returns 0 when unset or invalid so api applies its default.
func doctorMinRAMBytes(logger *slog.Logger) int64 {
	raw := os.Getenv("APP_DOCTOR_MIN_RAM_BYTES")
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MIN_RAM_BYTES, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// doctorMinCPUCount reads APP_DOCTOR_MIN_CPU_COUNT, applied to
// api.WithDoctorMinCPUCount. Returns 0 (api's own signal to fall back to
// its internal default) when unset or unparseable.
func doctorMinCPUCount(logger *slog.Logger) int {
	raw := os.Getenv("APP_DOCTOR_MIN_CPU_COUNT")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		logger.Warn("invalid APP_DOCTOR_MIN_CPU_COUNT, using the default", slog.String("value", raw), slog.String("error", err.Error()))
		return 0
	}
	return n
}

// Resolved here: api.WithAPIRateLimit treats 0 as disabled, not as the default.
func apiRateLimitReadRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_API_RATE_LIMIT_READ_RPM", defaultAPIRateLimitReadRPM)
}

func apiRateLimitWriteRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_API_RATE_LIMIT_WRITE_RPM", defaultAPIRateLimitWriteRPM)
}

// Keyed per (client IP, app name) since this unauthenticated route has no actor identity.
func webhookRateLimitRPM(logger *slog.Logger) int {
	return apiRateLimitRPM(logger, "APP_WEBHOOK_RATE_LIMIT_RPM", defaultWebhookRateLimitRPM)
}

func apiRateLimitRPM(logger *slog.Logger, envVar string, fallback int) int {
	raw := os.Getenv(envVar)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		logger.Warn("invalid "+envVar+", using the default", slog.String("value", raw), slog.Int("default", fallback))
		return fallback
	}
	return n
}

// publicHost is the address apps are reachable at: APP_PUBLIC_HOST, else
// the server's detected public IP (see ingressdriver.ResolvePublicHost).
// Resolved once per process; "" when neither is available.
func publicHost() string {
	host, _ := resolvedPublicHost()
	return host
}

var (
	publicHostOnce   sync.Once
	publicHostValue  string
	publicHostOrigin string
)

func resolvedPublicHost() (host, source string) {
	if v := strings.TrimSpace(os.Getenv("APP_PUBLIC_HOST")); v != "" {
		return v, ingressdriver.PublicHostSourceEnv
	}
	publicHostOnce.Do(func() {
		publicHostValue, publicHostOrigin = ingressdriver.ResolvePublicHost(context.Background())
	})
	return publicHostValue, publicHostOrigin
}

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
	// restartBackoff is shared for the same reason as livenessTracker.
	restartBackoff *application.RestartBackoff
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
	ingressEdge  ingressEdge
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
		controllers = append(controllers, appControllersFor(deps, services)...)
		controllers = append(controllers, databaseControllersFor(ctx, deps, databases)...)
		controllers = append(controllers, modelControllersFor(deps, modelRows)...)
		for _, n := range nodes {
			controllers = append(controllers, nodehealth.New(n.ID, deps.db, deps.heartbeatTimeout))
		}

		ingressOpts := []ingressreconcile.Option{
			ingressreconcile.WithLogger(deps.logger),
			ingressreconcile.WithPublicHost(deps.publicHost),
			ingressreconcile.WithCertStore(deps.db),
			ingressreconcile.WithServedHostsRecorder(deps.db),
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
		ingressOpts = append(ingressOpts, deps.ingressEdge.options()...)
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
		controllers = append(controllers, exposurereconcile.New(deps.db, newExposureManager(deps.firewallRulePrefix), deps.logger))

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
	if deps.secretsManager != nil {
		appOpts = append(appOpts, application.WithSecretResolver(deps.secretsManager))
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

// appControllersFor builds one application.Controller per service,
// skipping (with a warning) any whose node isn't currently reachable.
func appControllersFor(deps dynamicSourceDeps, services []store.DesiredService) []reconcile.Controller {
	appOpts := appOptionsFor(deps)
	controllers := make([]reconcile.Controller, 0, len(services))
	for _, svc := range services {
		svcRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, runtimeNodeID(deps, svc.NodeID))
		if err != nil {
			deps.logger.Warn("skipping service for this reconcile pass: node transport unavailable",
				slog.String("service", svc.Name), slog.String("node_id", svc.NodeID), slog.String("error", err.Error()))
			continue
		}
		controllers = append(controllers, application.New(svc.Name, deps.db, svcRuntime, appOpts...))
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
		controllers = append(controllers, database.New(desired.Name, deps.db, dbRuntime, dbOpts...))
	}
	return controllers
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
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name)...)
		opts = append(opts, database.WithSlowQueryThreshold(deps.databaseSlowQueryThresholdMs))
	case store.EngineRedis:
		// Redis needs no credentials to reconcile at all (this package's
		// own doc comment), so TLS is the only per-engine option it ever
		// gets here.
		opts = append(opts, databaseTLSOpt(ctx, deps, desired.Name)...)
	case store.EngineMySQL:
		creds, err := mysqlCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mysql credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMySQLCredentials(creds))
		}
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
func databaseTLSOpt(ctx context.Context, deps dynamicSourceDeps, dbName string) []database.Option {
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

// bootstrapAdmin creates the first admin from APP_ADMIN_USERNAME and
// APP_ADMIN_PASSWORD when no user exists yet. Without a password it does
// nothing and the setup-token flow (ensureSetupToken) takes over.
func bootstrapAdmin(ctx context.Context, db *store.DB) error {
	username := os.Getenv("APP_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("APP_ADMIN_PASSWORD")
	if password == "" {
		return nil
	}
	return api.BootstrapAdmin(ctx, db, username, password)
}

func publicHostSource() string {
	_, source := resolvedPublicHost()
	return source
}

// httpRedirectEnabled reads APP_INGRESS_HTTP_REDIRECT (true, false, or auto,
// the default). Auto enables the redirect only when the HTTP address can be
// bound right now, so a non-root dev run keeps working without port 80.
func httpRedirectEnabled(addr string, inherited *ingressdriver.InheritedSockets, logger *slog.Logger) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("APP_INGRESS_HTTP_REDIRECT"))) {
	case "1", "true", "on", "yes":
		return true
	case "0", "false", "off", "no":
		return false
	}
	if inherited.Active() && inherited.HTTP != 0 {
		return true
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Warn("http to https redirect disabled: cannot bind the HTTP ingress address", slog.String("addr", addr), slog.String("error", err.Error()))
		return false
	}
	_ = ln.Close()
	return true
}

// acmeDefaultDirectory returns the CA directory enable-https uses: Let's
// Encrypt staging when APP_ACME_STAGING is set, APP_ACME_DIRECTORY_URL when
// given, else "" for Let's Encrypt production.
func acmeDefaultDirectory() string {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv("APP_ACME_STAGING"))); err == nil && v {
		return api.ACMEStagingDirectoryURL
	}
	return strings.TrimSpace(os.Getenv("APP_ACME_DIRECTORY_URL"))
}

// acmePendingTimeout reads APP_ACME_PENDING_TIMEOUT; 0 keeps the default.
func acmePendingTimeout(logger *slog.Logger) time.Duration {
	raw := os.Getenv("APP_ACME_PENDING_TIMEOUT")
	if raw == "" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid APP_ACME_PENDING_TIMEOUT, using the default", slog.String("value", raw))
		return 0
	}
	return d
}

// purgeStaleIssuerCerts drops stored certificates from any CA other than the
// configured ACME one, so a leftover staging or internal certificate is never
// reused in place of a real one. No-op while ACME is disabled.
func purgeStaleIssuerCerts(ctx context.Context, db *store.DB, logger *slog.Logger) {
	settings, err := db.GetIngressSettings(ctx)
	if err != nil || !settings.ACMEEnabled {
		return
	}
	n, err := ingressdriver.PurgeCertsFromOtherIssuersOnce(ctx, db, settings.ACMEDirectoryURL)
	if err != nil {
		logger.Warn("purging certificates from other issuers failed", slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		logger.Info("dropped certificates from other issuers so they re-issue", slog.Int("keys", n))
	}
}

// wakeLogCollectorOnContainerEvents makes the log collector re-derive
// targets on container start/die so containers shorter-lived than the
// resync tick still get their output captured.
func wakeLogCollectorOnContainerEvents(ctx context.Context, src alerting.EventSource, lc *telemetry.LogCollector) {
	events, errs := src.Events(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Action == docker.EventStart || ev.Action == docker.EventDie {
				lc.Wake()
			}
		case _, ok := <-errs:
			if !ok {
				errs = nil
			}
		}
	}
}
