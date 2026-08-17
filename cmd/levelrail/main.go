// Command levelrail is the control plane binary. It starts the TASKS.md
// 1.9 HTTP API and a reconcile engine whose controller set is derived
// dynamically from desired state in the store every pass (see
// dynamicSource): one application.Controller per desired service, one
// database.Controller per desired database, plus a single ingress
// controller routing every service with domains. Everything else in the
// locked architecture (multi-node, WireGuard, MCP) lands in later phases.
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
	"strings"
	"syscall"
	"time"

	dockerclient "github.com/docker/docker/client"
	"google.golang.org/grpc"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	meshreconcile "github.com/GLINCKER/levelrail/internal/reconcile/mesh"
	"github.com/GLINCKER/levelrail/internal/reconcile/nodehealth"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/webhook"
	"github.com/GLINCKER/levelrail/web"
)

const (
	resyncInterval = 30 * time.Second
	// defaultDataDir matches the documented APP_DATA_DIR override
	// pattern. Relative, not /var/lib/levelrail-data (brand.yaml's noted
	// pending path), since Phase 0 runs locally, not installed as a
	// service yet.
	defaultDataDir = "./data"
	// defaultBrandFile is the same "relative default, env override"
	// pattern as defaultDataDir.
	defaultBrandFile = "./brand.yaml"
	// defaultGitHubAppManifestFile is the same "relative default, env
	// override" pattern as defaultBrandFile, for the optional GitHub App
	// manifest permissions/events file (internal/githubapp.ManifestConfig).
	// Unlike brand.yaml, absence is not an error: loadGitHubAppManifestConfig
	// falls back to githubapp.DefaultManifestConfig().
	defaultGitHubAppManifestFile = "./github-app-manifest.yaml"
	// defaultDevFixturesFile is the same "relative default, env
	// override" pattern as defaultBrandFile, for the optional dev-mode
	// fixture tokens file (internal/api/devfixtures.go). Only ever read
	// when APP_DEV_MODE=1, see MaybeSeedDevFixturesFromFile's own gate.
	defaultDevFixturesFile = "./dev-fixtures.yml"
	// defaultHTTPAddr is where the TASKS.md 1.9 HTTP API listens.
	defaultHTTPAddr = ":8080"

	// defaultAgentAddr is where TASKS.md 3.2's agent gRPC service
	// (Enroll, Session) listens: a distinct port from defaultHTTPAddr,
	// not multiplexed onto it, since the two have entirely different
	// transport security models (plain HTTP with its own session/token
	// auth vs. mTLS with a private, self-issued CA, ADR 003).
	defaultAgentAddr = ":9443"
	// agentServerCertValidity is how long the control plane's own gRPC
	// listener certificate is valid. Regenerated fresh on every startup
	// (unlike the CA itself, which persists, see loadOrGenerateCA): a
	// long-running control plane's own process lifetime is what
	// actually bounds this in practice, so persisting it buys nothing a
	// restart wouldn't already refresh.
	agentServerCertValidity = 365 * 24 * time.Hour

	httpShutdownTimeout = 10 * time.Second

	// metricsCollectionInterval matches the observability design's "15s
	// resolution" default. Not env-configurable like retention below:
	// resolution changes the shape of every stored sample going forward,
	// retention only trims how much of that shape is kept, a materially
	// smaller blast radius for an operator to tune without redesigning
	// the schema.
	metricsCollectionInterval = 15 * time.Second
	// defaultMetricsRetention matches the observability design's
	// "default 15 days" retention.
	defaultMetricsRetention = 15 * 24 * time.Hour
	// metricsRetentionSweepInterval: how often the retention sweep runs,
	// not how long data is kept (that's the retention duration itself).
	// Hourly is frequent enough that the table never grows much past its
	// steady-state size, infrequent enough to be a non-event on a
	// control plane whose idle cost the project's hardening phase wants
	// measured.
	metricsRetentionSweepInterval = 1 * time.Hour

	// logTargetsResyncInterval is how often the log collector re-derives
	// which containers it should be streaming from (TASKS.md 2.2), kept
	// as its own constant rather than reusing resyncInterval: it governs
	// a different concern (log stream subscriptions, not reconcile
	// passes) even though the two currently share the same value.
	logTargetsResyncInterval = 30 * time.Second
	// defaultLogsRetention matches the observability design's "default
	// 15 days," the same default metrics uses, applied to the sibling
	// log store.
	defaultLogsRetention = 15 * 24 * time.Hour
	// logsRetentionSweepInterval mirrors metricsRetentionSweepInterval's
	// own reasoning: frequent enough that log_entries never grows much
	// past its steady-state size, infrequent enough to be a non-event.
	logsRetentionSweepInterval = 1 * time.Hour

	// alertEvaluationInterval is how often internal/alerting's Engine
	// evaluates every enabled rule (TASKS.md 2.5/2.7). Deliberately
	// coarser than metricsCollectionInterval's 15s: a threshold rule's
	// own ForDuration is the real debounce against a noisy single
	// sample, so evaluating every tick metrics are collected buys
	// nothing but a quadrupled query load against telemetryDB. 30s means
	// a firing or resolved transition is noticed within, at worst, 30s
	// of the condition actually changing, an acceptable latency for a
	// notification, not a live dashboard.
	alertEvaluationInterval = 30 * time.Second

	// restartTrackerResyncInterval is how often alerting.RestartTracker
	// re-derives its known-services list from the store (the same
	// "resync" concern resyncInterval and logTargetsResyncInterval
	// already name for their own loops), not how often it processes
	// Docker start events, which is driven continuously by the event
	// stream itself.
	restartTrackerResyncInterval = 30 * time.Second

	// defaultNodeHeartbeatInterval matches internal/agent's own
	// defaultHeartbeatInterval (TASKS.md 3.7): how often a connected
	// node's last_seen_at is touched while its session stream stays
	// open. Duplicated as a constant here (not imported) purely so this
	// file's own env-var-with-default convention stays self-contained,
	// the same reasoning applicationControllerName's own doc comment
	// gives for a different small duplication.
	defaultNodeHeartbeatInterval = 15 * time.Second
	// defaultNodeHeartbeatTimeout is how long since a node's last_seen_at
	// internal/reconcile/nodehealth treats as stale (three missed
	// heartbeats at the default interval above): long enough that one
	// slow tick or a brief GC pause doesn't flip a healthy node Offline,
	// short enough that a real hang is caught well within a minute.
	defaultNodeHeartbeatTimeout = 45 * time.Second

	// defaultBackupSchedulerInterval is how often internal/backup.Scheduler
	// checks which databases have a due cron schedule (wave-2 roadmap
	// item 6), env-overridable via APP_BACKUP_SCHEDULER_INTERVAL
	// (backupSchedulerInterval below), the project's "no hardcoded
	// thresholds, use env vars" rule. One minute, not something coarser
	// like alertEvaluationInterval's 30s: standard 5-field cron
	// (internal/cronexpr's own doc comment) has no granularity finer
	// than a minute, so anything longer than a minute here would mean a
	// schedule like "*/1 * * * *" could never actually fire on time, and
	// anything shorter buys nothing since no schedule this parser can
	// express would ever need to be checked more often than once a
	// minute anyway.
	defaultBackupSchedulerInterval = 1 * time.Minute
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if len(os.Args) > 1 && os.Args[1] == "recover-admin" {
		if err := runRecoverAdmin(context.Background(), logger, os.Args[2:], os.Stdout, openStore); err != nil {
			logger.Error("recover-admin failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		return
	}

	if err := run(logger); err != nil {
		logger.Error("exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := docker.NewClient()
	if err != nil {
		return err
	}
	defer func() {
		if cerr := client.Close(); cerr != nil {
			logger.Error("closing docker client", slog.String("error", cerr.Error()))
		}
	}()

	db, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logger.Error("closing store", slog.String("error", cerr.Error()))
		}
	}()

	b, err := loadBrand()
	if err != nil {
		return err
	}

	if err := bootstrapAdmin(ctx, db); err != nil {
		// Not fatal: the control plane still starts and serves the public
		// /api/v1/brand endpoint. Every auth-required route just stays
		// inaccessible until an operator sets APP_ADMIN_USERNAME and
		// APP_ADMIN_PASSWORD and restarts, which is discoverable from this
		// log line rather than a startup crash.
		logger.Warn("admin account not bootstrapped", slog.String("error", err.Error()))
	}

	// A no-op unless APP_DEV_MODE=1 in a non -tags embedweb build (see
	// internal/api/devmode_debug.go / devmode_release.go). Called after
	// bootstrapAdmin, not before: BootstrapAdmin no-ops once any admin
	// exists, so an operator-set APP_ADMIN_USERNAME/APP_ADMIN_PASSWORD
	// always wins over the fixed dev/dev fallback, regardless of this
	// call order, but placing it second documents that intent.
	if err := api.MaybeBootstrapDevAdmin(ctx, db, logger); err != nil {
		logger.Warn("dev admin account not bootstrapped", slog.String("error", err.Error()))
	}

	// Also a no-op unless APP_DEV_MODE=1 (same gate as the dev admin
	// account above). A missing dev-fixtures.yml is not an error: it is
	// an opt-in convenience layered on top of dev mode, not a
	// requirement for dev mode to work at all.
	if err := api.MaybeSeedDevFixturesFromFile(ctx, db, logger, devFixturesFile()); err != nil {
		logger.Warn("dev fixtures not seeded", slog.String("error", err.Error()))
	}

	telemetryDB, err := openTelemetryStore(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := telemetryDB.Close(); cerr != nil {
			logger.Error("closing telemetry store", slog.String("error", cerr.Error()))
		}
	}()

	// deployRecorder is the one place internal/build.ProgressEvent output
	// from any of the three real trigger paths becomes both a persisted
	// row in telemetryDB and a live SSE fan-out (internal/deploylog's own
	// package doc comment). Constructed once, here, and shared by
	// reference into both api.Router (via api.WithDeployRecorder, see
	// rootHandler) and the webhook handler (loadWebhookHandler): a
	// webhook-triggered build's live output must be visible to a viewer
	// connected through the HTTP API's SSE route, which only works if
	// both consumers publish to and read from the exact same in-memory
	// subscriber set.
	deployRecorder := deploylog.NewRecorder(telemetryDB, logger)

	// logBroadcaster is deployRecorder's live-fan-out counterpart for app
	// container logs (internal/telemetry.LogBroadcaster's own doc
	// comment) rather than build/deploy output. Constructed once, here,
	// and shared by reference into both logCollector below (the
	// publishing side: StreamOne publishes every line it receives from
	// Docker) and api.Router (via api.WithLogBroadcaster, see
	// rootHandler), the subscribing side an SSE viewer connects to.
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

	// TASKS.md 3.2: the agent gRPC service. agentRegistry starts empty
	// and stays that way in this pass, wired to nothing else yet: 3.3
	// (placement) is what will have dynamicSource actually look nodes
	// up in it. Building and running the real Enroll/Session server now
	// anyway (not deferred to 3.3) is deliberate, matching the same
	// "build the primitive, prove it live, wire consumers later" shape
	// 3.1's own internal/agent package already used: a control plane
	// that can't yet route reconciles to a second node can still
	// legitimately accept an agent's enrollment and hold its connection
	// open.
	agentDataDir := os.Getenv("APP_DATA_DIR")
	if agentDataDir == "" {
		agentDataDir = defaultDataDir
	}
	agentCA, err := loadOrGenerateAgentCA(agentDataDir)
	if err != nil {
		return fmt.Errorf("load or generate agent CA: %w", err)
	}
	agentRegistry := agent.NewRegistry()
	agentCreds, err := agent.NewServerCredentials(agentCA, []string{agentAdvertiseHost()}, agentServerCertValidity)
	if err != nil {
		return fmt.Errorf("build agent grpc credentials: %w", err)
	}
	agentListener, err := net.Listen("tcp", agentAddr())
	if err != nil {
		return fmt.Errorf("start agent grpc listener: %w", err)
	}
	agentGRPCServer := grpc.NewServer(grpc.Creds(agentCreds))
	agentpb.RegisterAgentServiceServer(agentGRPCServer, agent.NewServer(agentCA, db, agentRegistry, logger, agent.WithHeartbeatInterval(nodeHeartbeatInterval())))
	go func() {
		logger.Info("agent grpc listening", slog.String("addr", agentListener.Addr().String()))
		if err := agentGRPCServer.Serve(agentListener); err != nil {
			logger.Error("agent grpc server stopped", slog.String("error", err.Error()))
		}
	}()
	defer agentGRPCServer.GracefulStop()

	secretsManager, err := loadSecretsManager(db, agentDataDir)
	if err != nil {
		// Not fatal, the same choice bootstrapAdmin makes above: the
		// control plane still starts. Every route and reconcile path
		// touching secrets (PUT .../secrets/{key}, any service declaring
		// a { secret: true } env var) fails loudly and specifically
		// instead, rather than this refusing to start at all. This is
		// now a genuine I/O failure (can't read/write the persisted key
		// file), not "operator hasn't configured secrets yet": an unset
		// APP_MASTER_KEY no longer leaves secrets disabled, see
		// loadSecretsManager.
		logger.Warn("secrets not configured", slog.String("error", err.Error()))
	}

	// emailSender is shared by alert-rule/deploy-outcome notifications
	// and the password-reset flow. Always non-nil: a DynamicSender
	// resolves email_settings (falling back to APP_SMTP_* env vars)
	// fresh on every send, deferring "not configured" to send time.
	emailSender := email.NewDynamicSender(emailConfigLoader(db, secretsManager, smtpConfigFromEnv()))
	deployDispatcher := alerting.NewDeployDispatcher(alertingDB, nil, emailSender, logger)

	// backupRunner is constructed once, here in run(), not inside
	// rootHandler where it used to live: wave-2 roadmap item 6
	// (scheduled backups) needs the exact same *backup.Runner instance
	// in two places, api.Router's manual trigger path (WithBackupRunner
	// below, via rootHandler) and the scheduled-backup loop started
	// further down this function, the same "construct once, share by
	// reference" reasoning deployRecorder and logBroadcaster already
	// follow above for their own two-consumer dependencies. nil
	// whenever secretsManager is nil (no APP_MASTER_KEY configured):
	// every backup-touching feature, manual or scheduled, shares that
	// one gate, per rootHandler's own doc comment on why backups need a
	// master key at all.
	var backupRunner *backup.Runner
	if secretsManager != nil {
		backupRunner = &backup.Runner{
			Store:    db,
			Secrets:  secretsManager,
			Dumper:   &backup.ContainerDumper{Runtime: client},
			Uploader: backup.S3Uploader{},
		}
	}

	builder, closeBuilder, err := loadBuilder(ctx, logger, db, telemetryDB, secretsManager, agentRegistry)
	if err != nil {
		// Not fatal, the same choice as everything else optional above:
		// the control plane still starts, serving apps deployed by hand
		// through the HTTP API with an already-built image tag
		// (POST .../deploys). Both build-dependent paths (git push,
		// manual build trigger) are unavailable, and specifically why is
		// right here in the log.
		logger.Warn("builder not configured: git webhook deploys and manual build trigger are both unavailable", slog.String("error", err.Error()))
	}
	if closeBuilder != nil {
		defer func() {
			if cerr := closeBuilder(); cerr != nil {
				logger.Error("closing build client", slog.String("error", cerr.Error()))
			}
		}()
	}

	webhookHandler, err := loadWebhookHandler(logger, b, db, deployRecorder, deployDispatcher, builder)
	if err != nil {
		// Not fatal, the same choice as everything else optional above:
		// the control plane still starts, serving apps deployed by
		// hand through the HTTP API. Only the git-push path is
		// unavailable, and specifically why is right here in the log.
		logger.Warn("webhook not configured", slog.String("error", err.Error()))
	}

	httpServer := &http.Server{
		Addr:              httpAddr(),
		Handler:           rootHandler(logger, b, db, telemetryDB, alertingDB, secretsManager, webhookHandler, client, builder, deployRecorder, logBroadcaster, deployDispatcher, backupRunner, agentRegistry, emailSender),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ingressDriver := ingressdriver.New(logger)
	defer func() {
		if cerr := ingressDriver.Stop(context.Background()); cerr != nil {
			logger.Error("stopping ingress driver", slog.String("error", cerr.Error()))
		}
	}()

	meshCfg, err := setupMesh(ctx, db, b, agentDataDir, logger)
	if err != nil {
		// Not fatal: the same choice this function already makes for
		// secrets and webhook above. A misconfigured, opted-in mesh
		// (APP_MESH_ENABLED=1) should not take down a control plane
		// that would otherwise run fine without it.
		logger.Warn("mesh not configured", slog.String("error", err.Error()))
	}
	if meshCfg != nil {
		defer meshCfg.close()
	}
	// Resolved once at startup, not per reconcile pass: the bridge
	// gateway IP a container-reachable mesh DNS address depends on
	// (containerDNSAddr's own doc comment) does not change while this
	// process runs, so every controller dynamicSource builds below
	// shares the one lookup instead of each re-querying the Docker
	// daemon on every tick.
	meshDNSAddr := containerDNSAddr(ctx, client, meshCfg, logger)

	engine := reconcile.NewEngine(logger)
	engine.SetStore(db)
	engine.SetSource(dynamicSource(dynamicSourceDeps{
		db:               db,
		runtime:          client,
		driver:           ingressDriver,
		logger:           logger,
		telemetryDB:      telemetryDB,
		secretsManager:   secretsManager,
		agentRegistry:    agentRegistry,
		heartbeatTimeout: nodeHeartbeatTimeout(),
		meshCfg:          meshCfg,
		meshDNSAddr:      meshDNSAddr,
		dashboardDial:    dashboardDialAddr(httpAddr()),
		networkPrefix:    b.ShortName,
	}))

	collector := telemetry.NewCollector(client, telemetryDB, metricsCollectionInterval, logger)
	go func() {
		if err := collector.Run(ctx, telemetryTargets(db, client)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry collector stopped", slog.String("error", err.Error()))
		}
	}()
	go runMetricsRetentionSweep(ctx, telemetryDB, logger)

	logCollector := telemetry.NewLogCollector(client, telemetryDB, logBroadcaster, logger)
	go func() {
		if err := logCollector.Run(ctx, logTargetsResyncInterval, logTargets(db, client)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry log collector stopped", slog.String("error", err.Error()))
		}
	}()
	go runLogsRetentionSweep(ctx, telemetryDB, logger)

	// Alerting (TASKS.md 2.5/2.7): RestartTracker watches the same
	// Docker client's event stream, independently of the reconcile
	// engine's own subscription below, to count restarts per service for
	// crashloop rules; Engine evaluates every enabled rule
	// (threshold and crashloop alike) on its own tick, querying
	// telemetryDB through the same kind of federator rootHandler already
	// builds for the HTTP query routes.
	restartTracker := alerting.NewRestartTracker()
	go func() {
		if err := restartTracker.Run(ctx, client, db, restartTrackerResyncInterval, logger); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("alerting restart tracker stopped", slog.String("error", err.Error()))
		}
	}()

	alertingFederator := telemetry.NewLocalFederator(telemetryDB)
	alertingNewNotifier := func(r alerting.Rule) alerting.Notifier { return alerting.NewNotifier(nil, emailSender, r) }
	alertingEngine := alerting.NewEngine(alertingDB, alertingFederator, alertingFederator, restartTracker, alertingNewNotifier, logger)
	go func() {
		if err := alertingEngine.Run(ctx, alertEvaluationInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("alerting engine stopped", slog.String("error", err.Error()))
		}
	}()

	// Scheduled backups (wave-2 roadmap item 6): internal/backup.Scheduler
	// checks, on its own tick, which databases have a due cron schedule
	// (store.DesiredDatabase's BackupSchedule/BackupTargetID/BackupRetain,
	// migrations/0023_scheduled_backups.sql) and runs them through the
	// same backupRunner the manual trigger path (api.WithBackupRunner
	// above) uses. Gated on backupRunner being non-nil, the identical
	// "no APP_MASTER_KEY, no backup features at all" boundary rootHandler
	// already draws for the manual path: a scheduled backup needs the
	// same stored, decryptable target credentials a manual one does, so
	// there is nothing for the scheduler to usefully do without one
	// either.
	if backupRunner != nil {
		scheduler := backup.NewScheduler(db, backupRunner, backup.S3Deleter{}, logger)
		go func() {
			if err := scheduler.Run(ctx, backupSchedulerInterval(logger)); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("backup scheduler stopped", slog.String("error", err.Error()))
			}
		}()
	}

	events, errs := client.Events(ctx)
	go func() {
		if err, ok := <-errs; ok && err != nil {
			logger.Error("docker event stream error", slog.String("error", err.Error()))
		}
	}()

	serveErrs := make(chan error, 1)
	go func() {
		logger.Info("http api listening", slog.String("addr", httpServer.Addr))
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
		return nil // Ctrl+C / SIGTERM is a clean shutdown, not a failure
	}
	return engineErr
}

func openStore(ctx context.Context) (*store.DB, error) {
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
	return store.Open(ctx, filepath.Join(dataDir, "levelrail.db"))
}

// openTelemetryStore opens the TASKS.md 2.1 metrics store on its own
// SQLite file (telemetry.db, alongside levelrail.db in the same data
// directory), per ADR 009: a separate file so its collection-tick write
// pattern never contends with levelrail.db's own WAL.
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

// openAlertingStore opens the TASKS.md 2.5/2.7 alert-rules store on its
// own SQLite file (alerting.db, alongside levelrail.db and
// telemetry.db in the same data directory), the same per-concern
// write-isolation reasoning (ADR 009) telemetry.db's own separation
// from levelrail.db already applies, per alerting.DB's own doc comment.
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

// smtpConfigFromEnv reads APP_SMTP_HOST/APP_SMTP_PORT/APP_SMTP_USERNAME/
// APP_SMTP_PASSWORD/APP_SMTP_FROM: the pre-existing, env-only SMTP path,
// kept as emailConfigLoader's fallback for when email_settings has no
// backend configured. Nil is a valid, expected outcome, not an error.
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

// agentCACertFilename/agentCAKeyFilename are TASKS.md 3.2's private CA
// (internal/agent.CA), persisted alongside levelrail.db/telemetry.db/
// alerting.db in the same data directory, the same per-concern
// separation ADR 009 already applies to this control plane's other
// SQLite files, applied here to a different kind of state.
const (
	agentCACertFilename = "agent-ca.crt.pem"
	agentCAKeyFilename  = "agent-ca.key.pem"

	// masterKeyFilename is the persisted secrets.MasterKey fallback used
	// when APP_MASTER_KEY isn't set, same data directory as the agent CA
	// above.
	masterKeyFilename = "master.key"
)

// loadOrGenerateAgentCA loads a previously persisted CA from dataDir, or
// generates and persists a fresh one if none exists yet. Unlike the
// SQLite stores opened elsewhere in this file, a missing CA on first
// startup is expected, not an error condition to report differently:
// every control plane's very first run has no CA yet by definition.
// Regenerating on every restart instead of persisting would invalidate
// every already-enrolled agent's certificate for no reason, the same
// reasoning LoadCA's own doc comment already gives.
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

// agentAdvertiseHost reads APP_AGENT_ADVERTISE_HOST: the hostname or IP
// agents will actually dial to reach this control plane, which has to
// be a name/address the gRPC server's own TLS certificate is issued
// for, or every agent's TLS verification against it fails. Defaults to
// "127.0.0.1", correct only for local/single-machine testing; a real
// multi-node deployment (TASKS.md 3.3 onward) needs this set to the
// control plane's actual reachable address.
func agentAdvertiseHost() string {
	host := os.Getenv("APP_AGENT_ADVERTISE_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	return host
}

// telemetryTargets lists every desired service's currently running
// container(s) as collection targets, re-derived from the store and
// runtime fresh on every call: the same level-triggered, no-cached-state
// principle dynamicSource already applies to reconcile controllers,
// applied here to "what should be collected." Uses runtime.ListByPrefix
// (already part of docker.Runtime) rather than reaching into
// internal/reconcile/application's private container-naming function,
// keeping this package decoupled from that one's internals: a service
// converged to steady state has exactly one running container under its
// service-name prefix, per that package's own blue-green design.
func telemetryTargets(db *store.DB, runtime docker.Runtime) func(context.Context) ([]telemetry.Target, error) {
	return func(ctx context.Context) ([]telemetry.Target, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}

		var targets []telemetry.Target
		for _, svc := range services {
			containers, err := runtime.ListByPrefix(ctx, svc.Name+"-")
			if err != nil {
				return nil, fmt.Errorf("list containers for %s: %w", svc.Name, err)
			}
			for _, c := range containers {
				if !c.Running {
					continue
				}
				targets = append(targets, telemetry.Target{
					ResourceID:  "service:" + svc.Name,
					ContainerID: c.ID,
				})
			}
		}
		return targets, nil
	}
}

// logTargets lists every desired service's currently running container(s)
// as log collection targets. A deliberate near-duplicate of
// telemetryTargets above rather than a shared helper: internal/telemetry.
// LogTarget and telemetry.Target are distinct types for distinct
// collector shapes (streaming vs polling, see logs.go's own doc comment
// on why the two collectors differ), and this mirrors the same
// "deliberate near-duplicate, not a shared package, while both pieces
// are still under active development" choice migrate.go's own doc
// comment already makes for internal/telemetry vs internal/store.
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
				if !c.Running {
					continue
				}
				targets = append(targets, telemetry.LogTarget{
					ResourceID:  "service:" + svc.Name,
					ContainerID: c.ID,
				})
			}
		}
		return targets, nil
	}
}

