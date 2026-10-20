package janitor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/ids"
)

// Item actions a stage reports.
const (
	ActionRemoved           = "removed"
	ActionWouldRemove       = "would_remove"
	ActionQuarantined       = "quarantined"
	ActionWouldQuarantine   = "would_quarantine"
	ActionKept              = "kept"
	StatusRan               = "ran"
	StatusSkipped           = "skipped"
	StatusFailed            = "failed"
	TriggerScheduled        = "scheduled"
	TriggerManual           = "manual"
	TriggerDiskFix          = "disk_fix"
	TriggerNodeGuardrail    = "node_guardrail"
	historyKeep             = 200
	maxAttemptsPerPass      = 500
	maxVolumeRemovalsPerRun = 5
	volumeKind              = "volume"
)

// ErrBusy means another pass is already running.
var ErrBusy = errors.New("janitor: a cleanup pass is already running")

// Item is one thing a stage removed, would remove, or deliberately kept.
type Item struct {
	Name      string `json:"name"`
	Detail    string `json:"detail,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	Action    string `json:"action"`
}

// StageResult is what one stage did in a pass.
type StageResult struct {
	Stage          Stage  `json:"stage"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	Items          []Item `json:"items"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
	Error          string `json:"error,omitempty"`
}

// Report is one pass.
type Report struct {
	ID             string        `json:"id"`
	StartedAt      time.Time     `json:"started_at"`
	Trigger        string        `json:"trigger"`
	DryRun         bool          `json:"dry_run"`
	FreePercent    float64       `json:"free_percent"`
	Pressure       bool          `json:"pressure"`
	Stages         []StageResult `json:"stages"`
	ReclaimedBytes int64         `json:"reclaimed_bytes"`
	Error          string        `json:"error,omitempty"`
}

// Docker is the daemon surface a pass uses. *docker.Client satisfies it.
type Docker interface {
	DiskUsage(ctx context.Context) (docker.DiskUsage, error)
	ListAllImages(ctx context.Context) ([]docker.ImageRecord, error)
	PruneContainers(ctx context.Context, keep []string) (docker.PruneContainersResult, error)
	PruneDanglingImages(ctx context.Context) (docker.PruneImagesResult, error)
	PruneAnonymousVolumes(ctx context.Context) (docker.PruneVolumesResult, error)
	BuildCacheRecords(ctx context.Context) ([]docker.BuildCacheRecord, error)
	PruneBuildCacheOlderThan(ctx context.Context, olderThan time.Duration) (docker.PruneBuildCacheResult, error)
}

// Store is the persistence a pass needs. *store.DB satisfies it.
type Store interface {
	PolicyStore
	InsertJanitorRun(ctx context.Context, r store.JanitorRun, keep int) error
	ListJanitorRuns(ctx context.Context, limit int) ([]store.JanitorRun, error)
	UpsertJanitorQuarantine(ctx context.Context, q store.JanitorQuarantine) error
	ListJanitorQuarantine(ctx context.Context, kind string) ([]store.JanitorQuarantine, error)
	DeleteJanitorQuarantine(ctx context.Context, kind, name string) error
	ListPrunableDeployAttempts(ctx context.Context, cutoff time.Time, keep, limit int) ([]store.PrunableAttempt, error)
	DeleteDeployAttempts(ctx context.Context, ids []string) error
}

// Deps are the engine's collaborators.
type Deps struct {
	Store  Store
	Docker Docker
	// DiskFree reports free and total bytes of the volume Docker data lives on.
	DiskFree func(ctx context.Context) (free, total int64, err error)
	// DesiredContainers lists container names the reconciler still wants, which a prune must keep.
	DesiredContainers func(ctx context.Context) ([]string, error)
	// OrphanVolumes lists named volumes no desired state or container references.
	OrphanVolumes func(ctx context.Context) ([]docker.NamedVolume, error)
	RemoveVolume  func(ctx context.Context, name string) error
	Logger        *slog.Logger
	Now           func() time.Time
}

// Engine runs cleanup passes one at a time.
type Engine struct {
	Deps
	mu      sync.Mutex
	lastRun time.Time
}

// New builds an Engine.
func New(d Deps) *Engine {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Engine{Deps: d}
}

// Options say how one pass runs.
type Options struct {
	Trigger string
	DryRun  bool
	// Force runs the size-driven stages even when free space is above budget.
	Force bool
	// Stages limits the pass to these stages; empty runs every stage the policy enables.
	Stages []Stage
}

func newID() string {
	return ids.MustHex(8)
}

// Run executes one pass under the saved policy and records it in history.
func (e *Engine) Run(ctx context.Context, opts Options) (Report, error) {
	if !e.mu.TryLock() {
		return Report{}, ErrBusy
	}
	defer e.mu.Unlock()

	policy, err := LoadPolicy(ctx, e.Store)
	if err != nil {
		return Report{}, err
	}
	now := e.Now()
	rep := Report{ID: newID(), StartedAt: now, Trigger: opts.Trigger, DryRun: opts.DryRun, FreePercent: -1, Stages: []StageResult{}}
	if free, total, derr := e.DiskFree(ctx); derr == nil && total > 0 {
		rep.FreePercent = float64(free) * 100 / float64(total)
		rep.Pressure = rep.FreePercent < policy.DiskFreeBudgetPercent
	}
	pressure := rep.Pressure || opts.Force

	steps := []struct {
		stage Stage
		run   func() StageResult
	}{
		{StageContainers, func() StageResult { return e.containers(ctx, opts.DryRun, pressure) }},
		{StageImages, func() StageResult { return e.images(ctx, opts.DryRun, pressure) }},
		{StageBuildCache, func() StageResult { return e.buildCache(ctx, policy, opts.DryRun, pressure, now) }},
		{StageVolumes, func() StageResult { return e.volumes(ctx, policy, opts.DryRun, pressure, now) }},
		{StageAttempts, func() StageResult { return e.attempts(ctx, policy, opts.DryRun, now) }},
	}
	for _, s := range steps {
		if len(opts.Stages) > 0 && !slices.Contains(opts.Stages, s.stage) {
			continue
		}
		if len(opts.Stages) == 0 && !policy.stageOn(s.stage) {
			rep.Stages = append(rep.Stages, StageResult{Stage: s.stage, Status: StatusSkipped, Reason: "disabled in policy", Items: []Item{}})
			continue
		}
		res := s.run()
		if res.Items == nil {
			res.Items = []Item{}
		}
		rep.ReclaimedBytes += res.ReclaimedBytes
		if res.Error != "" && rep.Error == "" {
			rep.Error = string(res.Stage) + ": " + res.Error
		}
		rep.Stages = append(rep.Stages, res)
	}
	e.lastRun = now
	e.record(ctx, rep)
	return rep, nil
}

func (e *Engine) record(ctx context.Context, rep Report) {
	if rep.DryRun || (rep.Trigger == TriggerScheduled && rep.Error == "" && !hasItems(rep)) {
		return
	}
	body, err := json.Marshal(rep.Stages)
	if err != nil {
		e.Logger.Error("janitor: encode run", slog.String("run_id", rep.ID), slog.String("error", err.Error()))
		return
	}
	run := store.JanitorRun{ID: rep.ID, StartedAt: rep.StartedAt, Trigger: rep.Trigger, DryRun: rep.DryRun,
		ReclaimedBytes: rep.ReclaimedBytes, FreePercent: rep.FreePercent, Body: string(body), Error: rep.Error}
	if err := e.Store.InsertJanitorRun(ctx, run, historyKeep); err != nil {
		e.Logger.Error("janitor: record run", slog.String("run_id", rep.ID), slog.String("error", err.Error()))
	}
}

func hasItems(rep Report) bool {
	for _, s := range rep.Stages {
		for _, it := range s.Items {
			if it.Action != ActionKept {
				return true
			}
		}
	}
	return false
}

// Due reports whether a scheduled pass should run now.
func (e *Engine) Due(p Policy) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.lastRun.IsZero() || e.Now().Sub(e.lastRun) >= time.Duration(p.IntervalMinutes)*time.Minute
}

// History returns the newest runs.
func (e *Engine) History(ctx context.Context, limit int) ([]Report, error) {
	runs, err := e.Store.ListJanitorRuns(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("janitor: history: %w", err)
	}
	out := make([]Report, 0, len(runs))
	for _, r := range runs {
		rep := Report{ID: r.ID, StartedAt: r.StartedAt, Trigger: r.Trigger, DryRun: r.DryRun,
			FreePercent: r.FreePercent, ReclaimedBytes: r.ReclaimedBytes, Error: r.Error, Stages: []StageResult{}}
		if err := json.Unmarshal([]byte(r.Body), &rep.Stages); err != nil {
			e.Logger.Warn("janitor: decode stored run", slog.String("run_id", r.ID), slog.String("error", err.Error()))
		}
		out = append(out, rep)
	}
	return out, nil
}
