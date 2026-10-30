package incident

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// AlertSource is the slice of the alerting store the guard reads.
type AlertSource interface {
	ListEnabledRules(ctx context.Context) ([]alerting.Rule, error)
	ListActiveSilences(ctx context.Context, now time.Time) ([]alerting.Silence, error)
	ListMaintenanceWindows(ctx context.Context) ([]alerting.MaintenanceWindow, error)
}

// AlertGuard applies the monitor_down rule switch and the silence and maintenance mutes.
type AlertGuard struct {
	Src    AlertSource
	Logger *slog.Logger
}

func (g AlertGuard) rule(ctx context.Context) (alerting.Rule, bool) {
	rules, err := g.Src.ListEnabledRules(ctx)
	if err != nil {
		g.log().Error("incident guard: list rules failed", slog.String("error", err.Error()))
		return alerting.Rule{}, false
	}
	for _, r := range rules {
		if r.Kind == alerting.KindMonitorDown {
			return r, true
		}
	}
	return alerting.Rule{}, false
}

func (g AlertGuard) log() *slog.Logger {
	if g.Logger != nil {
		return g.Logger
	}
	return slog.Default()
}

// RuleEnabled reports whether an enabled monitor_down rule exists.
func (g AlertGuard) RuleEnabled(ctx context.Context) bool {
	_, ok := g.rule(ctx)
	return ok
}

// Muted builds the context the monitor_down rule would use and asks FindMute about it.
func (g AlertGuard) Muted(ctx context.Context, monitorID, name string, now time.Time) (string, bool) {
	r, _ := g.rule(ctx)
	labels := map[string]string{"monitor_id": monitorID, "monitor": name}
	for k, v := range r.Labels {
		labels[k] = v
	}
	ac := alerting.AlertContext{RuleID: r.ID, Kind: string(alerting.KindMonitorDown), Labels: labels, Severity: r.Severity}
	silences, err := g.Src.ListActiveSilences(ctx, now)
	if err != nil {
		g.log().Error("incident guard: list silences failed", slog.String("error", err.Error()))
	}
	windows, err := g.Src.ListMaintenanceWindows(ctx)
	if err != nil {
		g.log().Error("incident guard: list maintenance windows failed", slog.String("error", err.Error()))
	}
	hit, ok := alerting.FindMute(silences, windows, ac, now)
	return hit.Detail, ok
}
