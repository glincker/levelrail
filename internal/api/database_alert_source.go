package api

import (
	"context"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/alerting"
	"github.com/glincker/levelrail/internal/dbaccess"
	"github.com/glincker/levelrail/internal/dbstats"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
	"github.com/glincker/levelrail/kit/forecast"
)

const (
	dbAlertTrendWindow = 7 * 24 * time.Hour
	dbAlertConnLookup  = 5 * time.Minute
	// dbAlertVolumeTTL keeps the df exec to one per database every few minutes, the trend moves slowly.
	dbAlertVolumeTTL = 10 * time.Minute
)

type dbVolumeReading struct {
	used, total int64
	at          time.Time
}

var (
	dbVolumeMu    sync.Mutex
	dbVolumeCache = map[string]dbVolumeReading{}
)

// DatabaseAlertPolicy reads the live alerts.database.* settings.
func (rt *Router) DatabaseAlertPolicy(ctx context.Context) alerting.DatabaseAlertPolicy {
	s := rt.runtimeSettings
	if s == nil {
		return alerting.DatabaseAlertPolicy{}
	}
	return alerting.DatabaseAlertPolicy{
		CrashLoopEnabled:       s.Bool(ctx, runtimesettings.KeyAlertsDatabaseCrashLoopEnabled),
		DiskGrowthEnabled:      s.Bool(ctx, runtimesettings.KeyAlertsDatabaseDiskEnabled),
		DiskWarnDays:           float64(s.Int(ctx, runtimesettings.KeyAlertsDatabaseDiskWarnDays)),
		DiskCriticalDays:       float64(s.Int(ctx, runtimesettings.KeyAlertsDatabaseDiskCriticalDays)),
		DiskFor:                s.Duration(ctx, runtimesettings.KeyAlertsDatabaseDiskFor),
		ConnectionsEnabled:     s.Bool(ctx, runtimesettings.KeyAlertsDatabaseConnectionsEnabled),
		ConnectionsPercent:     float64(s.Int(ctx, runtimesettings.KeyAlertsDatabaseConnectionsPercent)),
		ConnectionsFor:         s.Duration(ctx, runtimesettings.KeyAlertsDatabaseConnectionsFor),
		BackupEnabled:          s.Bool(ctx, runtimesettings.KeyAlertsDatabaseBackupEnabled),
		BackupWindow:           s.Duration(ctx, runtimesettings.KeyAlertsDatabaseBackupWindow),
		BackupUnprotectedBytes: int64(s.Int(ctx, runtimesettings.KeyAlertsDatabaseBackupUnprotectedMB)) << 20,
	}
}

// DatabaseAlertInputs implements alerting.DatabaseAlertSource from metrics the collectors already store.
// A database with no samples yet simply contributes zero values, which never breach.
func (rt *Router) DatabaseAlertInputs(ctx context.Context) ([]alerting.DatabaseAlertInput, error) {
	if rt.databases == nil {
		return nil, nil
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	crashing := map[string]string{}
	if rt.deploys != nil {
		if list, cerr := rt.crashLoopingDatabases(ctx); cerr == nil {
			for _, c := range list {
				crashing[c.Name] = c.Message
			}
		}
	}
	now := time.Now()
	out := make([]alerting.DatabaseAlertInput, 0, len(dbs))
	for _, d := range dbs {
		if d.Suspended {
			continue
		}
		in := alerting.DatabaseAlertInput{Name: d.Name, BackupTarget: d.BackupTargetID != "", BackupSchedule: d.BackupSchedule}
		if d.BackupPaused {
			in.BackupSchedule = ""
		}
		if msg, bad := crashing[d.Name]; bad {
			in.CrashLooping, in.CrashMessage = true, msg
		}
		if t, perr := time.Parse(time.RFC3339, d.CreatedAt); perr == nil {
			in.CreatedAt = t
		}
		rt.fillDatabaseMetrics(ctx, &in, d, now)
		rt.fillDatabaseBackup(ctx, &in, d.Name)
		out = append(out, in)
	}
	return out, nil
}

func (rt *Router) fillDatabaseMetrics(ctx context.Context, in *alerting.DatabaseAlertInput, d store.DesiredDatabase, now time.Time) {
	if rt.telemetry == nil {
		return
	}
	res := dbstats.ResourceID(d.Name)
	in.ConnTotal = latestSample(ctx, rt.telemetry, res, telemetry.MetricDBConnTotal, now.Add(-dbAlertConnLookup), now)
	in.ConnMax = latestSample(ctx, rt.telemetry, res, telemetry.MetricDBConnMax, now.Add(-dbAlertConnLookup), now)
	samples, err := rt.telemetry.QueryMetrics(ctx, res, telemetry.MetricDBSizePrefix+d.Name, now.Add(-dbAlertTrendWindow), now)
	if err != nil || len(samples) == 0 {
		return
	}
	latest := samples[len(samples)-1].Value
	in.SizeBytes = latest
	used, total, ok := rt.databaseVolume(ctx, d, now)
	if !ok {
		return
	}
	// The df figure anchors the line to the whole volume; the size series supplies the growth shape.
	in.DiskTotalBytes = float64(total)
	for _, s := range samples {
		in.DiskPoints = append(in.DiskPoints, forecast.Point{Timestamp: s.Timestamp, UsedBytes: float64(used) + (s.Value - latest)})
	}
}

func latestSample(ctx context.Context, q TelemetryQuerier, res, metric string, from, to time.Time) float64 {
	samples, err := q.QueryMetrics(ctx, res, metric, from, to)
	if err != nil || len(samples) == 0 {
		return 0
	}
	return samples[len(samples)-1].Value
}

func (rt *Router) databaseVolume(ctx context.Context, d store.DesiredDatabase, now time.Time) (used, total int64, ok bool) {
	dbVolumeMu.Lock()
	c, hit := dbVolumeCache[d.Name]
	dbVolumeMu.Unlock()
	if hit && now.Sub(c.at) < dbAlertVolumeTTL {
		return c.used, c.total, c.total > 0
	}
	_, ex, id, err := rt.databaseHealthTarget(ctx, d.Name)
	if err != nil {
		return 0, 0, false
	}
	pctx, cancel := context.WithTimeout(ctx, databaseHealthTimeout)
	defer cancel()
	used, total, err = dbaccess.ReadDiskBytes(pctx, ex, id, d.Engine)
	if err != nil {
		return 0, 0, false
	}
	dbVolumeMu.Lock()
	dbVolumeCache[d.Name] = dbVolumeReading{used: used, total: total, at: now}
	dbVolumeMu.Unlock()
	return used, total, total > 0
}

func (rt *Router) fillDatabaseBackup(ctx context.Context, in *alerting.DatabaseAlertInput, name string) {
	if rt.backupHistory == nil || !in.BackupTarget {
		return
	}
	history, err := rt.backupHistory.ListBackupHistory(ctx, name, 50, nil)
	if err != nil {
		// An unreadable history must not read as "never backed up".
		in.CreatedAt = time.Time{}
		return
	}
	for _, h := range history {
		if h.Status != store.BackupStatusSucceeded {
			continue
		}
		if t, perr := time.Parse(time.RFC3339, h.FinishedAt); perr == nil && t.After(in.LastBackupAt) {
			in.LastBackupAt = t
		}
	}
}
