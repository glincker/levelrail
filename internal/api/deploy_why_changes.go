package api

import (
	"context"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/deploywhy"
	"github.com/GLINCKER/levelrail/internal/lognorm"
	"github.com/GLINCKER/levelrail/internal/store"
)

// whyChanges lists what differs between two attempts. Env entries are key names
// only; a literal value is compared but never copied into the result.
func whyChanges(cur, base store.DeployAttempt) deploywhy.Changes {
	var c deploywhy.Changes
	if cur.Image != base.Image {
		c.Image = &deploywhy.Change{From: lognorm.Scrub(base.Image), To: lognorm.Scrub(cur.Image)}
	}
	if cur.Snapshot.Port != base.Snapshot.Port {
		c.Port = &deploywhy.Change{From: strconv.Itoa(base.Snapshot.Port), To: strconv.Itoa(cur.Snapshot.Port)}
	}
	old := make(map[string]store.DeployAttemptEnvKey, len(base.Snapshot.Env))
	for _, e := range base.Snapshot.Env {
		old[e.Key] = e
	}
	seen := make(map[string]bool, len(cur.Snapshot.Env))
	for _, e := range cur.Snapshot.Env {
		seen[e.Key] = true
		prev, ok := old[e.Key]
		switch {
		case !ok:
			c.EnvAdded = append(c.EnvAdded, e.Key)
		case prev.Kind != e.Kind || prev.Value != e.Value:
			c.EnvChanged = append(c.EnvChanged, e.Key)
		}
	}
	for _, e := range base.Snapshot.Env {
		if !seen[e.Key] {
			c.EnvRemoved = append(c.EnvRemoved, e.Key)
		}
	}
	return c
}

// whyCause classifies the attempt with its stored signals and returns the cause with its fix text.
func (rt *Router) whyCause(ctx context.Context, a store.DeployAttempt) *deploywhy.Cause {
	f, ok := rt.classifyAttempt(ctx, a, nil, false)
	if !ok {
		return nil
	}
	return &deploywhy.Cause{Code: f.Code, Text: lognorm.Scrub(f.Cause), Fix: lognorm.Scrub(f.SuggestedFix)}
}

// whyTailLines returns the failing container's output, secret-masked, newest last.
func (rt *Router) whyTailLines(ctx context.Context, a store.DeployAttempt) []string {
	lines := rt.attemptExitLines(ctx, a)
	if len(lines) == 0 {
		lines, _ = rt.scopedReleaseLogLines(ctx, a)
	}
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, lognorm.Scrub(l))
	}
	return out
}
