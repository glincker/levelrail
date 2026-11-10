package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/demo"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/safemode"
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
	startPprof(logger)
	footprint.ApplyGCPercent(os.Getenv)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ledger, policy, entry := prepareBoot(ctx, logger)
	if entry == nil {
		err := bootAndServe(ctx, stop, logger, ledger, policy)
		var sm *safeModeError
		var refusal *downgradeRefusal
		switch {
		case errors.As(err, &sm):
			ledger.NoteError(sm.err)
			entry = &sm.entry
		case err == nil || errors.Is(err, errRestartRequested):
			ledger.MarkClean()
			return err
		default:
			if !errors.As(err, &refusal) {
				ledger.NoteError(err)
			}
			return err
		}
	}
	return runSafeMode(ctx, stop, logger, ledger, policy, *entry)
}

func bootAndServe(ctx context.Context, stop context.CancelFunc, logger *slog.Logger, ledger *safemode.Ledger, policy safemode.Policy) error {
	s := &bootState{ctx: ctx, stop: stop, logger: logger, ledger: ledger, bootPolicy: policy}
	defer s.runDeferred()

	stages := []bootStage{
		{name: "store", fn: s.openStore},
		{name: "docker", fn: s.openDocker},
		{name: "identity", fn: s.bootIdentity},
		{name: "telemetry_stores", fn: s.openTelemetryStores},
		{name: "agent_server", fn: s.startAgentServer},
		{name: "secrets_notifiers", fn: func() error { s.loadSecretsAndNotifiers(); return nil }},
		{name: "runners", fn: s.buildRunners},
		{name: "build_ingress", fn: func() error { s.startBuildAndIngress(); return nil }},
		{name: "router", fn: func() error { s.wireRouter(); s.wireProcfilePort(); return nil }},
		{name: "mesh", fn: s.startMesh},
		{name: "reconciler", fn: func() error { s.startReconciler(); return nil }},
		{name: "telemetry", fn: func() error { s.startTelemetry(); return nil }},
		{name: "alerting", fn: func() error { s.startAlerting(); return nil }},
		{name: "heartbeat", deferrable: true, fn: func() error { s.startHeartbeat(); return nil }},
		{name: "monitors", deferrable: true, fn: func() error { s.startMonitors(); return nil }},
		{name: "disk_history", deferrable: true, fn: func() error { s.startDiskHistory(); return nil }},
		{name: "schedulers", fn: func() error { s.startSchedulers(); return nil }},
	}
	// The setting lives in the store, so it is read once the router exists (after the router stage).
	deferOn := false
	for _, st := range stages {
		if st.deferrable && s.apiRouter != nil && !deferOn {
			deferOn = s.apiRouter.RuntimeSettings().Bool(ctx, runtimesettings.KeyBootDeferredStages)
		}
		if st.deferrable && deferOn {
			s.pending.add(st.name)
			s.deferredQueue = append(s.deferredQueue, st)
			continue
		}
		if err := footprint.TimeBootStage(st.name, st.fn); err != nil {
			return err
		}
	}
	return s.serve()
}
