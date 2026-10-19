package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/version"
)

func run(logger *slog.Logger) error {
	logger.Info("starting", slog.String("version", version.Version))
	if demo.Enabled(os.Getenv) {
		logger.Warn("ignoring " + demo.EnvDemoMode + ": demo mode only exists behind the demo subcommand")
	}
	if err := experimental.Validate(); err != nil {
		logger.Warn("ignoring unknown experimental feature", slog.String("error", err.Error()))
	}
	logger.Info("experimental features", slog.Any("enabled", experimental.EnabledList()))

	footprint.ApplyMemoryLimit(os.Getenv, logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := &bootState{ctx: ctx, stop: stop, logger: logger}
	defer s.runDeferred()

	if err := s.openStore(); err != nil {
		return err
	}
	if err := s.openDocker(); err != nil {
		return err
	}
	if err := s.bootIdentity(); err != nil {
		return err
	}
	if err := s.openTelemetryStores(); err != nil {
		return err
	}
	if err := s.startAgentServer(); err != nil {
		return err
	}
	s.loadSecretsAndNotifiers()
	if err := s.buildRunners(); err != nil {
		return err
	}
	s.startBuildAndIngress()
	s.wireRouter()
	if err := s.startMesh(); err != nil {
		return err
	}
	s.startReconciler()
	s.startTelemetry()
	s.startAlerting()
	s.startSchedulers()
	return s.serve()
}
