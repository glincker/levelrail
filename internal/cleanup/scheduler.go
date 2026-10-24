package cleanup

import (
	"context"
	"log/slog"
	"maps"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const (
	hoursPerDay      = 24
	minSchedulerWait = time.Second
)

// Start runs the scheduler until ctx ends. Each target is due at its last run plus the interval, or at the
// startup delay plus its stagger slot before it has run; the schedule is re-read every cycle so a changed
// setting applies without a restart.
func (c *Center) Start(ctx context.Context) {
	c.mu.Lock()
	c.started = true
	c.bootedAt = c.cfg.Now()
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.started = false
		c.nextRun = map[string]time.Time{}
		c.mu.Unlock()
	}()
	for {
		plan := c.dueTimes(ctx)
		c.mu.Lock()
		c.nextRun = plan
		c.mu.Unlock()
		if !sleep(ctx, c.untilNext(ctx, plan)) {
			return
		}
		c.runDue(ctx)
	}
}

// untilNext is how long to wait for the earliest due target, or one interval when nothing is scheduled.
func (c *Center) untilNext(ctx context.Context, plan map[string]time.Time) time.Duration {
	if len(plan) == 0 || !c.cfg.Settings.Bool(ctx, runtimesettings.KeyCleanupEnabled) {
		return c.interval(ctx)
	}
	now := c.cfg.Now()
	var first time.Time
	for _, at := range plan {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}
	return max(first.Sub(now), minSchedulerWait)
}

// dueTimes plans when each eligible target runs next: last run plus interval, never earlier than the
// startup delay plus its stagger slot, which is also the time of a target that has never run.
func (c *Center) dueTimes(ctx context.Context) map[string]time.Time {
	last, err := c.lastRuns(ctx)
	if err != nil {
		c.cfg.Logger.Warn("cleanup: schedule history unreadable", slog.String("error", err.Error()))
	}
	interval := c.interval(ctx)
	c.mu.Lock()
	booted := c.bootedAt
	attempts := maps.Clone(c.attempted)
	c.mu.Unlock()
	if booted.IsZero() {
		booted = c.cfg.Now()
	}
	plan := map[string]time.Time{}
	for i, t := range c.eligible(ctx) {
		id := t.Meta().ID
		at := booted.Add(c.cfg.Limits.StartupDelay + time.Duration(i)*c.cfg.Limits.Stagger)
		if r, ok := last[id]; ok && r.FinishedAt.Add(interval).After(at) {
			at = r.FinishedAt.Add(interval)
		}
		if a, ok := attempts[id]; ok && a.Add(interval).After(at) {
			at = a.Add(interval)
		}
		plan[id] = at
	}
	return plan
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

// runDue runs the targets whose due time has passed.
func (c *Center) runDue(ctx context.Context) {
	if !c.cfg.Settings.Bool(ctx, runtimesettings.KeyCleanupEnabled) {
		return
	}
	c.pruneHistory(ctx)
	plan := c.dueTimes(ctx)
	now := c.cfg.Now()
	for _, t := range c.eligible(ctx) {
		if ctx.Err() != nil {
			return
		}
		if at, ok := plan[t.Meta().ID]; ok && !at.After(now) {
			c.runScheduled(ctx, t)
		}
	}
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
	c.mu.Lock()
	c.attempted[m.ID] = c.cfg.Now()
	c.mu.Unlock()
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