// runLogsRetentionSweep deletes log entries older than the configured
// retention window on a fixed interval, until ctx is done. Mirrors
// runMetricsRetentionSweep's shape exactly; see that function's doc
// comment for why retention duration is env-configurable
// (APP_LOGS_RETENTION here) while the sweep interval itself is not.
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

// runMetricsRetentionSweep deletes samples older than the configured
// retention window on a fixed interval, until ctx is done. Retention
// duration is env-configurable (APP_METRICS_RETENTION, a Go duration
// string like "360h"), following the project's "no hardcoded
// thresholds, use env vars" rule; the sweep interval itself is not, see
// metricsRetentionSweepInterval's doc comment for why that one's fixed.
func runMetricsRetentionSweep(ctx context.Context, db *telemetry.DB, logger *slog.Logger) {
	ticker := time.NewTicker(metricsRetentionSweepInterval)
	defer ticker.Stop()

	sweep := func() {
		cutoff := time.Now().Add(-metricsRetention())
		deleted, err := db.Retain(ctx, cutoff)
		if err != nil {
			logger.Error("metrics retention sweep failed", slog.String("error", err.Error()))
			return
		}
		if deleted > 0 {
			logger.Info("metrics retention sweep", slog.Int64("deleted", deleted), slog.Time("cutoff", cutoff))
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

func metricsRetention() time.Duration {
	raw := os.Getenv("APP_METRICS_RETENTION")
	if raw == "" {
		return defaultMetricsRetention
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return defaultMetricsRetention
	}
	return d
}

// nodeHeartbeatInterval reads APP_NODE_HEARTBEAT_INTERVAL (TASKS.md
// 3.7), the same env-var-with-default shape as metricsRetention/
// logsRetention above, applied to how often a connected agent's
// last_seen_at is touched (internal/agent.WithHeartbeatInterval).
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

// nodeHeartbeatTimeout reads APP_NODE_HEARTBEAT_TIMEOUT (TASKS.md 3.7),
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

func loadBrand() (*brand.Brand, error) {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = defaultBrandFile
	}
	return brand.Load(path)
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

// loadSecretsManager builds a secrets.Manager, sourcing the master key
// from APP_MASTER_KEY when an operator has set one (the explicit-config
// path production deployments managing their own secret store should
// use), or loading/generating one persisted in dataDir otherwise, the
// same loadOrGenerateAgentCA pattern just above: unlike Coolify's
// separate install-script step (which writes its own key to disk before
// the app process ever starts), this control plane ships as one binary
// with no install step to run that generation in ahead of time, so the
// binary does it itself on first boot instead. Regenerating on every
// restart instead of persisting would make every previously-wrapped DEK
// permanently unwrappable, the one thing this must never do.
func loadSecretsManager(db *store.DB, dataDir string) (*secrets.Manager, error) {
	if serialized := os.Getenv("APP_MASTER_KEY"); serialized != "" {
		mk, err := secrets.LoadMasterKey(serialized)
		if err != nil {
			return nil, fmt.Errorf("load master key from APP_MASTER_KEY: %w", err)
		}
		return secrets.NewManager(db, mk), nil
	}

	mk, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		return nil, err
	}
	return secrets.NewManager(db, mk), nil
}

// loadOrGenerateMasterKey loads a previously persisted master key from
// dataDir, or generates and persists a fresh one if none exists yet.
func loadOrGenerateMasterKey(dataDir string) (*secrets.MasterKey, error) {
	if err := os.MkdirAll(dataDir, 0o750); err != nil { //nolint:gosec // operator-controlled startup config, not user input
		return nil, fmt.Errorf("create data dir for master key: %w", err)
	}
	keyPath := filepath.Join(dataDir, masterKeyFilename)

	if serialized, err := os.ReadFile(keyPath); err == nil { //nolint:gosec // operator-controlled data directory path, not user input
		mk, err := secrets.LoadMasterKey(string(serialized))
		if err != nil {
			return nil, fmt.Errorf("load persisted master key: %w", err)
		}
		return mk, nil
	}

	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		return nil, fmt.Errorf("generate master key: %w", err)
	}
	if err := os.WriteFile(keyPath, []byte(mk.String()), 0o600); err != nil { //nolint:gosec // operator-controlled data directory path, not user input
		return nil, fmt.Errorf("persist master key: %w", err)
	}
	return mk, nil
}

// loadBuilder connects to BuildKit and returns a ready
// internal/deploy.Pipeline plus its closer, independent of any
// git/webhook-specific configuration: this is the shared "can this
// control plane build container images at all" gate both the git
// webhook receiver (loadWebhookHandler) and the manual build trigger
// endpoint (api.WithBuilder, this task's own "a user without a working
// webhook has zero way to build anything" gap) build on. A single shared
// Pipeline/BuildKit connection, not two independent ones, so a control
// plane running both paths does not hold two live BuildKit connections
// for the same purpose.
//
// A real BuildKit connection must succeed: unlike openStore's directory
// default or loadBrand's file default, there is no sensible zero-config
// builder, so any failure returns a plain error rather than partially
// wiring something broken. The caller (run) treats that as non-fatal,
// the same choice it already makes for admin bootstrap and secrets: the
// control plane still starts, only build-dependent paths (git push,
// manual build trigger) are unavailable, and specifically why is in the
// returned error.
//
// The returned closer releases the BuildKit connection and its own raw
// Docker client, distinct from the one docker.NewClient already opened
// for the reconciler: internal/build needs the raw *dockerclient.Client
// moby/buildkit's Go client expects, not internal/docker's Runtime
// wrapper, the same second-client pattern every BuildKit live test in
// this codebase already uses.
//
// agentRegistry is TASKS.md 3.5's build-node routing wiring: db's
// current nodes are checked for AcceptsBuildWorkloads
// (migrations/0010_node_workloads.sql), and build.SelectBuildNode picks
// among the ones currently reachable through agentRegistry (TASKS.md
// 3.1's transport). See checkLocalBuildNode's own doc comment for why a
// selected non-local node makes this function fail loudly rather than
// silently building locally: actually dispatching a build to a remote
// node isn't wired yet (internal/build/node.go's package doc comment
// has the full "why not" and what would need to change), and that
// refusal applies the same way regardless of which HTTP path
// eventually triggers a build.
func loadBuilder(ctx context.Context, logger *slog.Logger, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, agentRegistry *agent.Registry) (*deploy.Pipeline, func() error, error) {
	if err := checkLocalBuildNode(ctx, db, agentRegistry, logger); err != nil {
		return nil, nil, fmt.Errorf("select build node: %w", err)
	}

	rawDockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, nil, fmt.Errorf("new docker client for buildkit: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawDockerCli, buildCacheOptions()...)
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

	// build.type: static (static sites are served by the embedded ingress
	// directly, with no container) needs a durable directory to
	// copy site output into, distinct from BuildKit's own image build:
	// staticSitesDir lives under this control plane's own data directory,
	// the same "operator-controlled startup config, not user input"
	// dataDir every other on-disk state in this file already resolves,
	// not the ephemeral checkout dir internal/webhook removes the moment
	// Deploy returns (see static.go's deployStatic doc comment for why
	// serving straight from that checkout would break the instant the
	// webhook handler's own deferred cleanup runs).
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
		deploy.WithAppStore(db),
	}
	if secretsManager != nil {
		deployOpts = append(deployOpts, deploy.WithSecretChecker(secretsManager))
	}
	return deploy.New(buildClient, db, deployOpts...), closer, nil
}

// loadWebhookHandler builds the TASKS.md 1.5 git webhook receiver, wired
// to pipeline (loadBuilder's result): the one piece of the deploy chain
// no earlier pass connected to cmd/levelrail, per TASKS.md 1.7's own
// "known gap, honestly out of scope" note.
//
// Every input is required (APP_GIT_REPO_URL, APP_WEBHOOK_SECRET,
// APP_IMAGE_REPO, a discoverable app spec, and a non-nil pipeline), so
// any missing piece returns a plain error rather than partially wiring
// something broken; run treats that as non-fatal, same as loadBuilder's
// own failure.
//
// webhook.Config is single-app per its own package doc comment (the
// project's Phase 1 exit criterion is one app deploying from one push), so
// APP_SERVICE_NAME only needs setting when the app spec declares more
// than one service; with exactly one, it's the unambiguous default.
// db and recorder back this task's deploy-attempt tracking
// (webhook.AttemptStore and *deploylog.Recorder respectively): db is
// always the real store (never nil here, unlike pipeline), so the
// webhook handler this function builds always records real deploy
// history for a triggering push, per
// docs-local/research/deploy-attempt-id-and-log-persistence.md's own
// framing that an unattended webhook deploy is exactly the case
// persistence matters most for.
func loadWebhookHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, recorder *deploylog.Recorder, notifier *alerting.DeployDispatcher, pipeline *deploy.Pipeline) (http.Handler, error) {
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
	// specPath is built from an operator-controlled directory (env var
	// or the fixed "." default) and a fixed candidate-filename list
	// (spec.DiscoverPath), not attacker-controlled request input, the
	// same reasoning openStore's gosec exemption above already applies.
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
	return webhook.New(cfg, pipeline, db, recorder, notifier, logger), nil
}

