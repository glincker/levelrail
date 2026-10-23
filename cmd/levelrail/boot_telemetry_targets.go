package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Re-derived on every call from the store and runtime, so it never serves stale targets.
func telemetryTargets(db *store.DB, local docker.Runtime, registry *agent.Registry, meshCfg *meshSetup, logger *slog.Logger) func(context.Context) ([]telemetry.Target, error) {
	deps := dynamicSourceDeps{meshCfg: meshCfg}
	return func(ctx context.Context) ([]telemetry.Target, error) {
		services, err := db.ListDesiredServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list desired services: %w", err)
		}

		var targets []telemetry.Target
		listings := map[string]*tickListing{}
		for _, svc := range services {
			nodeID := runtimeNodeID(deps, svc.NodeID)
			nodeRuntime, err := resolveNodeTransport(local, registry, nodeID)
			if err != nil {
				// Skipping a disconnected node's services is deliberate: metrics resume on reconnect.
				logger.Warn("telemetry: service's node unreachable, skipping this tick",
					slog.String("service", svc.Name), slog.String("node_id", nodeID), slog.String("error", err.Error()))
				continue
			}
			listing := listings[nodeID]
			if listing == nil {
				listing = &tickListing{rt: nodeRuntime}
				listings[nodeID] = listing
			}
			containers, err := listing.withPrefix(ctx, svc.Name+"-")
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
		listing := &tickListing{rt: runtime}
		for _, svc := range services {
			containers, err := listing.withPrefix(ctx, svc.Name+"-")
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
	loop := footprint.Track("logs retention sweep", logsRetentionSweepInterval)
	defer loop.Active()()
	ticker := time.NewTicker(logsRetentionSweepInterval)
	defer ticker.Stop()

	sweep := func() {
		defer loop.Observe(time.Now(), nil)
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
	fallback := defaultLogsRetention
	if profileRetention > 0 {
		fallback = profileRetention
	}
	raw := os.Getenv("APP_LOGS_RETENTION")
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
