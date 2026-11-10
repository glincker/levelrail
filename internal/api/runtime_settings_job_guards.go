package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/scheduledtask"
)

// jobGuardSetting is one live job guard rail bound; env only seeds it.
type jobGuardSetting struct {
	key, env, def, title, desc string
	lo, hi                     int
}

var jobGuardSettings = []jobGuardSetting{
	{scheduledtask.KeyTimeoutMinSeconds, scheduledtask.EnvTimeoutMinSeconds, "1", "Job timeout floor",
		"Shortest timeout in seconds a scheduled job attempt may use; a shorter per-job timeout is raised to this.", 1, 86400},
	{scheduledtask.KeyTimeoutMaxSeconds, scheduledtask.EnvTimeoutMaxSeconds, "86400", "Job timeout ceiling",
		"Longest timeout in seconds a scheduled job attempt may use; the run is stopped at this even when the job asks for more.", 1, 604800},
	{scheduledtask.KeyRetentionMaxRuns, scheduledtask.EnvRetentionMaxRuns, "1000", "Job history cap (runs)",
		"Most runs kept per job, whatever the job's own keep runs value says.", 1, 100000},
	{scheduledtask.KeyRetentionMaxDays, scheduledtask.EnvRetentionMaxDays, "365", "Job history cap (days)",
		"Oldest run age in days kept per job, whatever the job's own keep days value says.", 1, 3650},
	{scheduledtask.KeyRetentionSweepMaxRows, scheduledtask.EnvRetentionSweepMaxRows, "500", "Job history prune batch",
		"Most history rows one prune pass deletes, so pruning a large backlog never locks the database for long.", 1, 100000},
	{scheduledtask.KeyFailureAlertWindowSecond, scheduledtask.EnvFailureAlertWindowSecond, "3600", "Job failure alert window",
		"Seconds between two failure alerts for the same job; failures inside the window are only in history.", 60, 604800},
	{scheduledtask.KeyFailureAlertBatch, scheduledtask.EnvFailureAlertBatch, "50", "Job failure alert batch",
		"Most failing jobs one job_failing alert rule reports per evaluation.", 1, 1000},
	{scheduledtask.KeyRetentionSkippedKeep, scheduledtask.EnvRetentionSkippedKeep, "20", "Job skipped run history",
		"Skipped runs kept per job, counted apart from real runs so overlap skips never push real history out.", 1, 1000},
}

func init() {
	for _, g := range jobGuardSettings {
		lo, hi := g.lo, g.hi
		runtimesettings.Register(runtimesettings.Definition{
			Key: g.key, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindInt,
			Default: g.def, Env: g.env, Apply: runtimesettings.ApplyLive, EnvLoose: true,
			CheckValue:  func(canonical string) error { return checkIntRange(canonical, lo, hi) },
			Title:       g.title,
			Description: g.desc + " Read on every run; the environment variable only seeds the default.",
		})
	}
}

// wireJobGuardSettings feeds the saved guard bounds to the scheduler, clamped
// so a loose env seed outside the range cannot widen them.
func wireJobGuardSettings(svc *runtimesettings.Service) {
	bounds := make(map[string]jobGuardSetting, len(jobGuardSettings))
	for _, g := range jobGuardSettings {
		bounds[g.key] = g
	}
	scheduledtask.SetGuardResolver(func(key string) int {
		g, ok := bounds[key]
		if !ok {
			return 0
		}
		return min(max(svc.Int(context.Background(), key), g.lo), g.hi)
	})
}

func checkIntRange(canonical string, lo, hi int) error {
	if n, err := strconv.Atoi(canonical); err != nil || n < lo || n > hi {
		return fmt.Errorf("value must be between %d and %d", lo, hi)
	}
	return nil
}