// buildCacheOptions reads TASKS.md 3.5's cache-backend env vars and
// turns whichever are set into build.Options for build.NewClient.
// APP_BUILD_CACHE_DIR wires build.WithCacheDir (Phase 1's local
// backend, still useful for a single build node with no registry
// available); APP_BUILD_CACHE_REGISTRY wires build.WithCacheRegistry,
// the actual "remote cache shared across dedicated build nodes" the
// build architecture calls for, optionally with
// APP_BUILD_CACHE_REGISTRY_INSECURE=true for a plain-HTTP or self-
// signed registry. Neither set (the default) returns no options at
// all, matching every other optional env-var-gated feature in this
// file (secrets, webhook): a control plane with no cache backend
// configured still builds correctly, just without cache reuse.
func buildCacheOptions() []build.Option {
	var opts []build.Option
	if dir := os.Getenv("APP_BUILD_CACHE_DIR"); dir != "" {
		opts = append(opts, build.WithCacheDir(dir))
	}
	if ref := os.Getenv("APP_BUILD_CACHE_REGISTRY"); ref != "" {
		opts = append(opts, build.WithCacheRegistry(ref))
		if os.Getenv("APP_BUILD_CACHE_REGISTRY_INSECURE") == "true" {
			opts = append(opts, build.WithCacheRegistryInsecure())
		}
	}
	return opts
}

