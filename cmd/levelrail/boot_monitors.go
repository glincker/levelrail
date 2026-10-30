package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
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
	downs := alerting.NewMonitorDownTracker()
	if s.alertingEngine != nil {
		s.alertingEngine.SetMonitorDowns(downs)
	}
	opts := monitor.Options{Config: cfg, Store: s.db, Logger: s.logger, OnDelete: downs.Forget, Notify: func(t monitor.Transition) {
		downs.Record(t.Monitor.ID, t.Monitor.Name, t.Reason, t.State == monitor.StateDown, time.Now())
		s.logger.Warn("monitor state changed", slog.String("monitor", t.Monitor.ID), slog.String("state", t.State), slog.String("reason", t.Reason))
	}}
	if s.secretsManager != nil {
		opts.Secrets = s.secretsManager
	}
	svc := monitor.New(opts)
	s.apiRouter.SetMonitors(svc)
	go svc.Run(s.ctx)
}
