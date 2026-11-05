// Package schedulescan finds stored cron schedules that can never fire, for
// rows saved before save time validation existed. It only reports.
package schedulescan

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/cronexpr"
)

// Source is the narrow store surface the scan reads.
type Source interface {
	ListScheduledDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	ListScheduledServiceVolumes(ctx context.Context) ([]store.ServiceVolumeBackupConfig, error)
	ListEnabledAppSchedules(ctx context.Context) ([]store.AppSchedule, error)
	ListEnabledScheduledTasks(ctx context.Context) ([]store.ScheduledTask, error)
}

// Finding is one stored schedule with no future occurrence.
type Finding struct {
	Kind   string // database_backup, volume_backup, app_deploy or scheduled_task
	ID     string
	App    string
	Expr   string
	Reason string
}

// Scan checks every stored schedule once. A list failure is returned with
// whatever was found before it, so one broken table hides nothing else.
func Scan(ctx context.Context, src Source, now time.Time) ([]Finding, error) {
	var out []Finding
	check := func(kind, id, app, expr string, loc *time.Location) {
		if expr == "" {
			return
		}
		if err := cronexpr.Validate(expr, loc, now); err != nil {
			out = append(out, Finding{Kind: kind, ID: id, App: app, Expr: expr, Reason: err.Error()})
		}
	}
	var firstErr error
	keep := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	dbs, err := src.ListScheduledDatabases(ctx)
	keep(err)
	for _, d := range dbs {
		check("database_backup", d.Name, d.Name, d.BackupSchedule, time.UTC)
	}
	vols, err := src.ListScheduledServiceVolumes(ctx)
	keep(err)
	for _, v := range vols {
		check("volume_backup", v.ServiceName+"/"+v.VolumeName, v.ServiceName, v.BackupSchedule, time.UTC)
	}
	apps, err := src.ListEnabledAppSchedules(ctx)
	keep(err)
	for _, a := range apps {
		loc := time.UTC
		if l, lerr := time.LoadLocation(a.Timezone); lerr == nil && a.Timezone != "" {
			loc = l
		}
		check("app_deploy", a.ServiceName, a.ServiceName, a.Cron, loc)
	}
	tasks, err := src.ListEnabledScheduledTasks(ctx)
	keep(err)
	for _, t := range tasks {
		check("scheduled_task", t.ID, t.ServiceName, t.Schedule, time.UTC)
	}
	if firstErr != nil {
		firstErr = fmt.Errorf("schedulescan: %w", firstErr)
	}
	return out, firstErr
}

// LogWarnings runs Scan once and logs one warning per offending schedule.
// Nothing is disabled: the owner decides whether to edit or delete it.
func LogWarnings(ctx context.Context, src Source, logger *slog.Logger, now time.Time) {
	found, err := Scan(ctx, src, now)
	if err != nil {
		logger.Warn("schedule scan incomplete", slog.String("error", err.Error()))
	}
	for _, f := range found {
		logger.Warn("stored schedule never runs",
			slog.String("kind", f.Kind), slog.String("id", f.ID), slog.String("app", f.App),
			slog.String("schedule", f.Expr), slog.String("reason", f.Reason))
	}
}
