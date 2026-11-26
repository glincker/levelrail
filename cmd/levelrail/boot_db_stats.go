package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbstats"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// dbStatsSource lists Postgres databases whose container runs on this node. Databases placed on remote nodes are
// skipped for now: their exec would have to go through the node agent, which this slice does not wire.
type dbStatsSource struct {
	db      *store.DB
	runtime docker.Runtime
	meshCfg *meshSetup
	logger  *slog.Logger
}

func (s dbStatsSource) RunningDatabases(ctx context.Context) ([]dbstats.Target, error) {
	databases, err := s.db.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	deps := dynamicSourceDeps{meshCfg: s.meshCfg}
	var out []dbstats.Target
	for _, d := range databases {
		if d.Engine != store.EnginePostgres || runtimeNodeID(deps, d.NodeID) != "" {
			continue
		}
		cs, err := s.runtime.InspectByName(ctx, "db-"+d.Name)
		if err != nil {
			s.logger.Debug("db stats: inspect failed", slog.String("database", d.Name), slog.String("error", err.Error()))
			continue
		}
		// A stopped, restoring or upgrading database has no running container under this name.
		if cs == nil || !cs.Running {
			continue
		}
		out = append(out, dbstats.Target{Name: d.Name, Engine: d.Engine, ContainerID: cs.ID})
	}
	return out, nil
}

// startDBStats runs the Postgres statistics collector. Levelrail has no multi-instance leader election yet, so the
// single control plane process owns it. It stops with the boot context.
func (s *bootState) startDBStats(out *telemetry.DB) {
	if s.apiRouter == nil || s.client == nil {
		return
	}
	rs := s.apiRouter.RuntimeSettings()
	c := dbstats.New(dbStatsSource{db: s.db, runtime: s.client, meshCfg: s.meshCfg, logger: s.logger}, s.client, out,
		func() dbstats.Config {
			ctx := context.Background()
			return dbstats.Config{
				Timeout:      time.Duration(rs.Limit(ctx, runtimesettings.KeyDatabaseStatsTimeout)) * time.Second,
				MaxDatabases: rs.Limit(ctx, runtimesettings.KeyDatabaseStatsMaxDatabases),
			}
		},
		func() time.Duration {
			return time.Duration(rs.Limit(context.Background(), runtimesettings.KeyDatabaseStatsInterval)) * time.Second
		}, s.logger)
	go func() {
		c.Run(s.ctx)
		if err := s.ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
			s.logger.Error("db stats collector stopped", slog.String("error", err.Error()))
		}
	}()
}
