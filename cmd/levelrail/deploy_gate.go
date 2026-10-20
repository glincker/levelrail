package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploywatch"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// deployGate is the shared state behind the readiness gate, drain and
// rollback pins, built once and reused by every per-pass controller.
type deployGate struct {
	defaults deploysafety.Effective
	drain    *application.DrainTracker
	pinner   *deploywatch.Pinner
}

// newDeployGate reads the APP_DEPLOY_* defaults. Pinning needs a local
// image client; without one releases are simply not pinned.
func newDeployGate(logger *slog.Logger, db *store.DB, runtime docker.Runtime, namespace, localNodeID string) deployGate {
	defaults, warnings := deploysafety.DefaultsFromEnv(os.LookupEnv)
	for _, w := range warnings {
		logger.Warn("invalid deploy safety setting, using the default", slog.String("error", w.Error()))
	}
	g := deployGate{defaults: defaults, drain: application.NewDrainTracker()}
	if images, ok := runtime.(docker.ImagePinner); ok {
		g.pinner = &deploywatch.Pinner{Store: db, Images: images, Keep: defaults.RollbackPins, Namespace: namespace, LocalNodeID: localNodeID, Logger: logger}
	}
	return g
}

func (g deployGate) appOptions(db *store.DB) []application.Option {
	opts := []application.Option{
		application.WithStabilityGate(g.defaults, db),
		application.WithGateRecorder(db),
		application.WithDrainTracker(g.drain),
	}
	if g.pinner != nil {
		opts = append(opts, application.WithRollbackPinner(g.pinner))
	}
	return opts
}

// deployGateDefaults is the APP_DEPLOY_* defaults; newDeployGate already
// logs any invalid value.
func deployGateDefaults() deploysafety.Effective {
	d, _ := deploysafety.DefaultsFromEnv(os.LookupEnv)
	return d
}

func meshLocalNodeID(cfg *meshSetup) string {
	if cfg == nil {
		return ""
	}
	return cfg.localNodeID
}

// deployWatchDeps is what the post-deploy watch reads and acts through.
type deployWatchDeps struct {
	db          *store.DB
	local       docker.Runtime
	registry    *agent.Registry
	localNodeID string
	restarts    deploywatch.RestartCounter
	sampleEvery time.Duration
	metrics     telemetry.MetricsQuerier
	nudger      deploy.ReconcileNudger
	dispatcher  *alerting.DeployDispatcher
	logger      *slog.Logger
}

// startDeployWatch runs the post-deploy watch until ctx is cancelled.
func startDeployWatch(ctx context.Context, g deployGate, d deployWatchDeps) {
	resolver, _ := d.local.(docker.ImageResolver)
	w := &deploywatch.Watcher{
		Store: d.db, Audit: d.db, Defaults: g.defaults, Restarts: d.restarts, Metrics: d.metrics,
		Resolver: resolver, Nudger: d.nudger, Logger: d.logger, SampleInterval: d.sampleEvery,
		Inspectors: func(nodeID string) (docker.ExitStateInspector, error) {
			if nodeID == d.localNodeID {
				nodeID = ""
			}
			rt, err := resolveNodeTransport(d.local, d.registry, nodeID)
			if err != nil {
				return nil, err
			}
			inspector, _ := rt.(docker.ExitStateInspector)
			return inspector, nil
		},
		Notify: func(ctx context.Context, app, image, message string) {
			if d.dispatcher == nil {
				return
			}
			d.dispatcher.Dispatch(ctx, "service:"+app, alerting.DeployOutcome{AppName: app, Image: image, Succeeded: false, Error: message})
		},
	}
	go w.Run(ctx, g.defaults.WatchInterval)
}