// checkLocalBuildNode is TASKS.md 3.5's build-node routing gate: it
// looks at every node's AcceptsBuildWorkloads flag
// (migrations/0010_node_workloads.sql) and, via
// internal/build.SelectBuildNode, decides which node a build should
// run on.
//
// Two outcomes let loadBuilder proceed exactly as it always has,
// building against this control plane's own local BuildKit connection:
// no node is marked build-capable at all (the default, zero-configuration
// case every deployment already had), which returns a nil error here.
//
// A third outcome does not: SelectBuildNode picking a real, reachable,
// build-capable node. That's the case this function refuses, loudly,
// rather than silently building locally instead or pretending to
// dispatch somewhere it can't reach: actually running a build on a
// remote node needs a way to open a BuildKit connection against that
// node's Docker daemon, and today's agent.Transport (TASKS.md 3.1/3.2)
// only carries docker.Runtime's container-operation surface, not a raw
// Docker Engine API connection. Extending that wire protocol is
// explicitly out of scope for this task (TASKS.md's Phase 3 sequencing
// note: "extends internal/build, doesn't touch the reconciler or
// transport"); see internal/build/node.go's package doc comment for the
// full reasoning. An operator who marks a node build-capable today gets
// a clear, specific error here (surfaced as run's usual "webhook not
// configured" warning, non-fatal to control-plane startup) instead of
// deploys that quietly keep landing on the control plane's own
// resources.
func checkLocalBuildNode(ctx context.Context, db *store.DB, agentRegistry *agent.Registry, logger *slog.Logger) error {
	nodes, err := db.ListNodes(ctx)
	if err != nil {
		return fmt.Errorf("list nodes: %w", err)
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

	selected, err := build.SelectBuildNode(infos)
	if err != nil {
		return fmt.Errorf("no build-capable node is reachable: %w", err)
	}
	if selected == "" {
		return nil
	}

	logger.Warn("build node selected but remote build dispatch is not implemented yet, refusing to start the webhook handler",
		slog.String("node_id", selected))
	return fmt.Errorf("node %q is marked build-capable, but dispatching a build to a remote node isn't implemented yet (TASKS.md 3.5); unmark it via PUT /api/v1/nodes/%s/workloads or wait for remote build dispatch to land", selected, selected)
}

// rootHandler combines the TASKS.md 1.9 HTTP API with the TASKS.md 1.10
// frontend into one *http.Server handler: "/api/" (a subtree pattern,
// so the full original path reaches api's own mux unchanged, matching
// its routes' own "/api/v1/..." patterns) goes to the API, everything
// else to web.Handler's embedded frontend with SPA fallback. Combining
// them here rather than running two servers keeps the control plane's
// single-binary story intact: one process, one listen address.
//
// webhook.Handler mounts at POST /webhook, outside the /api/ subtree:
// GitHub calls this URL directly, it is not part of the versioned
// platform API surface the frontend and future MCP layer build on.
//
// telemetryDB and alertingDB are always non-nil (openTelemetryStore and
// openAlertingStore have no optional gating, unlike secrets/webhook
// below), so api.WithTelemetryQuerier and api.WithAlertRules are both
// applied unconditionally. telemetryDB is wrapped in a fresh
// telemetry.NewLocalFederator per TASKS.md 2.3's federated-shape design
// (today: exactly one source, this node's own local store); alertingDB
// itself already satisfies api.AlertRules structurally, no wrapper
// needed.
//
// secretsManager, webhookHandler, and builder may all be nil
// (APP_MASTER_KEY, the webhook env vars, and a working BuildKit
// connection are each independently optional): api.WithSecretSetter,
// the /webhook mount, and api.WithBuilder are only applied when set,
// since a nil *secrets.Manager wrapped in a non-nil api.SecretSetter
// interface value would panic the first time PUT .../secrets/{key}
// tried to call a method on it, rather than hitting api.Router's own
// "not configured" 501 path; api.Builder has the identical nil-interface
// hazard, so builder gets the same explicit nil check rather than being
// passed to api.WithBuilder unconditionally. Backup targets
// (api.WithBackupSecrets, api.WithBackupRunner) share this exact
// dependency on secretsManager and get the same nil check: a backup
// target's credentials go through internal/secrets the same as any app
// secret, so there is nowhere to store or resolve them without a master
// key either.
//
// client is always non-nil here: run() returns early on a
// docker.NewClient error, before rootHandler is ever called, so unlike
// secretsManager/webhookHandler/builder above, api.WithDockerPinger,
// api.WithImageLister, api.WithDockerDiskUsager, api.WithDockerPruner,
// and api.WithExecRuntime are all applied unconditionally, the same way
// api.WithTelemetryQuerier and api.WithAlertRules already are for
// telemetryDB/alertingDB. agentRegistry, WithExecRuntime's other
// dependency, is likewise always non-nil: run() constructs it
// unconditionally before rootHandler is ever called (a single-node
// deployment simply never has anything registered in it beyond its own
// local node), the same status db/telemetryDB/alertingDB already have.
// deployRecorder and logBroadcaster are both always non-nil (constructed
// unconditionally in run, unlike builder/secretsManager/webhookHandler
// which are each independently optional): api.WithDeployRecorder,
// api.WithDeployLogStore, and api.WithLogBroadcaster are all applied
// unconditionally, the same way api.WithTelemetryQuerier and
// api.WithAlertRules already are for telemetryDB/alertingDB.
// handleTriggerBuild (internal/api/builds.go) still degrades gracefully
// to build.SlogProgress if a *caller* passes a nil recorder in some
// other wiring path (e.g. a test), but production wiring here always has
// a real one.
//
// deployDispatcher (wave-2 roadmap item #5, deploy-outcome
// notifications) is likewise always non-nil here: run() constructs it
// unconditionally from alertingDB, the same way alertingDB itself is
// always non-nil. api.WithDeployNotifyTargets(alertingDB) and
// api.WithDeployNotifier(deployDispatcher) are therefore both applied
// unconditionally too, wiring the plain image-tag and manual-build
// deploy triggers (internal/api/deploys.go, internal/api/builds.go) to
// the same dispatcher already passed to loadWebhookHandler for the
// push-triggered path, so all three real deploy-trigger call sites fire
// through one shared instance.
//
// backupRunner is built by run(), not here, unlike everything else this
// function used to construct inline: wave-2 roadmap item 6's scheduled
// backup loop (also started in run(), see that function's own comment
// just above where it calls backup.NewScheduler) needs the exact same
// *backup.Runner instance this function wires into api.WithBackupRunner,
// the same "construct once, share by reference" reasoning
// deployRecorder/logBroadcaster already follow for their own two-
// consumer dependencies. Its own nil-ness already tracks secretsManager
// (run() only constructs one when secretsManager != nil), so the nil
// check just below covers both in one place instead of two independent
// checks that could drift apart.
// agentRegistry (new parameter, exec's own addition) is threaded
// through unconditionally, the same "always available by the time
// rootHandler is called" status client already has (run() aborts
// startup if docker.NewClient fails, before agentRegistry is even
// constructed): api.WithExecRuntime wires
// resolveNodeTransport(client, agentRegistry, nodeID) in as this
// package's own NodeRuntimeResolver, so handleExecApp (internal/api/
// exec.go) can resolve the same local-or-remote docker.Runtime every
// reconciler controller already resolves per service, without
// internal/api importing internal/agent.Registry directly (see
// api.NodeRuntimeResolver's own doc comment for why this stays a
// closure over resolveNodeTransport instead of a new dependency edge).
func rootHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, alertingDB *alerting.DB, secretsManager *secrets.Manager, webhookHandler http.Handler, client *docker.Client, builder *deploy.Pipeline, deployRecorder *deploylog.Recorder, logBroadcaster *telemetry.LogBroadcaster, deployDispatcher *alerting.DeployDispatcher, backupRunner *backup.Runner, agentRegistry *agent.Registry, emailSender email.Sender) http.Handler {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	opts := []api.Option{
		api.WithTelemetryQuerier(telemetry.NewLocalFederator(telemetryDB)),
		api.WithAlertRules(alertingDB),
		api.WithDeployNotifyTargets(alertingDB),
		api.WithDeployNotifier(deployDispatcher),
		api.WithNotificationChannels(alertingDB),
		api.WithNotificationChannelTester(deployDispatcher),
		api.WithSessionTTL(sessionTTL(logger)),
		api.WithDataDir(dataDir),
		api.WithDockerPinger(client),
		api.WithImageLister(client),
		api.WithDockerDiskUsager(client),
		api.WithDockerPruner(client),
		api.WithExecRuntime(func(nodeID string) (docker.Runtime, error) {
			return resolveNodeTransport(client, agentRegistry, nodeID)
		}),
		api.WithCertExpiryWarningWindow(certExpiryWarningWindow(logger)),
		api.WithPublicHost(publicHost()),
		api.WithDeployLogStore(telemetryDB),
		api.WithDeployRecorder(deployRecorder),
		api.WithLogBroadcaster(logBroadcaster),
		// emailSender is always non-nil (run() builds it unconditionally):
		// forgot-password always exists, it just fails clearly at send
		// time when nothing is actually configured.
		api.WithEmailSender(emailSender),
	}
	if secretsManager != nil {
		opts = append(opts, api.WithSecretSetter(secretsManager))
		// Email settings' SMTP password / SES secret access key go
		// through the same secretsManager as everything else here.
		opts = append(opts, api.WithEmailSecrets(secretsManager))
		// Git sources (TASKS.md 1.7's own deferred follow-up,
		// internal/api/git_sources.go, git_webhook.go): a connected
		// source's deploy token and webhook secret go through the same
		// secretsManager as everything else in this block, same nil-
		// interface hazard, same "skipped entirely without a master key"
		// shape.
		opts = append(opts, api.WithGitSourceSecrets(secretsManager))
		// Compose ingestion's generated template secrets (SERVICE_PASSWORD_*
		// etc, internal/compose.ResolveMagicVars): same secretsManager,
		// same nil-interface hazard as everything else in this block.
		opts = append(opts, api.WithComposeSecrets(secretsManager))
		// OAuth provider client secrets (Google/GitHub sign-in): same
		// secretsManager, same nil-interface hazard as everything else in
		// this block.
		opts = append(opts, api.WithOAuthSecrets(secretsManager))
		// Backup targets and running a real backup both need
		// secretsManager: creating a target writes its credentials
		// through it (api.WithBackupSecrets), and actually running a
		// backup reads them back through the identical *secrets.Manager
		// value (internal/backup.Runner.Secrets), the one internal/api
		// deliberately never does itself, see BackupSecretsSetter's own
		// doc comment for why that split exists. Same nil-interface
		// hazard as api.WithSecretSetter above: both are skipped
		// entirely, not passed with a nil secretsManager inside a
		// non-nil interface value, whenever no master key is configured.
		opts = append(opts,
			api.WithBackupSecrets(secretsManager),
			// Registry credentials (build.type: image's optional
			// registryCredential field): same secretsManager, same
			// nil-interface hazard as everything else in this block.
			api.WithRegistryCredentialSecrets(secretsManager),
			api.WithBackupRunner(backupRunner),
			// The restore counterpart of the backup runner just above:
			// same secretsManager dependency (a restore resolves the same
			// stored target credentials a backup wrote), same nil check,
			// wired from the same client (this control plane's own node's
			// docker.Runtime, since ContainerRestorer needs
			// ExecWithInput the same way ContainerDumper above needs
			// Exec).
			api.WithRestoreRunner(&backup.RestoreRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: backup.S3Downloader{},
				Restorer:   &backup.ContainerRestorer{Runtime: client},
			}),
			// The plain-download counterpart of the restore runner above:
			// same secretsManager dependency and same backup.S3Downloader,
			// but hands the object stream straight back to internal/api
			// instead of piping it into a container's stdin.
			api.WithBackupDownloader(&backup.DownloadRunner{
				Store:      db,
				Secrets:    secretsManager,
				Downloader: backup.S3Downloader{},
			}),
			// GitHub App connection: reads and writes through
			// secretsManager directly rather than a separate runner
			// type, see api.GitHubAppSecrets's own doc comment for why.
			// Same nil-interface hazard as every other
			// secretsManager-dependent option in this block.
			api.WithGitHubAppSecrets(secretsManager),
			// Per-user TOTP secrets (internal/api/twofactor.go): same
			// secretsManager, same nil-interface hazard as everything else
			// in this block.
			api.WithTwoFactorSecrets(secretsManager),
			// GitLab App connection: same secretsManager, same
			// nil-interface hazard, the OAuth-Application counterpart of
			// the GitHub App connection just above.
			api.WithGitLabAppSecrets(secretsManager),
		)
	}
	if builder != nil {
		opts = append(opts, api.WithBuilder(builder))
	}
	if manifestCfg, err := loadGitHubAppManifestConfig(); err != nil {
		logger.Error("load github app manifest config failed, using defaults", slog.String("error", err.Error()))
	} else {
		opts = append(opts, api.WithGitHubAppManifestConfig(manifestCfg))
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewRouter(logger, b, db, opts...).Handler())
	if webhookHandler != nil {
		mux.Handle("POST /webhook", webhookHandler)
	}
	mux.Handle("/", web.Handler())
	return mux
}

