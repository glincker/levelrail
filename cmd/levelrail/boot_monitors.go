package main

import (
	"log/slog"
	"os"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

// startMonitors runs the uptime monitor engine only when APP_MONITORS_ENABLED is true.
func (s *bootState) startMonitors() {
	cfg, err := monitor.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		s.logger.Error("monitors configuration invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if !cfg.Enabled {
		return
	}
	cfg.UserAgent = s.b.BinaryName + "-monitor"
	opts := monitor.Options{Config: cfg, Store: s.db, Logger: s.logger, Notify: func(t monitor.Transition) {
		s.logger.Warn("monitor state changed", slog.String("monitor", t.Monitor.ID), slog.String("state", t.State), slog.String("reason", t.Reason))
	}}
	if s.secretsManager != nil {
		opts.Secrets = s.secretsManager
	}
	svc := monitor.New(opts)
	s.apiRouter.SetMonitors(svc)
	go svc.Run(s.ctx)
}
