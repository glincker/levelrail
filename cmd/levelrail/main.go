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
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	dockerclient "github.com/docker/docker/client"

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
	// defaultHTTPAddr is where the TASKS.md 1.9 HTTP API listens.
	defaultHTTPAddr = ":8080"

	httpShutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

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

	webhookHandler, closeWebhook, err := loadWebhookHandler(ctx, logger, b, db, secretsManager)
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
		Handler:           rootHandler(logger, b, db, secretsManager, webhookHandler),
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
	engine.SetSource(dynamicSource(db, client, ingressDriver, logger, secretsManager))

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

func loadBrand() (*brand.Brand, error) {
	path := os.Getenv("APP_BRAND_FILE")
	if path == "" {
		path = defaultBrandFile
	}
	return brand.Load(path)
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
func loadWebhookHandler(ctx context.Context, logger *slog.Logger, b *brand.Brand, db *store.DB, secretsManager *secrets.Manager) (http.Handler, func() error, error) {
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

	var deployOpts []deploy.Option
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
// secretsManager and webhookHandler may both be nil (APP_MASTER_KEY and
// the webhook env vars are each independently optional): api.
// WithSecretSetter and the /webhook mount are only applied when set,
// since a nil *secrets.Manager wrapped in a non-nil api.SecretSetter
// interface value would panic the first time PUT .../secrets/{key}
// tried to call a method on it, rather than hitting api.Router's own
// "not configured" 501 path.
func rootHandler(logger *slog.Logger, b *brand.Brand, db *store.DB, secretsManager *secrets.Manager, webhookHandler http.Handler) http.Handler {
	var opts []api.Option
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
func dynamicSource(db *store.DB, runtime docker.Runtime, driver *ingressdriver.Driver, logger *slog.Logger, secretsManager *secrets.Manager) reconcile.Source {
	return func(ctx context.Context) ([]reconcile.Controller, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}
		databases, err := db.ListDesiredDatabases(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired databases: %w", err)
		}

		var appOpts []application.Option
		if secretsManager != nil {
			appOpts = append(appOpts, application.WithSecretResolver(secretsManager))
		}

		controllers := make([]reconcile.Controller, 0, len(services)+len(databases)+1)
		for _, svc := range services {
			controllers = append(controllers, application.New(svc.Name, db, runtime, appOpts...))
		}
		for _, desired := range databases {
			controllers = append(controllers, database.New(desired.Name, db, runtime))
		}
		controllers = append(controllers, ingressreconcile.New(db, runtime, driver, ingressreconcile.WithLogger(logger)))
		return controllers, nil
	}
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