// backupSchedulerInterval reads APP_BACKUP_SCHEDULER_INTERVAL as a Go
// duration string, the project's "no hardcoded thresholds, use env
// vars" rule applied to how often internal/backup.Scheduler checks
// which databases have a due cron schedule. Returns
// defaultBackupSchedulerInterval (see that constant's own doc comment
// for why 1 minute) when unset or unparseable, logging a warning in the
// latter case, the same shape sessionTTL/certExpiryWarningWindow already
// use for their own env vars. Unlike those two (which return 0 as a
// signal for api.Router to apply its own internal default),
// backup.Scheduler.Run takes interval directly with no built-in
// fallback of its own, so this function resolves the real value once,
// here, rather than needing two copies of the same default to stay in
// sync.
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

func httpAddr() string {
	addr := os.Getenv("APP_HTTP_ADDR")
	if addr == "" {
		addr = defaultHTTPAddr
	}
	return addr
}

// dashboardDialAddr normalizes httpAddr's listen address (e.g. ":8080",
// the normal "listen on every interface" shorthand a *http.Server.Addr
// takes) into a loopback dial address (e.g. "127.0.0.1:8080") the
// embedded Caddy instance can reverse-proxy to, the same
// "127.0.0.1:" + port normalization internal/reconcile/ingress's own
// routeFor already applies to a service container's published host
// port, for the identical reason given there: Caddy and this control
// plane's own HTTP server share one process and one loopback interface,
// so a listen-on-every-interface bind address is never itself a valid
// single dial target. An address with an explicit, non-empty host (an
// operator who set APP_HTTP_ADDR to something more specific than
// ":port") is passed through unchanged rather than overridden, since
// that host was chosen deliberately. Returns "" (WithDashboardDial's own
// "no dashboard route" default) if addr doesn't parse as host:port at
// all, rather than guessing.
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

