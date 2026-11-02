package main

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/diskguard"
	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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
	ingressdriver.DefaultAccessLog().SetVisitorRotationSource(func() time.Duration {
		return rs().VisitorSaltRotation(context.Background())
	})
	backup.SetLocalRootSource(func() string {
		return rs().String(context.Background(), runtimesettings.KeyBackupLocalRoot)
	})
	telemetry.SetLogBudgetSource(func() int64 {
		return int64(rs().Int(context.Background(), runtimesettings.KeyLogBudgetMB)) * mibBytes
	})
}
