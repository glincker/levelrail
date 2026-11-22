package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/incident"
	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// startMonitors watches the monitors.enabled runtime setting (seeded by APP_MONITORS_ENABLED) and
// starts or stops the engine to match, so toggling it needs no restart.
func (s *bootState) startMonitors() {
	cfg, err := monitor.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		s.logger.Error("monitors configuration invalid", slog.String("error", err.Error()))
		os.Exit(1)
	}
	go s.superviseMonitors(cfg)
}

func (s *bootState) superviseMonitors(cfg monitor.Config) {
	var stop context.CancelFunc
	var done chan struct{}
	tick := time.NewTicker(cfg.FlushInterval)
	defer tick.Stop()
	for {
		on := s.apiRouter.RuntimeSettings().Bool(s.ctx, runtimesettings.KeyMonitorsEnabled)
		switch {
		case on && stop == nil:
			ctx, cancel := context.WithCancel(s.ctx)
			stop, done = cancel, make(chan struct{})
			go func() { defer close(done); s.runMonitors(ctx, cfg) }()
			s.logger.Info("monitors started")
		case !on && stop != nil:
			stop()
			<-done
			s.apiRouter.SetMonitors(nil)
			stop = nil
			s.logger.Info("monitors stopped")
		}
		select {
		case <-s.ctx.Done():
			if stop != nil {
				stop()
				<-done
			}
			return
		case <-tick.C:
		}
	}
}

// runMonitors blocks until ctx ends, running the engine and the incident helpers around it.
func (s *bootState) runMonitors(ctx context.Context, cfg monitor.Config) {
	cfg.UserAgent = s.b.BinaryName + "-monitor"
	downs := alerting.NewMonitorDownTracker()
	if s.alertingEngine != nil {
		s.alertingEngine.SetMonitorDowns(downs)
	}
	opener := s.newIncidentOpener()
	linker := s.newMonitorLinker()
	opts := monitor.Options{Config: cfg, Store: s.db, Heartbeats: s.db, Logger: s.logger, OnDelete: downs.Forget, Notify: func(t monitor.Transition) {
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
	svc.SetRawRetentionSource(retentionSetting(s.apiRouter, runtimesettings.KeyMonitorRawRetained, 0))
	s.apiRouter.SetMonitors(svc)
	var wg sync.WaitGroup
	run := func(f func(context.Context)) {
		wg.Add(1)
		go func() { defer wg.Done(); f(ctx) }()
	}
	run(svc.Run)
	if linker != nil {
		linker.SetPublisher(s.apiRouter.EmitStatusIncident)
		linker.SetMonitors(svc)
		run(linker.Run)
	}
	if opener != nil {
		opener.SetMonitors(svc)
		run(opener.Run)
	}
	wg.Wait()
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
	guard := incidentAlertGuard{Src: s.alertingDB, Logger: s.logger}
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
		guard = incidentAlertGuard{Src: s.alertingDB, Logger: s.logger}
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
