package objectstore

import (
	"context"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxPrunePerRun = 500

// Run checks for due policies every Opts.Tick until ctx ends. With no
// policies a tick is one small query, so idle cost stays near zero.
func (a *Archiver) Run(ctx context.Context) {
	if err := a.Store.FailStaleLogArchiveRuns(ctx, a.stamp(a.Now())); err != nil {
		a.Logger.Error("log archive: reset interrupted runs failed", slog.String("error", err.Error()))
	}
	t := time.NewTicker(a.Opts.Tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.RunDue(ctx)
		}
	}
}

// RunDue runs every enabled policy whose interval has elapsed since its last run.
func (a *Archiver) RunDue(ctx context.Context) {
	policies, err := a.Store.ListLogArchivePolicies(ctx)
	if err != nil {
		a.Logger.Error("log archive: list policies failed", slog.String("error", err.Error()))
		return
	}
	now := a.Now()
	for _, p := range policies {
		if ctx.Err() != nil {
			return
		}
		if !p.Enabled || !isDue(p, now) {
			continue
		}
		if err := a.RunPolicy(ctx, p); err != nil {
			a.Logger.Warn("log archive: policy run failed", slog.String("policy_id", p.ID), slog.String("app", p.AppName))
		}
	}
}

func isDue(p store.LogArchivePolicy, now time.Time) bool {
	if p.LastRunAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, p.LastRunAt)
	if err != nil {
		return true
	}
	return now.Sub(last) >= time.Duration(p.IntervalSeconds)*time.Second
}

// prune deletes archived objects older than the policy's retention. The
// global policy leaves apps that have their own policy alone.
func (a *Archiver) prune(ctx context.Context, cl *Client, p store.LogArchivePolicy) error {
	prefix := a.Opts.Prefix + "/"
	skip := map[string]bool{}
	if p.AppName != "" {
		prefix = AppPrefix(a.Opts.Prefix, p.AppName)
	} else {
		policies, err := a.Store.ListLogArchivePolicies(ctx)
		if err != nil {
			return err
		}
		for _, o := range policies {
			if o.AppName != "" {
				skip[sanitizeSegment(o.AppName)] = true
			}
		}
	}
	cutoff := a.Now().Add(-time.Duration(p.RetentionDays) * 24 * time.Hour)
	deleted, token := 0, ""
	for deleted < maxPrunePerRun {
		page, err := cl.List(ctx, prefix, token, 1000)
		if err != nil {
			return err
		}
		for _, o := range page.Objects {
			if app, ok := appOfKey(a.Opts.Prefix, o.Key); ok && skip[app] {
				continue
			}
			if o.LastModified.Before(cutoff) && deleted < maxPrunePerRun {
				if err := cl.Delete(ctx, o.Key); err != nil {
					return err
				}
				deleted++
			}
		}
		if page.Next == "" {
			return nil
		}
		token = page.Next
	}
	return nil
}

// HealthSource adapts stored policies to the alerting engine.
type HealthSource struct {
	Store interface {
		ListLogArchivePolicies(ctx context.Context) ([]store.LogArchivePolicy, error)
	}
}

// LogArchiveHealth implements alerting.LogArchiveSource.
func (h HealthSource) LogArchiveHealth(ctx context.Context) ([]alerting.LogArchiveHealth, error) {
	policies, err := h.Store.ListLogArchivePolicies(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]alerting.LogArchiveHealth, 0, len(policies))
	for _, p := range policies {
		created, _ := time.Parse(time.RFC3339, p.CreatedAt)
		success, _ := time.Parse(time.RFC3339, p.LastSuccessAt)
		out = append(out, alerting.LogArchiveHealth{
			Scope: p.AppName, Enabled: p.Enabled, Interval: time.Duration(p.IntervalSeconds) * time.Second,
			CreatedAt: created, LastSuccessAt: success, LastError: p.LastError,
		})
	}
	return out, nil
}