// sessionTTL reads APP_SESSION_TTL as a Go duration string (e.g. "24h",
// "30m"), following the project's "no hardcoded thresholds, use env
// vars" rule for the value api.WithSessionTTL configures. Returns 0
// (api.NewRouter's own signal to fall back to its internal default)
// when unset or unparseable, logging a warning in the latter case so a
// typo'd env var is visible rather than silently ignored.
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

// certExpiryWarningWindow reads APP_CERT_EXPIRY_WARNING_WINDOW as a Go
// duration string, the same env-var-with-default shape sessionTTL above
// already uses for api.WithSessionTTL, applied here to
// api.WithCertExpiryWarningWindow (GET /api/v1/certificates's
// "expiring_soon" threshold). Returns 0 (api's own signal to fall back
// to its internal default, api.defaultCertExpiryWarningWindow) when
// unset or unparseable, logging a warning in the latter case so a
// typo'd env var is visible rather than silently ignored.
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

// publicHost reads APP_PUBLIC_HOST: the IP address or hostname operators
// should point a DNS A record at to reach this control plane's embedded
// ingress (internal/ingress), surfaced through GET
// /api/v1/apps/{name}/domains/{domain}/check (api.WithPublicHost) so
// DomainEditor can tell an operator what to actually type at their
// registrar. No default: this control plane cannot reliably discover its
// own public-facing address on its own (NAT, a container port mapping, a
// cloud load balancer all break self-discovery), so an unset value is a
// real "not configured, guessing from the request" state
// (api.handleCheckDomain's own advertisedHost fallback), not a value to
// invent one for here.
func publicHost() string {
	return strings.TrimSpace(os.Getenv("APP_PUBLIC_HOST"))
}

