// Command levelrail is the control plane binary. It starts the TASKS.md
// 1.9 HTTP API and a reconcile engine whose controller set is derived
// dynamically from desired state in the store every pass (see
// dynamicSource): one application.Controller per desired service, one
// database.Controller per desired database, plus a single ingress
// controller routing every service with domains. Everything else in
// CLAUDE.md 4 (multi-node, WireGuard, MCP) lands in later phases.
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
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/webhook"
	"github.com/GLINCKER/levelrail/web"
)

const (
	resyncInterval = 30 * time.Second
	// defaultDataDir matches CLAUDE.md section 3's APP_DATA_DIR override
	// pattern. Relative, not /var/lib/levelrail-data (brand.yaml's noted
	// pending path), since Phase 0 runs locally, not installed as a
	// service yet.
	defaultDataDir = "./data"
	// defaultBrandFile is the same "relative default, env override"
	// pattern as defaultDataDir, per CLAUDE.md section 3.
	defaultBrandFile = "./brand.yaml"
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

	// metricsCollectionInterval matches CLAUDE.md 4.8's "15s resolution."
	// Not env-configurable like retention below: resolution changes the
	// shape of every stored sample going forward, retention only trims
	// how much of that shape is kept, a materially smaller blast radius
	// for an operator to tune without redesigning the schema.
	metricsCollectionInterval = 15 * time.Second
	// defaultMetricsRetention matches CLAUDE.md 4.8's "default 15 days."
	defaultMetricsRetention = 15 * 24 * time.Hour
	// metricsRetentionSweepInterval: how often the retention sweep runs,
	// not how long data is kept (that's the retention duration itself).
	// Hourly is frequent enough that the table never grows much past its
	// steady-state size, infrequent enough to be a non-event on a
	// control plane CLAUDE.md 6 Phase 5 wants measured for idle cost.
	metricsRetentionSweepInterval = 1 * time.Hour

	// logTargetsResyncInterval is how often the log collector re-derives
	// which containers it should be streaming from (TASKS.md 2.2), kept
	// as its own constant rather than reusing resyncInterval: it governs
	// a different concern (log stream subscriptions, not reconcile
	// passes) even though the two currently share the same value.
	logTargetsResyncInterval = 30 * time.Second
	// defaultLogsRetention matches CLAUDE.md 4.8's "default 15 days,"
	// the same default metrics uses, applied to the sibling log store.
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
	agentpb.RegisterAgentServiceServer(agentGRPCServer, agent.NewServer(agentCA, db, agentRegistry, logger))
	go func() {
		logger.Info("agent grpc listening", slog.String("addr", agentListener.Addr().String()))
		if err := agentGRPCServer.Serve(agentListener); err != nil {
			logger.Error("agent grpc server stopped", slog.String("error", err.Error()))
		}
	}()
	defer agentGRPCServer.GracefulStop()

	secretsManager, err := loadSecretsManager(db)
	if err != nil {
		// Not fatal, the same choice bootstrapAdmin makes above: the
		// control plane still starts. Every route and reconcile path
		// touching secrets (PUT .../secrets/{key}, any service declaring
		// a { secret: true } env var) fails loudly and specifically
		// instead, rather than this refusing to start at all over a
		// feature an operator may not be using yet.
		logger.Warn("secrets not configured", slog.String("error", err.Error()))
	}

	webhookHandler, closeWebhook, err := loadWebhookHandler(ctx, logger, b, db, telemetryDB, secretsManager)
	if err != nil {
		// Not fatal, the same choice as everything else optional above:
		// the control plane still starts, serving apps deployed by
		// hand through the HTTP API. Only the git-push path is
		// unavailable, and specifically why is right here in the log.
		logger.Warn("webhook not configured", slog.String("error", err.Error()))
	}
	if closeWebhook != nil {
		defer func() {
			if cerr := closeWebhook(); cerr != nil {
				logger.Error("closing webhook build client", slog.String("error", cerr.Error()))
			}
		}()
	}

	httpServer := &http.Server{
		Addr:              httpAddr(),
		Handler:           rootHandler(logger, b, db, telemetryDB, alertingDB, secretsManager, webhookHandler, client),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ingressDriver := ingressdriver.New(logger)
	defer func() {
		if cerr := ingressDriver.Stop(context.Background()); cerr != nil {
			logger.Error("stopping ingress driver", slog.String("error", cerr.Error()))
		}
	}()

	engine := reconcile.NewEngine(logger)
	engine.SetStore(db)
	engine.SetSource(dynamicSource(db, client, ingressDriver, logger, telemetryDB, secretsManager, agentRegistry))

	collector := telemetry.NewCollector(client, telemetryDB, metricsCollectionInterval, logger)
	go func() {
		if err := collector.Run(ctx, telemetryTargets(db, client)); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("telemetry collector stopped", slog.String("error", err.Error()))
		}
	}()
	go runMetricsRetentionSweep(ctx, telemetryDB, logger)

	logCollector := telemetry.NewLogCollector(client, telemetryDB, logger)
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
	smtpCfg := smtpConfigFromEnv()
	alertingNewNotifier := func(r alerting.Rule) alerting.Notifier { return alerting.NewNotifier(nil, smtpCfg, r) }
	alertingEngine := alerting.NewEngine(alertingDB, alertingFederator, alertingFederator, restartTracker, alertingNewNotifier, logger)
	go func() {
		if err := alertingEngine.Run(ctx, alertEvaluationInterval); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("alerting engine stopped", slog.String("error", err.Error()))
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
// APP_SMTP_PASSWORD/APP_SMTP_FROM for TASKS.md 2.5's email notification
// channel. There is no sensible zero-config default for an SMTP server
// (unlike, say, APP_DATA_DIR), so returning nil (both APP_SMTP_HOST and
// APP_SMTP_FROM unset) is a valid, expected outcome, not an error:
// alerting.NewNotifier's own doc comment covers what an email-kind rule
// does with a nil *alerting.SMTPConfig (fails clearly, every time,
// naming exactly what to set). APP_SMTP_PORT defaults to 587
// (STARTTLS), the common submission port, not 25.
func smtpConfigFromEnv() *alerting.SMTPConfig {
	host := os.Getenv("APP_SMTP_HOST")
	from := os.Getenv("APP_SMTP_FROM")
	if host == "" || from == "" {
		return nil
	}
	port := os.Getenv("APP_SMTP_PORT")
	if port == "" {
		port = "587"
	}
	return &alerting.SMTPConfig{
		Addr:     host + ":" + port,
		Host:     host,
		Username: os.Getenv("APP_SMTP_USERNAME"),
		Password: os.Getenv("APP_SMTP_PASSWORD"),
		From:     from,
	}
}

// agentCACertFilename/agentCAKeyFilename are TASKS.md 3.2's private CA
// (internal/agent.CA), persisted alongside levelrail.db/telemetry.db/
// alerting.db in the same data directory, the same per-concern
// separation ADR 009 already applies to this control plane's other
// SQLite files, applied here to a different kind of state.
const (
	agentCACertFilename = "agent-ca.crt.pem"
	agentCAKeyFilename  = "agent-ca.key.pem"
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
// string like "360h"), per the root CLAUDE.md's "no hardcoded
// thresholds" rule; the sweep interval itself is not, see
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

func loadBrand() (*brand.Brand, error) {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = defaultBrandFile
	}
	return brand.Load(path)
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

// loadSecretsManager builds a secrets.Manager from APP_MASTER_KEY (CLAUDE.md
// 4.10: "Master key can be sourced from file, env, or an external KMS
// interface added later"; env is what this control plane supports today).
// An unset APP_MASTER_KEY is not an error: it returns (nil, nil), the
// same "feature just isn't configured" shape openStore's directory
// default and loadBrand's file default don't need, because unlike those,
// secrets have no sensible zero-config default to fall back to. A
// generated-on-the-fly key would make every previously-wrapped DEK
// permanently unwrappable on the next restart, worse than not offering
// the feature at all.
func loadSecretsManager(db *store.DB) (*secrets.Manager, error) {
	serialized := os.Getenv("APP_MASTER_KEY")
	if serialized == "" {
		return nil, fmt.Errorf("APP_MASTER_KEY not set")
	}
	mk, err := secrets.LoadMasterKey(serialized)
	if err != nil {
		return nil, fmt.Errorf("load master key: %w", err)
	}
	return secrets.NewManager(db, mk), nil
}

// loadWebhookHandler builds the TASKS.md 1.5 git webhook receiver,
// wired to a real build via TASKS.md 1.4's internal/deploy.Pipeline: the
// one piece of the deploy chain no earlier pass connected to
// cmd/levelrail, per TASKS.md 1.7's own "known gap, honestly out of
// scope" note.
//
// Every input is required (APP_GIT_REPO_URL, APP_WEBHOOK_SECRET,
// APP_IMAGE_REPO, and a discoverable app spec), and a real BuildKit
// connection must succeed: unlike openStore's directory default or
// loadBrand's file default, there is no sensible zero-config webhook,
// so any missing piece returns a plain error rather than partially
// wiring something broken. The caller (run) treats that as non-fatal,
// the same choice it already makes for admin bootstrap and secrets:
// the control plane still starts, only the git-push path is
// unavailable, and specifically why is in the returned error.
//
// webhook.Config is single-app per its own package doc comment (CLAUDE.md
// Phase 1's exit criterion is one app deploying from one push), so
// APP_SERVICE_NAME only needs setting when the app spec declares more
// than one service; with exactly one, it's the unambiguous default.
//
// The returned closer releases the BuildKit connection and its own raw
// Docker client, distinct from the one docker.NewClient already opened
// for the reconciler: internal/build needs the raw *dockerclient.Client
// moby/buildkit's Go client expects, not internal/docker's Runtime
// wrapper, the same second-client pattern every BuildKit live test in
// this codebase already uses.
func loadWebhookHandler(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, secretsManager *secrets.Manager) (http.Handler, func() error, error) {
	repoURL := os.Getenv("APP_GIT_REPO_URL")
	webhookSecret := os.Getenv("APP_WEBHOOK_SECRET")
	imageRepo := os.Getenv("APP_IMAGE_REPO")
	if repoURL == "" || webhookSecret == "" || imageRepo == "" {
		return nil, nil, fmt.Errorf("APP_GIT_REPO_URL, APP_WEBHOOK_SECRET, and APP_IMAGE_REPO must all be set")
	}

	specDir := os.Getenv("APP_SPEC_DIR")
	if specDir == "" {
		specDir = "."
	}
	specPath, err := spec.DiscoverPath(specDir, strings.ToLower(b.BinaryName))
	if err != nil {
		return nil, nil, fmt.Errorf("discover app spec: %w", err)
	}
	// specPath is built from an operator-controlled directory (env var
	// or the fixed "." default) and a fixed candidate-filename list
	// (spec.DiscoverPath), not attacker-controlled request input, the
	// same reasoning openStore's gosec exemption above already applies.
	data, err := os.ReadFile(specPath) //nolint:gosec // operator-controlled startup config, not user input
	if err != nil {
		return nil, nil, fmt.Errorf("read app spec %s: %w", specPath, err)
	}
	parsed, err := spec.Parse(data)
	if err != nil {
		return nil, nil, fmt.Errorf("parse app spec %s: %w", specPath, err)
	}

	serviceName := os.Getenv("APP_SERVICE_NAME")
	if serviceName == "" {
		if len(parsed.Services) != 1 {
			return nil, nil, fmt.Errorf("APP_SERVICE_NAME must be set: app spec %s declares %d services, not exactly 1", specPath, len(parsed.Services))
		}
		for name := range parsed.Services {
			serviceName = name
		}
	}
	svc, ok := parsed.Services[serviceName]
	if !ok {
		return nil, nil, fmt.Errorf("service %q not found in app spec %s", serviceName, specPath)
	}

	rawDockerCli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, nil, fmt.Errorf("new docker client for buildkit: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	buildClient, err := build.NewClient(connectCtx, rawDockerCli)
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

	deployOpts := []deploy.Option{
		deploy.WithBuildMetricsRecorder(telemetryDB),
		deploy.WithLogger(logger),
	}
	if secretsManager != nil {
		deployOpts = append(deployOpts, deploy.WithSecretChecker(secretsManager))
	}
	pipeline := deploy.New(buildClient, db, deployOpts...)

	cfg := webhook.Config{
		Secret:      []byte(webhookSecret),
		RepoURL:     repoURL,
		Branch:      os.Getenv("APP_GIT_BRANCH"),
		ServiceName: serviceName,
		Service:     svc,
		ImageRepo:   imageRepo,
	}
	return webhook.New(cfg, pipeline, logger), closer, nil
}

// rootHandler combines the TASKS.md 1.9 HTTP API with the TASKS.md 1.10
// frontend into one *http.Server handler: "/api/" (a subtree pattern,
// so the full original path reaches api's own mux unchanged, matching
// its routes' own "/api/v1/..." patterns) goes to the API, everything
// else to web.Handler's embedded frontend with SPA fallback. Combining
// them here rather than running two servers keeps CLAUDE.md 4.1's
// single-binary story intact: one process, one listen address.
//
// webhook.Handler mounts at POST /webhook, outside the /api/ subtree:
// GitHub calls this URL directly, it is not part of the versioned
// Levelrail API surface the frontend and future MCP layer build on.
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
// secretsManager and webhookHandler may both be nil (APP_MASTER_KEY and
// the webhook env vars are each independently optional): api.
// WithSecretSetter and the /webhook mount are only applied when set,
// since a nil *secrets.Manager wrapped in a non-nil api.SecretSetter
// interface value would panic the first time PUT .../secrets/{key}
// tried to call a method on it, rather than hitting api.Router's own
// "not configured" 501 path.
//
// client is always non-nil here: run() returns early on a
// docker.NewClient error, before rootHandler is ever called, so unlike
// secretsManager/webhookHandler above, api.WithDockerPinger and
// api.WithImageLister are both applied unconditionally, the same way
// api.WithTelemetryQuerier and api.WithAlertRules already are for
// telemetryDB/alertingDB.
func rootHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, telemetryDB *telemetry.DB, alertingDB *alerting.DB, secretsManager *secrets.Manager, webhookHandler http.Handler, client *docker.Client) http.Handler {
	dataDir := os.Getenv("APP_DATA_DIR")
	if dataDir == "" {
		dataDir = defaultDataDir
	}
	opts := []api.Option{
		api.WithTelemetryQuerier(telemetry.NewLocalFederator(telemetryDB)),
		api.WithAlertRules(alertingDB),
		api.WithSessionTTL(sessionTTL(logger)),
		api.WithDataDir(dataDir),
		api.WithDockerPinger(client),
		api.WithImageLister(client),
	}
	if secretsManager != nil {
		opts = append(opts, api.WithSecretSetter(secretsManager))
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewRouter(logger, b, db, opts...).Handler())
	if webhookHandler != nil {
		mux.Handle("POST /webhook", webhookHandler)
	}
	mux.Handle("/", web.Handler())
	return mux
}

func httpAddr() string {
	addr := os.Getenv("APP_HTTP_ADDR")
	if addr == "" {
		addr = defaultHTTPAddr
	}
	return addr
}

// sessionTTL reads APP_SESSION_TTL as a Go duration string (e.g. "24h",
// "30m"), CLAUDE.md 7's "no hardcoded thresholds, use env vars" rule for
// the value api.WithSessionTTL configures. Returns 0 (api.NewRouter's
// own signal to fall back to its internal default) when unset or
// unparseable, logging a warning in the latter case so a typo'd env var
// is visible rather than silently ignored.
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

// dynamicSource builds a reconcile.Source that re-lists desired services
// and databases from the store on every call and returns one controller
// per resource, plus a single ingress controller. Level-triggered by
// construction, matching every controller it builds: nothing about which
// apps or databases currently exist is cached here, it's re-derived from
// the store every pass, so a service or database created or deleted
// through the HTTP API takes effect on the very next reconcile, no
// restart needed.
//
// secretsManager may be nil (APP_MASTER_KEY unset): application.
// WithSecretResolver is only applied when it isn't, the same nil-check
// rootHandler makes for api.WithSecretSetter and for the identical
// reason. A service with no { secret: true } env vars reconciles
// exactly the same either way; one that declares any fails loudly with
// a clear reason instead of silently starting without the variable.
//
// telemetryDB is always non-nil (openTelemetryStore has no optional
// gating, unlike secretsManager above), so application.WithDeployRecorder
// is applied unconditionally: TASKS.md 2.1's deploy_count metric is
// recorded for every application controller this Source builds.
//
// agentRegistry is TASKS.md 3.3's placement wiring, the first real
// consumer of the Registry TASKS.md 3.1/3.2 built and left otherwise
// unused: each service/database's own NodeID (empty string means this
// control plane's own local node, migrations/0009_node_placement.sql)
// picks which docker.Runtime its controller gets, resolved fresh via
// resolveNodeTransport on every pass, the identical "never cache,
// re-derive every call" principle this function already applies to
// which services/databases exist at all. The ingress controller is
// deliberately NOT routed through this: it stays on the local runtime
// unconditionally, a real, honest, and currently open gap (a service
// placed on a remote node has no working ingress route yet), because
// Caddy runs embedded in this one process (CLAUDE.md 4.5) and a remote
// node's container isn't reachable at 127.0.0.1:hostPort from here even
// if its Transport could be inspected; closing this needs the WireGuard
// mesh and internal DNS TASKS.md 3.4 builds, not something this task
// can honestly solve on its own.
func dynamicSource(db *store.DB, runtime docker.Runtime, driver *ingressdriver.Driver, logger *slog.Logger, telemetryDB *telemetry.DB, secretsManager *secrets.Manager, agentRegistry *agent.Registry) reconcile.Source {
	return func(ctx context.Context) ([]reconcile.Controller, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}
		databases, err := db.ListDesiredDatabases(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired databases: %w", err)
		}

		appOpts := []application.Option{application.WithDeployRecorder(telemetryDB)}
		if secretsManager != nil {
			appOpts = append(appOpts, application.WithSecretResolver(secretsManager))
		}

		controllers := make([]reconcile.Controller, 0, len(services)+len(databases)+1)
		for _, svc := range services {
			svcRuntime, err := resolveNodeTransport(runtime, agentRegistry, svc.NodeID)
			if err != nil {
				logger.Warn("skipping service for this reconcile pass: node transport unavailable",
					slog.String("service", svc.Name), slog.String("node_id", svc.NodeID), slog.String("error", err.Error()))
				continue
			}
			controllers = append(controllers, application.New(svc.Name, db, svcRuntime, appOpts...))
		}
		for _, desired := range databases {
			dbRuntime, err := resolveNodeTransport(runtime, agentRegistry, desired.NodeID)
			if err != nil {
				logger.Warn("skipping database for this reconcile pass: node transport unavailable",
					slog.String("database", desired.Name), slog.String("node_id", desired.NodeID), slog.String("error", err.Error()))
				continue
			}
			var dbOpts []database.Option
			if desired.Engine == store.EnginePostgres {
				creds, err := postgresCredentialsFor(ctx, secretsManager, desired.Name)
				if err != nil {
					// Not fatal to the whole pass: this one Postgres
					// database just stays credentials-blocked for this
					// tick, the same "one broken resource must not
					// block others" principle this function's own doc
					// comment already applies to node connectivity
					// above. It retries on the next pass, since a
					// transient secrets-store failure (unlike a
					// permanently-unconfigured secrets manager, which
					// postgresCredentialsFor itself handles by
					// returning nil, nil) should recover on its own.
					logger.Warn("skipping postgres credentials for this reconcile pass",
						slog.String("database", desired.Name), slog.String("error", err.Error()))
				} else if creds != nil {
					dbOpts = append(dbOpts, database.WithPostgresCredentials(creds))
				}
			}
			controllers = append(controllers, database.New(desired.Name, db, dbRuntime, dbOpts...))
		}
		controllers = append(controllers, ingressreconcile.New(db, runtime, driver, ingressreconcile.WithLogger(logger)))
		return controllers, nil
	}
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
