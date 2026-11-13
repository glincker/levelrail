package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	quarantineRuleID                  = "crash-loop-quarantine"
	restartQuarantineDBTimeoutEnv     = "APP_RESTART_QUARANTINE_DB_TIMEOUT"
	defaultRestartQuarantineDBTimeout = 5 * time.Second
)

// quarantineSink persists breaker holds and raises an inbox alert plus an app event for each.
type quarantineSink struct {
	db     *store.DB
	alerts *alerting.DB
	logger *slog.Logger
	// timeout bounds each DB write so a stuck store cannot stall the reconcile pass.
	timeout time.Duration
}

// wireRestartQuarantine loads persisted holds into b and attaches the sink so new ones are saved.
func wireRestartQuarantine(ctx context.Context, b *application.RestartBackoff, db *store.DB, alerts *alerting.DB, logger *slog.Logger) {
	if _, err := db.PruneRestartQuarantine(ctx); err != nil {
		logger.Error("restart quarantine: prune failed", slog.String("error", err.Error()))
	}
	rows, err := db.ListRestartQuarantine(ctx)
	if err != nil {
		// Fail open: boot must not block on this table, so containers are simply not held after a failed load.
		logger.Error("restart quarantine: load failed", slog.String("error", err.Error()))
	}
	for _, q := range rows {
		b.Restore(q.Container, q.Restarts, q.QuarantinedAt)
	}
	b.SetSink(&quarantineSink{db: db, alerts: alerts, logger: logger, timeout: envDurationOr(logger, restartQuarantineDBTimeoutEnv, defaultRestartQuarantineDBTimeout)})
}

func (s *quarantineSink) Held(container string, restarts int, at time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	app, _ := application.ParseContainerName(container)
	if err := s.db.SaveRestartQuarantine(ctx, store.RestartQuarantine{Container: container, AppName: app, Restarts: restarts, QuarantinedAt: at}); err != nil {
		s.logger.Error("restart quarantine: save failed", slog.String("container", container), slog.String("error", err.Error()))
	}
	msg := deploysafety.QuarantineMessage(container, restarts)
	s.record(ctx, app, alerting.EventFired, msg, at)
	if err := s.db.AddAppEvent(ctx, store.AppEvent{AppName: app, Kind: store.AppEventRestart, Actor: "system", Title: "Quarantined after repeated crashes", Detail: msg}); err != nil {
		s.logger.Error("restart quarantine: app event failed", slog.String("error", err.Error()))
	}
}

func (s *quarantineSink) Released(container string) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	app, _ := application.ParseContainerName(container)
	if err := s.db.DeleteRestartQuarantine(ctx, container); err != nil {
		s.logger.Error("restart quarantine: delete failed", slog.String("container", container), slog.String("error", err.Error()))
	}
	s.record(ctx, app, alerting.EventResolved, fmt.Sprintf("%s was released from quarantine", container), time.Now())
}

func (s *quarantineSink) record(ctx context.Context, app, event, detail string, at time.Time) {
	if s.alerts == nil {
		return
	}
	err := s.alerts.RecordHistory(ctx, alerting.HistoryEntry{
		At: at, RuleID: quarantineRuleID, RuleName: "Crash-loop quarantine", RuleKind: string(alerting.KindCrashloop),
		ResourceID: "service:" + app, App: app, Severity: "critical", Event: event, Outcome: alerting.OutcomeSent, Detail: detail,
	})
	if err != nil {
		s.logger.Error("restart quarantine: alert failed", slog.String("error", err.Error()))
	}
}