// dynamicSource builds a reconcile.Source that re-lists desired
// services/databases/nodes from the store on every call, level-triggered
// by construction. The ingress controller is deliberately not routed
// through per-node placement: it stays on the local runtime
// unconditionally, since Caddy runs embedded in this one process and a
// remote node's container isn't reachable at 127.0.0.1:hostPort from
// here (closing this needs the WireGuard mesh, TASKS.md 3.4).
//
// dynamicSourceDeps bundles dynamicSource's wiring dependencies: every
// field here is fixed for the process lifetime, only the store contents
// dynamicSource reads change between reconcile passes.
type dynamicSourceDeps struct {
	db               *store.DB
	runtime          docker.Runtime
	driver           *ingressdriver.Driver
	logger           *slog.Logger
	telemetryDB      *telemetry.DB
	secretsManager   *secrets.Manager
	agentRegistry    *agent.Registry
	heartbeatTimeout time.Duration
	meshCfg          *meshSetup
	meshDNSAddr      string
	dashboardDial    string
	networkPrefix    string
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

		controllers := make([]reconcile.Controller, 0, len(services)+len(databases)+len(nodes)+2)
		controllers = append(controllers, appControllersFor(deps, services)...)
		controllers = append(controllers, databaseControllersFor(ctx, deps, databases)...)
		for _, n := range nodes {
			controllers = append(controllers, nodehealth.New(n.ID, deps.db, deps.heartbeatTimeout))
		}

		ingressOpts := []ingressreconcile.Option{ingressreconcile.WithLogger(deps.logger)}
		if deps.dashboardDial != "" {
			ingressOpts = append(ingressOpts, ingressreconcile.WithDashboardDial(deps.dashboardDial))
		}
		controllers = append(controllers, ingressreconcile.New(deps.db, deps.runtime, deps.driver, ingressOpts...))

		// Local runtime unconditionally, same reasoning as the ingress
		// controller above: per-app networks are single-node scope until
		// the WireGuard mesh (TASKS.md 3.4) exists.
		controllers = append(controllers, application.NewNetworkCleanupController(deps.db, deps.runtime, deps.networkPrefix))

		if deps.meshCfg != nil {
			controllers = append(controllers, meshreconcile.New(deps.meshCfg.localNodeID, deps.db, deps.meshCfg.coordinator, deps.meshCfg.resolver, meshreconcile.WithLogger(deps.logger)))
		}
		return controllers, nil
	}
}

