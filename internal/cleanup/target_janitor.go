package cleanup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/janitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	janitorReportLookback = 20
	hoursPerDayInt        = 24
)

// JanitorSource reads the janitor policy and its recorded passes. *store.DB satisfies it.
type JanitorSource interface {
	janitor.PolicyStore
	ListJanitorRuns(ctx context.Context, limit int) ([]store.JanitorRun, error)
}

// janitorTarget shows one janitor stage. Its policy and runs stay in the janitor, so it only reports.
type janitorTarget struct {
	src   JanitorSource
	stage janitor.Stage
	meta  Meta
	days  func(janitor.Policy) int
}

func janitorTargets(src JanitorSource) []Target {
	mk := func(id string, cat Category, stage janitor.Stage, title, desc string, days func(janitor.Policy) int) Target {
		m := baseMeta(id, cat, title, desc, "")
		m.ReadOnly, m.Link, m.Destructive = true, linkSettingsJanitor, true
		return &janitorTarget{src: src, stage: stage, meta: m, days: days}
	}
	return []Target{
		mk("docker.containers", CategoryDocker, janitor.StageContainers, "Stopped containers", "Stopped containers no desired app still needs. Managed by the janitor.", func(janitor.Policy) int { return 0 }),
		mk("docker.images", CategoryDocker, janitor.StageImages, "Unused images", "Dangling and unused Docker images. Managed by the janitor.", func(janitor.Policy) int { return 0 }),
		mk("docker.build_cache", CategoryDocker, janitor.StageBuildCache, "Build cache", "Build cache over its budget or older than its age limit. Managed by the janitor.",
			func(p janitor.Policy) int { return (p.BuildCacheMaxAgeHours + hoursPerDayInt - 1) / hoursPerDayInt }),
		mk("docker.volumes", CategoryDocker, janitor.StageVolumes, "Orphaned volumes", "Volumes no app references, held in quarantine first. Managed by the janitor.",
			func(p janitor.Policy) int { return p.VolumeQuarantineDays }),
		mk("deploys.attempts", CategoryDeploys, janitor.StageAttempts, "Old deploy attempts", "Finished deploy attempts past the retention window. Managed by the janitor.",
			func(p janitor.Policy) int { return p.AttemptRetentionDays }),
	}
}

func (t *janitorTarget) Meta() Meta { return t.meta }

func (t *janitorTarget) State(ctx context.Context) (ManagedState, error) {
	p, err := janitor.LoadPolicy(ctx, t.src)
	if err != nil {
		return ManagedState{}, err
	}
	st := ManagedState{Days: t.days(p), DryRun: p.DryRun, Mode: ModeOn}
	switch {
	case !p.Enabled || !p.StageEnabled(t.stage):
		st.Mode = ModeOff
	case p.DryRun:
		st.Mode = ModePreview
	}
	return st, nil
}

// lastImpact sums what the newest pass that touched this stage removed or would remove.
func (t *janitorTarget) lastImpact(ctx context.Context) (Impact, error) {
	runs, err := t.src.ListJanitorRuns(ctx, janitorReportLookback)
	if err != nil {
		return Impact{}, fmt.Errorf("cleanup: janitor history: %w", err)
	}
	for _, r := range runs {
		var stages []janitor.StageResult
		if json.Unmarshal([]byte(r.Body), &stages) != nil {
			continue
		}
		for _, s := range stages {
			if s.Stage != t.stage {
				continue
			}
			var imp Impact
			for _, it := range s.Items {
				if it.Action == janitor.ActionKept {
					continue
				}
				imp.Rows += max(int64(it.Count), 1)
				imp.Bytes += it.SizeBytes
			}
			if imp.Rows > 0 {
				return imp, nil
			}
		}
	}
	return Impact{}, nil
}

func (t *janitorTarget) Measure(ctx context.Context) (Usage, error) {
	imp, err := t.lastImpact(ctx)
	return Usage{Rows: imp.Rows, Bytes: imp.Bytes}, err
}

func (t *janitorTarget) Preview(ctx context.Context, _ int) (Impact, error) { return t.lastImpact(ctx) }

func (t *janitorTarget) Run(context.Context, int, RunOptions) Result {
	return Result{Err: ErrReadOnly}
}
