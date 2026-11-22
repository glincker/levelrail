package main

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

// durationAlertSource feeds kind=deploy_duration_regression rules from build runs and deploy attempts.
type durationAlertSource struct{ db *store.DB }

// RecentDurations implements alerting.DurationSource. Build rows come newest first from the
// build history, deploy rows from the succeeded attempts that have both start and finish times.
func (s durationAlertSource) RecentDurations(ctx context.Context) ([]alerting.DurationSeries, error) {
	runs, err := s.db.ListAllBuildRuns(ctx, backupSizeHistoryRows)
	if err != nil {
		return nil, err
	}
	attempts, err := s.db.ListRecentSucceededDeployAttempts(ctx, backupSizeHistoryRows)
	if err != nil {
		return nil, err
	}
	idx := map[string]int{}
	var out []alerting.DurationSeries
	add := func(app, stage string, seconds int64) {
		k := app + "\x00" + stage
		i, ok := idx[k]
		if !ok {
			i = len(out)
			idx[k] = i
			out = append(out, alerting.DurationSeries{App: app, Stage: stage})
		}
		out[i].Seconds = append(out[i].Seconds, seconds)
	}
	for _, r := range runs {
		if r.Status == string(build.RunSuccess) && r.DurationMS > 0 {
			add(r.App, alerting.DurationStageBuild, r.DurationMS/1000)
		}
	}
	for _, a := range attempts {
		if a.FinishedAt != nil && a.FinishedAt.After(a.StartedAt) {
			add(a.ServiceName, alerting.DurationStageDeploy, int64(a.FinishedAt.Sub(a.StartedAt).Seconds()))
		}
	}
	return out, nil
}
