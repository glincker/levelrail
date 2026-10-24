// Package janitor runs bounded, level-triggered cleanup passes on the control
// plane's own node: unused Docker data, build cache over budget, orphaned
// volumes (quarantined first) and old failed deploy attempts.
package janitor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Stage names one cleanup category.
type Stage string

// Cleanup stages.
const (
	StageContainers Stage = "containers"
	StageImages     Stage = "images"
	StageBuildCache Stage = "build_cache"
	StageVolumes    Stage = "volumes"
	StageAttempts   Stage = "deploy_attempts"
	StageTokens     Stage = "api_tokens"
)

// AllStages lists every stage in the order a pass runs them.
var AllStages = []Stage{StageContainers, StageImages, StageBuildCache, StageVolumes, StageAttempts, StageTokens}

// Policy is the operator-editable cleanup configuration.
type Policy struct {
	Enabled bool `json:"enabled"`
	// DryRun makes scheduled passes report without removing anything.
	DryRun          bool `json:"dry_run"`
	IntervalMinutes int  `json:"interval_minutes"`
	// DiskFreeBudgetPercent is the free space below which size-driven stages run on a schedule.
	DiskFreeBudgetPercent float64 `json:"disk_free_budget_percent"`
	BuildCacheBudgetMB    int64   `json:"build_cache_budget_mb"`
	BuildCacheMaxAgeHours int     `json:"build_cache_max_age_hours"`
	// VolumeQuarantineDays is how long an orphaned volume is held before it may be deleted.
	VolumeQuarantineDays int `json:"volume_quarantine_days"`
	// DeleteOrphanVolumes lets a pass delete volumes whose quarantine ended; off keeps them listed only.
	DeleteOrphanVolumes  bool    `json:"delete_orphan_volumes"`
	AttemptRetentionDays int     `json:"attempt_retention_days"`
	AttemptKeepPerApp    int     `json:"attempt_keep_per_app"`
	DisabledStages       []Stage `json:"disabled_stages"`
}

// DefaultPolicy reports only until an operator turns removal on.
func DefaultPolicy() Policy {
	return Policy{
		Enabled: true, DryRun: true, IntervalMinutes: 60,
		DiskFreeBudgetPercent: 15, BuildCacheBudgetMB: 10240, BuildCacheMaxAgeHours: 168,
		VolumeQuarantineDays: 14, AttemptRetentionDays: 30, AttemptKeepPerApp: 20,
		DisabledStages: []Stage{},
	}
}

// Validate rejects values that would make a pass unsafe or meaningless.
func (p Policy) Validate() error {
	switch {
	case p.IntervalMinutes < 5 || p.IntervalMinutes > 7*24*60:
		return fmt.Errorf("interval_minutes must be between 5 and %d", 7*24*60)
	case p.DiskFreeBudgetPercent < 1 || p.DiskFreeBudgetPercent > 90:
		return fmt.Errorf("disk_free_budget_percent must be between 1 and 90")
	case p.BuildCacheBudgetMB < 0:
		return fmt.Errorf("build_cache_budget_mb must not be negative")
	case p.BuildCacheMaxAgeHours < 1:
		return fmt.Errorf("build_cache_max_age_hours must be at least 1")
	case p.VolumeQuarantineDays < 1:
		return fmt.Errorf("volume_quarantine_days must be at least 1")
	case p.AttemptRetentionDays < 1:
		return fmt.Errorf("attempt_retention_days must be at least 1")
	case p.AttemptKeepPerApp < 1:
		return fmt.Errorf("attempt_keep_per_app must be at least 1")
	}
	for _, s := range p.DisabledStages {
		if !validStage(s) {
			return fmt.Errorf("unknown stage %q", s)
		}
	}
	return nil
}

func validStage(s Stage) bool {
	for _, k := range AllStages {
		if k == s {
			return true
		}
	}
	return false
}

func (p Policy) stageOn(s Stage) bool {
	for _, d := range p.DisabledStages {
		if d == s {
			return false
		}
	}
	return true
}

// PolicyStore persists the policy as JSON.
type PolicyStore interface {
	GetJanitorPolicy(ctx context.Context) (string, bool, error)
	SetJanitorPolicy(ctx context.Context, body, actor string, now time.Time) error
}

// LoadPolicy returns the saved policy, or the default when none was saved.
func LoadPolicy(ctx context.Context, s PolicyStore) (Policy, error) {
	body, ok, err := s.GetJanitorPolicy(ctx)
	if err != nil {
		return Policy{}, fmt.Errorf("janitor: load policy: %w", err)
	}
	p := DefaultPolicy()
	if !ok {
		return p, nil
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		return Policy{}, fmt.Errorf("janitor: decode policy: %w", err)
	}
	if p.DisabledStages == nil {
		p.DisabledStages = []Stage{}
	}
	return p, nil
}

// SavePolicy validates and stores p.
func SavePolicy(ctx context.Context, s PolicyStore, p Policy, actor string, now time.Time) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("janitor: %w", err)
	}
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("janitor: encode policy: %w", err)
	}
	if err := s.SetJanitorPolicy(ctx, string(body), actor, now); err != nil {
		return fmt.Errorf("janitor: save policy: %w", err)
	}
	return nil
}
