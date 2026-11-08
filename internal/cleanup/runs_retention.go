package cleanup

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// RunTrimmer is an optional RunStore capability: keep only the newest runs of one target.
type RunTrimmer interface {
	TrimCleanupRuns(ctx context.Context, target string, keep int) (int64, error)
}

// trimRuns enforces cleanup.runs_keep per target; 0 or a store without trimming leaves history to the age limit.
func (c *Center) trimRuns(ctx context.Context) {
	keep := c.cfg.Settings.Int(ctx, runtimesettings.KeyCleanupRunsKeep)
	tr, ok := c.cfg.Store.(RunTrimmer)
	if keep <= 0 || !ok {
		return
	}
	for _, t := range c.cfg.Registry.Targets() {
		id := t.Meta().ID
		if _, err := tr.TrimCleanupRuns(ctx, id, keep); err != nil {
			c.cfg.Logger.Warn("cleanup: trim runs", slog.String("target", id), slog.String("error", err.Error()))
		}
	}
}

// failureStreak counts the newest real runs of a target that failed in a row, looking at most limit runs deep.
func (c *Center) failureStreak(ctx context.Context, id string, limit int) (int, error) {
	dry := false
	runs, err := c.cfg.Store.ListCleanupRuns(ctx, store.CleanupRunFilter{Target: id, DryRun: &dry, Limit: limit})
	if err != nil {
		return 0, fmt.Errorf("cleanup: failure streak: %w", err)
	}
	n := 0
	for _, r := range runs {
		if r.DryRun {
			continue
		}
		if r.Error == "" {
			break
		}
		n++
	}
	return n, nil
}

// paused reports whether the scheduler should skip a target after too many failed runs in a row.
func (c *Center) paused(ctx context.Context, id string) bool {
	limit := c.cfg.Settings.Int(ctx, runtimesettings.KeyCleanupMaxFailures)
	if limit <= 0 {
		return false
	}
	n, err := c.failureStreak(ctx, id, limit)
	return err == nil && n >= limit
}

// pauseReached is true on exactly the run that made the streak hit the limit, so the alert goes out once.
func (c *Center) pauseReached(ctx context.Context, id string) bool {
	limit := c.cfg.Settings.Int(ctx, runtimesettings.KeyCleanupMaxFailures)
	if limit <= 0 {
		return false
	}
	n, err := c.failureStreak(ctx, id, limit+1)
	return err == nil && n == limit
}

func (c *Center) sendPauseNotice(ctx context.Context, m Meta, rec store.CleanupRun) {
	if c.cfg.Notify == nil {
		return
	}
	limit := c.cfg.Settings.Int(ctx, runtimesettings.KeyCleanupMaxFailures)
	text := fmt.Sprintf("Cleanup of %s paused after %d failed runs in a row: %s", m.Title, limit, rec.Error)
	if err := c.cfg.Notify(ctx, text); err != nil {
		c.cfg.Logger.Warn("cleanup: pause notice not delivered", slog.String("target", m.ID), slog.String("error", err.Error()))
	}
}
