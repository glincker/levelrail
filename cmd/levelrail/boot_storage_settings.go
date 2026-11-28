package main

import (
	"context"
	"time"

	"github.com/glincker/levelrail/internal/api"
	"github.com/glincker/levelrail/internal/backup"
	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/diskguard"
	ingressdriver "github.com/glincker/levelrail/internal/ingress"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
)

const mibBytes = int64(1) << 20

// diskThresholdSource reads the disk floors from runtime settings on every check.
func diskThresholdSource(router *api.Router) func() diskguard.Thresholds {
	return func() diskguard.Thresholds {
		rs, c := router.RuntimeSettings(), context.Background()
		return diskguard.Thresholds{
			LowFree:      int64(rs.Int(c, runtimesettings.KeyDiskLowFreeMB)) * mibBytes,
			CriticalFree: int64(rs.Int(c, runtimesettings.KeyDiskCriticalFreeMB)) * mibBytes,
			Reserve:      int64(rs.Int(c, runtimesettings.KeyDiskReserveMB)) * mibBytes,
		}
	}
}

// retentionSetting returns the stored window; a profile default only fills in when nothing was saved or set in env.
func retentionSetting(router *api.Router, key string, profileDefault time.Duration) func() time.Duration {
	return func() time.Duration {
		rs, c := router.RuntimeSettings(), context.Background()
		if profileDefault > 0 && rs.Source(c, key) == runtimesettings.SourceDefault {
			return profileDefault
		}
		return rs.Duration(c, key)
	}
}

// wireStorageSettings points telemetry at the runtime settings so saved changes apply without a restart.
func (s *bootState) wireStorageSettings() {
	rs := func() *runtimesettings.Service { return s.apiRouter.RuntimeSettings() }
	s.telemetryDB.SetRawRetentionSource(retentionSetting(s.apiRouter, runtimesettings.KeyMetricsRetention, profileRetention))
	ingressdriver.DefaultAccessLog().SetRetentionSource(func() time.Duration {
		return rs().AccessLogRetention(context.Background())
	})
	ingressdriver.DefaultAccessLog().SetCapacitySource(func() int {
		return rs().Limit(context.Background(), runtimesettings.KeyAccessLogSize)
	})
	ingressdriver.DefaultAccessLog().SetVisitorRotationSource(func() time.Duration {
		return rs().VisitorSaltRotation(context.Background())
	})
	log := ingressdriver.DefaultAccessLog()
	log.SetVisitorHistory(s.db, ingressdriver.DefaultRequestStats().OwnerOf, func() time.Duration {
		return rs().VisitorHistoryRetention(context.Background())
	}, s.logger)
	go log.RunVisitorHistory(s.ctx)
	backup.SetLocalRootSource(func() string {
		return rs().String(context.Background(), runtimesettings.KeyBackupLocalRoot)
	})
	deploy.SetBuildReuseSource(func() bool {
		return rs().Bool(context.Background(), runtimesettings.KeyBuildReuseUnchanged)
	})
	store.SetReadConnsSource(func() int {
		return rs().Limit(context.Background(), runtimesettings.KeyReadConns)
	})
	store.SetTuningSource(func() store.Tuning {
		c := context.Background()
		return store.Tuning{
			CacheSizeMB: rs().Int(c, runtimesettings.KeyDBCacheSizeMB),
			MmapSizeMB:  rs().Int(c, runtimesettings.KeyDBMmapSizeMB),
			TempStore:   dbTuningName(rs().String(c, runtimesettings.KeyDBTempStore)),
			Synchronous: dbTuningName(rs().String(c, runtimesettings.KeyDBSynchronous)),
		}
	})
	go s.db.RunOptimize(s.ctx, func() time.Duration {
		return rs().Duration(context.Background(), runtimesettings.KeyDBOptimizeInterval)
	})
	telemetry.SetRetentionTuningSource(func() telemetry.RetentionTuning {
		c := context.Background()
		return telemetry.RetentionTuning{
			BatchRows:   rs().Int(c, runtimesettings.KeyRetentionBatchRows),
			SweepBudget: rs().Duration(c, runtimesettings.KeyRetentionSweepBudget),
			Pause:       rs().Duration(c, runtimesettings.KeyRetentionBatchPause),
		}
	})
	telemetry.SetCumulativeLookbackSource(func() time.Duration {
		c := context.Background()
		base := telemetry.CumulativeLookback(rs().Duration(c, runtimesettings.KeyMetricsIdleInterval), rs().Duration(c, runtimesettings.KeyMetricsSampleInterval))
		return lookbackWithDBInterval(base, time.Duration(rs().Limit(c, runtimesettings.KeyDatabaseStatsInterval))*time.Second)
	})
	telemetry.SetTierConfigSource(func(base telemetry.TierConfig) telemetry.TierConfig {
		return tierConfigFromSettings(base, rs().MetricsTiers(context.Background()))
	})
	telemetry.SetCapTargetPercentSource(func() int {
		return rs().Int(context.Background(), runtimesettings.KeyMetricsCapTarget)
	})
	telemetry.SetLogBudgetSource(func() int64 {
		return int64(rs().Int(context.Background(), runtimesettings.KeyLogBudgetMB)) * mibBytes
	})
}

// tierConfigFromSettings overlays the live tier settings on the booted config; a zero duration keeps the booted value.
func tierConfigFromSettings(base telemetry.TierConfig, t runtimesettings.MetricsTiers) telemetry.TierConfig {
	keep := func(dst *time.Duration, v time.Duration) {
		if v > 0 {
			*dst = v
		}
	}
	keep(&base.MinuteRetention, t.Retention1m)
	keep(&base.HourRetention, t.Retention1h)
	keep(&base.RawMaxRange, t.RawRange)
	keep(&base.MinuteMaxRange, t.Range1m)
	keep(&base.Lag, t.Lag)
	if t.MaxDBBytes >= 0 {
		base.MaxBytes = t.MaxDBBytes
	}
	return base
}

// dbTuningName maps the "default" setting value to empty, meaning leave SQLite alone.
func dbTuningName(v string) string {
	if v == runtimesettings.DBTuningDefault {
		return ""
	}
	return v
}

// lookbackWithDBInterval widens the cumulative scan to two database sample gaps, which can exceed the metric ones.
func lookbackWithDBInterval(base, dbInterval time.Duration) time.Duration {
	return max(base, 2*dbInterval)
}
