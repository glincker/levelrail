package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/archive"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/cpbackup"
	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/gpu"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/nodefailure"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/selfupgrade"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/web"
)

// rootDeps carries everything rootHandler wires into the API router.
type rootDeps struct {
	logger              *slog.Logger
	b                   *brand.Brand
	db                  *store.DB
	telemetryDB         *telemetry.DB
	alertingDB          *alerting.DB
	secretsManager      *secrets.Manager
	masterKeyFilePath   string
	webhookHandler      http.Handler
	client              *docker.Client
	builder             *deploy.Pipeline
	deployRecorder      *deploylog.Recorder
	logBroadcaster      *telemetry.LogBroadcaster
	deployDispatcher    *alerting.DeployDispatcher
	backupRunner        *backup.Runner
	backupVerifyRunner  *backup.VerifyRunner
	agentRegistry       *agent.Registry
	agentCAFingerprint  string
	emailSender         email.Sender
	scheduledTaskRunner *scheduledtask.Runner
	functionQueue       *functions.Queue
	engine              *reconcile.Engine
	ingressDriver       *ingressdriver.Driver
	pushVAPIDPublicKey  string
}

// rootHandler mounts the API under /api/ and the embedded frontend everywhere else, on one server.
// Optional dependencies (secrets, webhook, builder) are applied only when non-nil, to avoid nil-interface panics.
func rootHandler(d rootDeps) (http.Handler, *api.Router) {
	logger := d.logger
	b := d.b
	db := d.db
	telemetryDB := d.telemetryDB
	alertingDB := d.alertingDB
	secretsManager := d.secretsManager
	masterKeyFilePath := d.masterKeyFilePath
	webhookHandler := d.webhookHandler
	client := d.client
	builder := d.builder
	deployRecorder := d.deployRecorder
	logBroadcaster := d.logBroadcaster
	deployDispatcher := d.deployDispatcher
	backupRunner := d.backupRunner
	backupVerifyRunner := d.backupVerifyRunner
	agentRegistry := d.agentRegistry
	agentCAFingerprint := d.agentCAFingerprint
	emailSender := d.emailSender
	scheduledTaskRunner := d.scheduledTaskRunner
	fq := d.functionQueue
	if fq == nil {
		fq = &functions.Queue{Config: functions.TriggerConfigFromEnv(os.LookupEnv)}
	}
	engine := d.engine
	ingressDriver := d.ingressDriver
	pushVAPIDPublicKey := d.pushVAPIDPublicKey
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
		api.WithAlertDefaults(alertingDB),
		api.WithInbox(alertingDB),
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
		api.WithDatabasePasswordRotation(db),
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
		api.WithReleaseSchemaSource(api.NewReleaseSchemaFetcher(dataDir, trustRoot)),
		api.WithSourceCache(sourcecache.New(sourcecache.ConfigFromEnv(filepath.Join(dataDir, "source-cache")), logger)),
		api.WithControlPlaneBackups(cpbackup.NewManager(db, dataDir)),
		api.WithArchives(archive.New(db.DB, dataDir), db),
		multiNodeOption(db),
		api.WithNodeFailure(db, nodefailure.ConfigFromEnv(os.LookupEnv)),
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
		buildCacheOption(func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(client, agentRegistry, nodeID)
		}),
		imageVerifyOption(logger),
		// Applied unconditionally: scheduledTaskRunner is always non-nil.
		api.WithScheduledTaskRunner(scheduledTaskRunner),
		api.WithJobLimits(jobsPerAppCap(scheduledTaskRunner)),
		api.WithFunctionTriggers(db, fq.Config, fq.Kick),
		api.WithCertExpiryWarningWindow(certExpiryWarningWindow(logger)),
		api.WithCertRenewalTracking(alertingDB, certRenewalStalledThreshold(logger)),
		api.WithNodeAlertThresholds(
			patchStatusThreshold(logger), nodeDiskSpaceThreshold(logger),
			nodeCPUThreshold(logger), nodeMemoryThreshold(logger),
		),
		api.WithResourceRecommendationLookback(resourceRecommendationLookback(logger)),
		api.WithAnalysisReports(db),
		api.WithCapacityForecastLookback(capacityForecastLookback(logger)),
		api.WithPreviewTTL(previewTTL(logger)),
		api.WithPreviewLimits(previewLimits(logger)),
		api.WithPRCommentCommands(prCommentCommandConfig(logger)),
		api.WithPreviewStuckAfter(previewStuckAfter(logger)),
		api.WithStaticOrphanGrace(staticOrphanGrace(logger)),
		api.WithInviteTTL(inviteTTL(logger)),
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
		api.WithPublishLocks(db),
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
		// Traffic export sink credentials use the same secretsManager; the service taps the ingress access log.
		if svc := startTrafficExport(context.Background(), logger, b, db, secretsManager); svc != nil {
			opts = append(opts, api.WithTrafficExport(secretsManager, svc))
		}
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
		opts = append(opts, api.WithGoogleCloudDNSSecrets(secretsManager))
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
			Downloader:     routedStorage(agentRegistry),
			Restorer:       &backup.ContainerRestorer{Runtime: client},
			VolumeRestorer: &backup.ContainerVolumeRestorer{Runtime: client},
			Identities:     backupIdentities(backupRunner),
		}
		// pitrRunner's Nudge is the engine, so a PITR wipe's suspend and unsuspend are picked up immediately.
		baseBackupRunner := newBaseBackupRunner(db, secretsManager, client, backupRunner, logger)
		pitrRunner := &backup.PITRRunner{
			Store:      db,
			Secrets:    secretsManager,
			Downloader: routedStorage(agentRegistry),
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
			api.WithRegistryRefresher(newRegistryRefresher(db, secretsManager, logger)),
			// Network shares (CIFS password only; NFS shares write no
			// secret): same secretsManager, same nil-interface hazard.
			api.WithNetworkShareSecrets(secretsManager),
			api.WithBackupRunner(backupRunner),
			mailBackupOption(backupRunner, secretsManager),
			// Volume backups reuse the database runner: one implementation serves both resource kinds.
			api.WithServiceVolumeBackupRunner(backupRunner),
			// Needs ExecWithInput on the local runtime, the same way ContainerDumper needs Exec.
			api.WithRestoreRunner(restoreRunner),
			api.WithServiceVolumeRestoreRunner(restoreRunner),
			api.WithRehydrateStore(db),
			// Waits for the new database to come up, unlike the restore it sits beside.
			api.WithCloneRestoreRunner(&backup.CloneRestoreRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: routedStorage(agentRegistry),
				Restorer:   &backup.ContainerRestorer{Runtime: client},
				Identities: backupIdentities(backupRunner),
			}),
			// Creates the volume synchronously: a bare volume has no reconciler to wait on.
			api.WithVolumeCloneRestoreRunner(&backup.VolumeCloneRestoreRunner{
				Store:          db,
				Secrets:        secretsManager,
				Downloader:     routedStorage(agentRegistry),
				VolumeRestorer: &backup.ContainerVolumeRestorer{Runtime: client},
				Volumes:        client,
				Identities:     backupIdentities(backupRunner),
			}),
			api.WithBackupProtection(newBackupProtection(db, secretsManager, client, backupRunner, logger), db),
			// Streams the object straight back rather than into a container's stdin.
			api.WithBackupDownloader(&backup.DownloadRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: routedStorage(agentRegistry),
			}),
			// Shares the retention scheduler's deleter, so manual and automatic deletes clean up the same way.
			api.WithBackupDeleter(&backup.DeleteRunner{
				Store:      db,
				Secrets:    secretsManager,
				Deleter:    routedStorage(agentRegistry),
				Tombstones: db,
				Logger:     logger,
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
				Tester:  routedStorage(agentRegistry),
			}),
			// Reads and writes through secretsManager directly, so no separate runner type is needed.
			api.WithGitHubAppSecrets(secretsManager),
			// GitLab, Bitbucket and Gitea connections share the secrets manager, same
			// nil-interface hazard as the GitHub App connection just above.
			api.WithGitConnectionSecrets(secretsManager),
			api.WithAIVault(ai.NewVault(db, secretsManager, logger)),
			api.WithAIUsage(db),
			// Point-in-time restore (baseBackupRunner/pitrRunner just
			// above): manual base-backup trigger and the actual PITR
			// restore endpoint.
			api.WithBaseBackupRunner(baseBackupRunner),
			api.WithPITRRestoreRunner(pitrRunner),
			api.WithMySQLPITRRestoreRunner(newMySQLPITRRunner(db, secretsManager, client, routedStorage(agentRegistry), logger)),
			api.WithMajorUpgrader(newMajorUpgradeRunner(db, client, engine.Nudge, logger)),
		)
		// Optional: without a dial address the
		// operator loses the assistant, not the control plane.
		if dial := dashboardDialAddr(httpAddr()); dial != "" {
			engine := setupAIAssistantEngine(context.Background(), db, secretsManager, dial, b.Name, logger)
			opts = append(opts, api.WithAIEngine(engine))
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
	opts = append(opts, api.WithMeshSettings(db), api.WithServerProfile(db), api.WithFixit(db), api.WithBuildPerformance(db))
	if client != nil {
		opts = append(opts, api.WithGPUHostDiagnoser(func(ctx context.Context) gpu.HostDiagnosis {
			return gpu.DiagnoseHost(ctx, gpu.ExecRunner{}, client, nil)
		}))
	}
	rt := api.NewRouter(logger, b, db, opts...)
	if builder != nil {
		// Spec file deploys (webhook, git push, builds) admit host ports like POST /apps.
		builder.SetPortAdmitter(rt)
	}
	wireBindMountSettings(rt)
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
	mux.Handle("/mail/unsubscribe", apiHandler)
	if webhookHandler != nil {
		mux.Handle("POST /webhook", webhookHandler)
	}
	mux.Handle("/", webHandler)
	return mux
}
