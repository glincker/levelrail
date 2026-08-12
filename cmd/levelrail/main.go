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
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/docker"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	ingressreconcile "github.com/GLINCKER/levelrail/internal/reconcile/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
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

	httpServer := &http.Server{
		Addr:              httpAddr(),
		Handler:           api.NewRouter(logger, b, db).Handler(),
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
	engine.SetSource(dynamicSource(db, client, ingressDriver, logger))

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
func dynamicSource(db *store.DB, runtime docker.Runtime, driver *ingressdriver.Driver, logger *slog.Logger) reconcile.Source {
	return func(ctx context.Context) ([]reconcile.Controller, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}
		databases, err := db.ListDesiredDatabases(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired databases: %w", err)
		}

		controllers := make([]reconcile.Controller, 0, len(services)+len(databases)+1)
		for _, svc := range services {
			controllers = append(controllers, application.New(svc.Name, db, runtime))
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
