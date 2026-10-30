package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/heartbeat"
	"github.com/GLINCKER/levelrail/internal/version"
)

const (
	heartbeatStoreProbeTimeout = 5 * time.Second
	reconcileLoopName          = "reconcile pass"
)

// startHeartbeat runs the dead-man ping. It is idle until a URL is set by env or from the dashboard.
func (s *bootState) startHeartbeat() {
	cfg, err := heartbeat.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		s.logger.Error("heartbeat configuration invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}
	opts := heartbeat.Options{Config: cfg, State: s.db, Health: s.heartbeatHealth(cfg), Logger: s.logger,
		Version: version.Version, UserAgent: s.b.BinaryName + "-heartbeat"}
	if s.secretsManager != nil {
		opts.Secrets = s.secretsManager
	}
	svc := heartbeat.New(opts)
	s.apiRouter.SetHeartbeat(svc)
	if s.agentServer != nil {
		s.agentServer.SetSilenceSource(svc.AgentSilenceURL)
	}
	go svc.Run(s.ctx)
}

// heartbeatHealth is the gate for the ping: a box that is up but not converging must stop pinging.
func (s *bootState) heartbeatHealth(cfg heartbeat.Config) heartbeat.Health {
	return func(ctx context.Context) (bool, string) {
		pctx, cancel := context.WithTimeout(ctx, heartbeatStoreProbeTimeout)
		defer cancel()
		if err := s.db.PingContext(pctx); err != nil {
			return false, "store"
		}
		for _, l := range footprint.Loops() {
			if l.Name != reconcileLoopName {
				continue
			}
			window := time.Duration(float64(cfg.ReconcileWindows) * l.IntervalSecs * float64(time.Second))
			if !l.LastRunAt.IsZero() && time.Since(l.LastRunAt) > window {
				return false, "reconcile"
			}
		}
		// A fresh install has no local node id yet, so there is no agent to require.
		if id := readLocalNodeID(s.agentDataDir); id != "" {
			if _, err := s.agentRegistry.Resolve(id); err != nil {
				return false, "agent"
			}
		}
		return true, ""
	}
}
