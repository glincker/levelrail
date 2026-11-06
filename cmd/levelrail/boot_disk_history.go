package main

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskhistory"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// startDiskHistory samples disk usage on the runtime-set interval and serves the history API.
func (s *bootState) startDiskHistory() {
	store := diskhistory.NewStore(s.db.DB)
	s.apiRouter.SetDiskHistory(store)
	rs := func() *runtimesettings.Service { return s.apiRouter.RuntimeSettings() }
	settings := diskhistory.Settings{
		Interval: func() time.Duration {
			return max(rs().Duration(context.Background(), runtimesettings.KeyDiskHistorySampleInterval), runtimesettings.DiskHistoryMinSampleInterval)
		},
		Retention: func() time.Duration {
			return time.Duration(rs().Int(context.Background(), runtimesettings.KeyDiskHistoryRetentionDays)) * 24 * time.Hour
		},
	}
	sampler := diskhistory.NewSampler(store, s.apiRouter.DiskHistorySource(), settings, runtimesettings.DiskHistoryMinSampleInterval, s.logger)
	go sampler.Run(s.ctx)
}
