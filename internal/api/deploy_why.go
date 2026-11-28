package api

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/deploywhy"
	"github.com/glincker/levelrail/internal/lognorm"
	"github.com/glincker/levelrail/internal/store"
)

// DeployLogTemplateStore persists mined log clusters per deploy. *store.DB satisfies it.
type DeployLogTemplateStore interface {
	ReplaceDeployLogTemplates(ctx context.Context, deployID string, rows []store.DeployLogTemplate) error
	ListDeployLogTemplates(ctx context.Context, deployID string) ([]store.DeployLogTemplate, error)
}

// handleGetDeployWhy handles GET /api/v1/deploys/{id}/why: the verdict for one deploy
// from its exit capture, last probe and log clusters compared with the last good deploy.
func (rt *Router) handleGetDeployWhy(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	ctx := r.Context()
	in := deploywhy.Input{DeployID: a.ID, Failed: a.Status == store.DeployAttemptStatusFailed || a.Status == store.DeployAttemptStatusRefused}
	if rt.deployExits != nil {
		if e, err := rt.deployExits.GetDeployExit(ctx, a.ServiceName, a.ID); err != nil {
			rt.logger.Warn("api: deploy why: load exit failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		} else if e != nil {
			code := e.ExitCode
			in.ExitCode, in.OOMKilled = &code, e.OOMKilled
			in.Failed = in.Failed || code != 0
		}
	}
	in.Health = rt.whyLastProbe(ctx, a.ID)
	in.Templates = rt.deployTemplates(ctx, a)
	if base, found := rt.lastGoodAttempt(ctx, a); found {
		in.BaselineID = base.ID
		in.BaselineTemplate = rt.deployTemplates(ctx, base)
		in.Changes = whyChanges(a, base)
	}
	if in.Failed {
		in.Cause = rt.whyCause(ctx, a)
		in.TailLines = rt.whyTailLines(ctx, a)
		in.TailMax = failureOptions().MaxLines
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, deploywhy.Compute(in))
}

func (rt *Router) whyLastProbe(ctx context.Context, id string) *deploywhy.Health {
	if rt.probeAttempts == nil {
		return nil
	}
	probes, err := rt.probeAttempts.ListProbeAttempts(ctx, id)
	if err != nil || len(probes) == 0 {
		return nil
	}
	last := probes[0]
	for _, p := range probes {
		if p.ProbedAt.After(last.ProbedAt) {
			last = p
		}
	}
	return &deploywhy.Health{Target: lognorm.Scrub(last.Target), Success: last.Success, StatusCode: last.StatusCode, Error: lognorm.Scrub(last.Error)}
}

func (rt *Router) lastGoodAttempt(ctx context.Context, a store.DeployAttempt) (store.DeployAttempt, bool) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, a.ServiceName)
	if err != nil {
		return store.DeployAttempt{}, false
	}
	for _, c := range attempts {
		if c.ID != a.ID && c.Status == store.DeployAttemptStatusSucceeded && c.StartedAt.Before(a.StartedAt) {
			return c, true
		}
	}
	return store.DeployAttempt{}, false
}

// deployTemplates returns the stored clusters of a deploy, mining and storing them on first
// read once the deploy has settled. Errors are logged and never reach the caller.
func (rt *Router) deployTemplates(ctx context.Context, a store.DeployAttempt) []deploywhy.Template {
	if rt.deployLogTemplates == nil {
		return nil
	}
	rows, err := rt.deployLogTemplates.ListDeployLogTemplates(ctx, a.ID)
	if err != nil {
		rt.logger.Warn("api: deploy why: list templates failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		return nil
	}
	if len(rows) == 0 && rt.whyMineable(a) && !rt.whyEmpty.has(a.ID) {
		rows = rt.captureDeployTemplates(ctx, a)
		if len(rows) == 0 {
			rt.whyEmpty.add(a.ID)
		}
	}
	// Stored rows predate newer masking rules, so scrub again on the way out.
	out := make([]deploywhy.Template, 0, len(rows))
	for _, r := range rows {
		out = append(out, deploywhy.Template{TemplateID: r.TemplateID, Template: lognorm.Scrub(r.Template), Count: r.Count, Sample: lognorm.Scrub(r.Sample)})
	}
	return out
}

// whyMineable is true once the release log tail window has passed, so the mined
// clusters are final and safe to store.
func (rt *Router) whyMineable(a store.DeployAttempt) bool {
	return a.FinishedAt != nil && time.Now().After(a.FinishedAt.Add(releaseLogTail))
}

// whyEmptyCache remembers deploys that mined to nothing so repeat reads skip the
// log and Docker queries. It is bounded and rebuilt after a restart.
type whyEmptyCache struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

const whyEmptyCacheMax = 4096

func (c *whyEmptyCache) has(id string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.ids[id]
	return ok
}

func (c *whyEmptyCache) add(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil || len(c.ids) >= whyEmptyCacheMax {
		c.ids = map[string]struct{}{}
	}
	c.ids[id] = struct{}{}
}

func (rt *Router) captureDeployTemplates(ctx context.Context, a store.DeployAttempt) []store.DeployLogTemplate {
	lines := rt.attemptExitLines(ctx, a)
	if len(lines) == 0 {
		lines, _ = rt.scopedReleaseLogLines(ctx, a)
	}
	if len(lines) == 0 {
		return nil
	}
	m := lognorm.NewMiner(lognorm.MinerConfig{})
	for _, e := range lognorm.Normalize(lines, nil) {
		m.Add(e)
	}
	var rows []store.DeployLogTemplate
	for _, t := range m.Templates() {
		rows = append(rows, store.DeployLogTemplate{DeployID: a.ID, TemplateID: t.ID, Template: t.Template, Count: t.Count, Sample: t.Sample})
	}
	if err := rt.deployLogTemplates.ReplaceDeployLogTemplates(ctx, a.ID, rows); err != nil {
		rt.logger.Warn("api: deploy why: store templates failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
	}
	return rows
}
