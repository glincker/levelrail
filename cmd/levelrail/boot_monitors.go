package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/incident"
	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/store"
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
	opener := s.newIncidentOpener()
	linker := s.newMonitorLinker()
	opts := monitor.Options{Config: cfg, Store: s.db, Logger: s.logger, OnDelete: downs.Forget, Notify: func(t monitor.Transition) {
		downs.Record(t.Monitor.ID, t.Monitor.Name, t.Reason, t.State == monitor.StateDown, time.Now())
		if opener != nil {
			opener.Notify(t)
		}
		if linker != nil {
			linker.Notify(t)
		}
		s.logger.Warn("monitor state changed", slog.String("monitor", t.Monitor.ID), slog.String("state", t.State), slog.String("reason", t.Reason))
	}}
	if s.secretsManager != nil {
		opts.Secrets = s.secretsManager
	}
	svc := monitor.New(opts)
	s.apiRouter.SetMonitors(svc)
	go svc.Run(s.ctx)
	if linker != nil {
		linker.SetMonitors(svc)
		go linker.Run(s.ctx)
	}
	if opener != nil {
		opener.SetMonitors(svc)
		go opener.Run(s.ctx)
	}
}

// newIncidentOpener returns nil unless APP_MONITORS_ENABLED and APP_INCIDENTS_ENABLED are both true.
func (s *bootState) newIncidentOpener() *incident.Opener {
	cfg, err := incident.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		s.logger.Error("incidents configuration invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if !cfg.Enabled || s.alertingDB == nil {
		return nil
	}
	guard := incident.AlertGuard{Src: s.alertingDB, Logger: s.logger}
	s.apiRouter.SetIncidents(s.db, cfg.UpdateBodyMax)
	return incident.New(cfg, s.db, guard, s.logger)
}

// newMonitorLinker builds the opt-in status page link; it runs under APP_MONITORS_ENABLED alone and
// does nothing for a monitor until its status_link setting is on.
func (s *bootState) newMonitorLinker() *incident.Linker {
	cfg, err := incident.LinkConfigFromEnv(os.LookupEnv)
	if err != nil {
		s.logger.Error("monitor link configuration invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}
	var guard incident.Guard
	if s.alertingDB != nil {
		guard = incident.AlertGuard{Src: s.alertingDB, Logger: s.logger}
	}
	audit := func(ctx context.Context, action, monitorID, incidentID string) {
		detail, _ := json.Marshal(map[string]string{"monitor_id": monitorID, "incident_id": incidentID})
		e := store.AuditEntry{ID: incident.NewAuditID(), ActorType: "system", ActorName: "monitor-link", Method: "SYSTEM",
			Path: "/api/v1/monitors/" + monitorID, StatusCode: 200, CreatedAt: store.FormatAuditTime(time.Now()),
			ClientKind: "system", Action: action, Detail: string(detail)}
		if err := s.db.SaveAuditEntry(ctx, e); err != nil {
			s.logger.Error("monitor link audit failed", slog.String("error", err.Error()))
		}
	}
	return incident.NewLinker(cfg, s.db, guard, nil, audit, s.logger)
}