// appControllersFor builds one application.Controller per service,
// skipping (with a warning) any whose node isn't currently reachable.
func appControllersFor(deps dynamicSourceDeps, services []store.DesiredService) []reconcile.Controller {
	// application.WithStorageTargets is unconditional, unlike
	// WithSecretResolver below: resolving a StorageTarget's
	// endpoint/region/bucket needs no master key, only its access keys
	// do. A service with a StorageTargetID but no secretsManager still
	// fails Reconcile loudly rather than starting half-configured.
	appOpts := []application.Option{
		application.WithDeployRecorder(deps.telemetryDB),
		application.WithStorageTargets(deps.db),
		application.WithRegistryCredentials(deps.db),
		application.WithProjectEnv(deps.db),
		application.WithNetworkPrefix(deps.networkPrefix),
	}
	if deps.secretsManager != nil {
		appOpts = append(appOpts, application.WithSecretResolver(deps.secretsManager))
	}
	if deps.meshDNSAddr != "" {
		appOpts = append(appOpts, application.WithMeshDNSAddr(deps.meshDNSAddr))
	}

	controllers := make([]reconcile.Controller, 0, len(services))
	for _, svc := range services {
		svcRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, svc.NodeID)
		if err != nil {
			deps.logger.Warn("skipping service for this reconcile pass: node transport unavailable",
				slog.String("service", svc.Name), slog.String("node_id", svc.NodeID), slog.String("error", err.Error()))
			continue
		}
		controllers = append(controllers, application.New(svc.Name, deps.db, svcRuntime, appOpts...))
	}
	return controllers
}

// databaseControllersFor builds one database.Controller per database,
// skipping (with a warning) any whose node isn't currently reachable. A
// database whose engine credentials can't be resolved this pass still
// gets a controller, just without WithPostgresCredentials/
// WithMySQLCredentials/WithMongoDBCredentials set: one broken resource
// must not block others, and it retries fresh on the next pass.
func databaseControllersFor(ctx context.Context, deps dynamicSourceDeps, databases []store.DesiredDatabase) []reconcile.Controller {
	controllers := make([]reconcile.Controller, 0, len(databases))
	for _, desired := range databases {
		dbRuntime, err := resolveNodeTransport(deps.runtime, deps.agentRegistry, desired.NodeID)
		if err != nil {
			deps.logger.Warn("skipping database for this reconcile pass: node transport unavailable",
				slog.String("database", desired.Name), slog.String("node_id", desired.NodeID), slog.String("error", err.Error()))
			continue
		}
		dbOpts := databaseCredentialOpts(ctx, deps, desired)
		if deps.meshDNSAddr != "" {
			dbOpts = append(dbOpts, database.WithMeshDNSAddr(deps.meshDNSAddr))
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
	case store.EngineMySQL:
		creds, err := mysqlCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mysql credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMySQLCredentials(creds))
		}
	case store.EngineMongoDB:
		creds, err := mongoCredentialsFor(ctx, deps.secretsManager, desired.Name)
		if err != nil {
			deps.logger.Warn("skipping mongodb credentials for this reconcile pass",
				slog.String("database", desired.Name), slog.String("error", err.Error()))
		} else if creds != nil {
			opts = append(opts, database.WithMongoDBCredentials(creds))
		}
	}
	return opts
}

// resolveNodeTransport picks the docker.Runtime a controller for
// nodeID should use: the control plane's own local runtime for the
// empty string (TASKS.md 3.3's "" == local node convention), or a
// connected remote agent's Transport (TASKS.md 3.2), looked up fresh
// from registry, for anything else. A node that isn't currently
// connected returns agent.ErrNodeNotRegistered wrapped; the caller logs
// and skips that resource for this pass rather than failing the whole
// reconcile (dynamicSource's own doc comment on "one broken resource
// must not block others," applied here to node connectivity): a
// temporarily disconnected node's services simply don't reconcile this
// tick, and pick back up automatically once it reconnects
// (Server.Session re-registers on every new connection), not
// permanently broken.
func resolveNodeTransport(local docker.Runtime, registry *agent.Registry, nodeID string) (docker.Runtime, error) {
	if nodeID == "" {
		return local, nil
	}
	return registry.Get(nodeID)
}

// bootstrapAdmin creates the single admin account from
// APP_ADMIN_USERNAME/APP_ADMIN_PASSWORD if none exists yet. See
// api.BootstrapAdmin's doc comment for why this is safe to call on every
// startup.
func bootstrapAdmin(ctx context.Context, db *store.DB) error {
	username := os.Getenv("APP_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("APP_ADMIN_PASSWORD")
	return api.BootstrapAdmin(ctx, db, username, password)
}
