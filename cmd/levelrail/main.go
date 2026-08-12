// Command levelrail is the control plane binary. This entry point is the
// Phase 0 skeleton only: it wires the reconcile engine to a single
// hardcoded nginx controller, with status conditions persisted to SQLite.
// Reading desired state from the store, the HTTP API, and everything else
// in CLAUDE.md 4 lands in later phases.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/nginxdemo"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	resyncInterval = 30 * time.Second
	// defaultDataDir matches CLAUDE.md section 3's APP_DATA_DIR override
	// pattern. Relative, not /var/lib/levelrail-data (brand.yaml's noted
	// pending path), since Phase 0 runs locally, not installed as a
	// service yet.
	defaultDataDir = "./data"
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

	engine := reconcile.NewEngine(logger, nginxdemo.New(client))
	engine.SetStore(db)

	events, errs := client.Events(ctx)
	go func() {
		if err, ok := <-errs; ok && err != nil {
			logger.Error("docker event stream error", slog.String("error", err.Error()))
		}
	}()

	err = engine.Run(ctx, events, resyncInterval)
	if errors.Is(err, context.Canceled) {
		return nil // Ctrl+C / SIGTERM is a clean shutdown, not a failure
	}
	return err
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
