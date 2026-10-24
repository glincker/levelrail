package cleanup

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const hoursPerDay = 24

// Start runs the scheduler until ctx ends. The first pass waits out the startup delay; each later pass
// waits the current interval, re-read every cycle so a changed setting applies without a restart.
func (c *Center) Start(ctx context.Context) {
	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.started = false
		c.nextRun = map[string]time.Time{}
		c.mu.Unlock()
	}()
	wait := c.cfg.Limits.StartupDelay
	for {
		c.planNext(ctx, c.cfg.Now().Add(wait))
		if !sleep(ctx, wait) {
			return
		}
		c.Tick(ctx)
		wait = c.interval(ctx)
	}
}

// eligible lists the targets a scheduled pass would run, in registration order.
func (c *Center) eligible(ctx context.Context) []Target {
	var out []Target
	for _, t := range c.cfg.Registry.Targets() {
		m := t.Meta()
		if m.ReadOnly || c.ModeOf(ctx, m) == ModeOff {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (c *Center) planNext(ctx context.Context, at time.Time) {
	plan := map[string]time.Time{}
	for i, t := range c.eligible(ctx) {
		plan[t.Meta().ID] = at.Add(time.Duration(i) * c.cfg.Limits.Stagger)
	}
	c.mu.Lock()
	c.nextRun = plan
	c.mu.Unlock()
}

// Tick runs one scheduled pass: every target whose mode is on (a real run) or preview (a dry run), spaced by the stagger.
func (c *Center) Tick(ctx context.Context) {
	if !c.cfg.Settings.Bool(ctx, runtimesettings.KeyCleanupEnabled) {
		return
	}
	c.pruneHistory(ctx)
	for i, t := range c.eligible(ctx) {
		if i > 0 && !sleep(ctx, c.cfg.Limits.Stagger) {
			return
		}
		if ctx.Err() != nil {
			return
		}
		c.runScheduled(ctx, t)
	}
}

func (c *Center) runScheduled(ctx context.Context, t Target) {
	m := t.Meta()
	mode := c.ModeOf(ctx, m)
	days := c.cfg.Settings.Int(ctx, m.PolicySetting)
	if days <= 0 || mode == ModeOff {
		return
	}
	if m.Floor > 0 && days < m.Floor {
		days = m.Floor
	}
	_, res, err := c.execute(ctx, t, days, RunOptions{DryRun: mode == ModePreview, Trigger: TriggerScheduled, Actor: "scheduler"})
	switch {
	case err != nil:
		c.cfg.Logger.Info("cleanup: scheduled run skipped", slog.String("target", m.ID), slog.String("reason", err.Error()))
	case res.Err != nil:
		c.cfg.Logger.Warn("cleanup: scheduled run failed", slog.String("target", m.ID), slog.String("error", res.Err.Error()))
	}
}

func (c *Center) pruneHistory(ctx context.Context) {
	cutoff := c.cfg.Now().Add(-time.Duration(c.cfg.Limits.HistoryDays) * hoursPerDay * time.Hour)
	if _, err := c.cfg.Store.PruneCleanupRuns(ctx, cutoff); err != nil {
		c.cfg.Logger.Warn("cleanup: prune history", slog.String("error", err.Error()))
	}
}

// sleep waits d or until ctx ends, and reports whether the full wait elapsed.
func sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
