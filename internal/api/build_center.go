package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	buildCenterDefaultLimit = 100
	buildCenterMaxLimit     = 500
	buildFixPruneCache      = "prune_cache"
	buildRunMatchSlack      = 30 * time.Second
)

// BuildLoadSource reports per-node build slot usage.
type BuildLoadSource interface {
	Load() []build.NodeLoad
}

// SetBuildLoad enables live slot usage in the build center.
func (rt *Router) SetBuildLoad(s BuildLoadSource) { rt.buildLoad = s }

type buildHint struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Explanation string `json:"explanation"`
	Suggestion  string `json:"suggestion,omitempty"`
	Fix         string `json:"fix,omitempty"`
}

type buildRow struct {
	ID              string     `json:"id"`
	App             string     `json:"app"`
	Status          string     `json:"status"`
	Source          string     `json:"source"`
	Branch          string     `json:"branch,omitempty"`
	CommitSHA       string     `json:"commit_sha,omitempty"`
	Image           string     `json:"image,omitempty"`
	QueuedAt        *time.Time `json:"queued_at,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty"`
	DurationSeconds *float64   `json:"duration_seconds,omitempty"`
	QueuePosition   int        `json:"queue_position,omitempty"`
	CacheHitRatio   *float64   `json:"cache_hit_ratio,omitempty"`
	ImageBytes      int64      `json:"image_bytes,omitempty"`
	ImageDeltaBytes *int64     `json:"image_delta_bytes,omitempty"`
	Node            string     `json:"node,omitempty"`
	Platform        string     `json:"platform,omitempty"`
	Error           string     `json:"error,omitempty"`
	Hint            *buildHint `json:"hint,omitempty"`
	LogsPath        string     `json:"logs_path"`
	CanCancel       bool       `json:"can_cancel"`
	CanRetry        bool       `json:"can_retry"`
}

type buildCounts struct {
	Queued   int `json:"queued"`
	Running  int `json:"running"`
	Finished int `json:"finished"`
	Failed   int `json:"failed"`
}

type buildNodeLoadResource struct {
	Node    string `json:"node"`
	Running int    `json:"running"`
	Waiting int    `json:"waiting"`
	Limit   int    `json:"limit"`
}

type buildPlatforms struct {
	Host          string `json:"host"`
	MultiPlatform bool   `json:"multi_platform"`
}

type buildCenterResponse struct {
	Rows      []buildRow              `json:"rows"`
	Counts    buildCounts             `json:"counts"`
	Nodes     []buildNodeLoadResource `json:"nodes"`
	Limit     int                     `json:"max_concurrent_per_node"`
	Platforms buildPlatforms          `json:"platforms"`
}

func isQueuedStatus(s string) bool {
	return s == store.DeployAttemptStatusQueued || s == store.DeployAttemptStatusHeld
}

func isFinishedStatus(s string) bool {
	return s == store.DeployAttemptStatusSucceeded || s == store.DeployAttemptStatusFailed ||
		s == store.DeployAttemptStatusCanceled || s == store.DeployAttemptStatusSuperseded
}

// matchBuildRuns pairs each attempt with the build run of the same app and
// tag that started closest after it, using every run at most once.
func matchBuildRuns(attempts []store.DeployAttempt, runs []store.BuildRun) map[string]store.BuildRun {
	byTag := map[string][]store.BuildRun{}
	for _, r := range runs {
		byTag[r.App+"\x00"+r.Tag] = append(byTag[r.App+"\x00"+r.Tag], r)
	}
	used := map[string]bool{}
	out := map[string]store.BuildRun{}
	ordered := append([]store.DeployAttempt(nil), attempts...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].StartedAt.Before(ordered[j].StartedAt) })
	for _, a := range ordered {
		if a.Image == "" || isQueuedStatus(a.Status) {
			continue
		}
		bestKey, best := "", store.BuildRun{}
		var bestGap time.Duration = -1
		for _, r := range byTag[a.ServiceName+"\x00"+a.Image] {
			key := r.App + r.Tag + r.StartedAt.Format(time.RFC3339Nano)
			gap := r.StartedAt.Sub(a.StartedAt)
			if used[key] || gap < -buildRunMatchSlack {
				continue
			}
			if gap < 0 {
				gap = -gap
			}
			if bestGap < 0 || gap < bestGap {
				bestKey, best, bestGap = key, r, gap
			}
		}
		if bestGap >= 0 {
			used[bestKey] = true
			out[a.ID] = best
		}
	}
	return out
}

// imageDeltas maps a successful run to its image size change against the app's
// previous successful run, keyed like matchBuildRuns.
func imageDeltas(runs []store.BuildRun) map[string]int64 {
	asc := append([]store.BuildRun(nil), runs...)
	sort.SliceStable(asc, func(i, j int) bool { return asc[i].StartedAt.Before(asc[j].StartedAt) })
	prev := map[string]int64{}
	out := map[string]int64{}
	for _, r := range asc {
		if r.Status != string(build.RunSuccess) || r.ImageBytes <= 0 {
			continue
		}
		if p, ok := prev[r.App]; ok {
			out[r.App+r.Tag+r.StartedAt.Format(time.RFC3339Nano)] = r.ImageBytes - p
		}
		prev[r.App] = r.ImageBytes
	}
	return out
}

func failureHint(app, errText string) *buildHint {
	if errText == "" {
		return nil
	}
	in := diagnose.Input{ServiceName: app, Attempt: &diagnose.AttemptInput{Status: store.DeployAttemptStatusFailed, Error: errText}, Facts: diagnose.Facts{BuildFailed: true}}
	if causes := diagnose.Analyze(in); len(causes) > 0 {
		c := causes[0]
		h := &buildHint{Code: c.Code, Title: c.Title, Explanation: c.Explanation}
		if len(c.Fixes) > 0 {
			h.Suggestion = c.Fixes[0].Hint
			if h.Suggestion == "" {
				h.Suggestion = c.Fixes[0].Label
			}
		}
		if c.Code == diagnose.CauseBuildOutOfDisk {
			h.Fix = buildFixPruneCache
		}
		return h
	}
	if res := diagnose.Diagnose(in); res.Confidence != diagnose.ConfidenceNone {
		return &buildHint{Code: "MATCHED", Title: "Known failure", Explanation: res.Explanation, Suggestion: res.Suggestion}
	}
	return nil
}

// assembleBuildRows joins attempts with their build runs, in queue order:
// queued by arrival, then running, then finished newest first.
func assembleBuildRows(attempts []store.DeployAttempt, runs []store.BuildRun) []buildRow {
	matched := matchBuildRuns(attempts, runs)
	deltas := imageDeltas(runs)
	rows := make([]buildRow, 0, len(attempts))
	for _, a := range attempts {
		row := buildRow{
			ID: a.ID, App: a.ServiceName, Status: a.Status, Source: a.Source, Branch: a.Branch, CommitSHA: a.CommitSHA,
			Image: a.Image, QueuedAt: a.QueuedAt, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, Error: a.Error,
			LogsPath:  "/api/v1/apps/" + a.ServiceName + "/deploys/" + a.ID + "/logs",
			CanCancel: isQueuedStatus(a.Status) || a.Status == store.DeployAttemptStatusRunning,
			CanRetry:  a.Status == store.DeployAttemptStatusFailed || a.Status == store.DeployAttemptStatusCanceled,
		}
		if a.FinishedAt != nil && !a.FinishedAt.Before(a.StartedAt) {
			d := a.FinishedAt.Sub(a.StartedAt).Seconds()
			row.DurationSeconds = &d
		}
		if r, ok := matched[a.ID]; ok {
			d := float64(r.DurationMS) / 1000
			row.DurationSeconds = &d
			row.Node, row.Platform, row.ImageBytes = r.NodeID, r.Platform, r.ImageBytes
			if r.TotalSteps > 0 {
				ratio := float64(r.CachedSteps) / float64(r.TotalSteps)
				row.CacheHitRatio = &ratio
			}
			if d, ok := deltas[r.App+r.Tag+r.StartedAt.Format(time.RFC3339Nano)]; ok {
				row.ImageDeltaBytes = &d
			}
			if row.Error == "" {
				row.Error = r.Error
			}
		}
		if a.Status == store.DeployAttemptStatusFailed {
			row.Hint = failureHint(a.ServiceName, row.Error)
		}
		rows = append(rows, row)
	}
	sortBuildRows(rows)
	return rows
}

func buildRank(s string) int {
	switch {
	case isQueuedStatus(s):
		return 0
	case s == store.DeployAttemptStatusRunning:
		return 1
	}
	return 2
}

// sortBuildRows orders the queue by arrival and numbers its waiting rows.
func sortBuildRows(rows []buildRow) {
	arrival := func(r buildRow) time.Time {
		if r.QueuedAt != nil {
			return *r.QueuedAt
		}
		return r.StartedAt
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ri, rj := buildRank(rows[i].Status), buildRank(rows[j].Status)
		if ri != rj {
			return ri < rj
		}
		if ri == 2 {
			return rows[i].StartedAt.After(rows[j].StartedAt)
		}
		return arrival(rows[i]).Before(arrival(rows[j]))
	})
	pos := 0
	for i := range rows {
		if isQueuedStatus(rows[i].Status) {
			pos++
			rows[i].QueuePosition = pos
		}
	}
}

func countBuildRows(rows []buildRow) buildCounts {
	var c buildCounts
	for _, r := range rows {
		switch {
		case isQueuedStatus(r.Status):
			c.Queued++
		case r.Status == store.DeployAttemptStatusRunning:
			c.Running++
		case isFinishedStatus(r.Status):
			c.Finished++
			if r.Status == store.DeployAttemptStatusFailed {
				c.Failed++
			}
		}
	}
	return c
}

func (rt *Router) buildCenterRows(r *http.Request, limit int) ([]buildRow, error) {
	attempts, err := rt.buildPerf.ListBuildAttempts(r.Context(), limit)
	if err != nil {
		return nil, err
	}
	runs, err := rt.buildPerf.ListAllBuildRuns(r.Context(), buildCenterMaxLimit)
	if err != nil {
		return nil, err
	}
	visible, err := rt.visibleAppNames(r)
	if err != nil {
		return nil, err
	}
	rows := assembleBuildRows(attempts, runs)
	if visible == nil {
		return rows, nil
	}
	allowed := make(map[string]bool, len(visible))
	for _, n := range visible {
		allowed[n] = true
	}
	kept := rows[:0]
	for _, row := range rows {
		if allowed[row.App] {
			kept = append(kept, row)
		}
	}
	return kept, nil
}

func (rt *Router) buildCenterReady(w http.ResponseWriter) bool {
	if rt.buildPerf == nil {
		writeError(w, http.StatusNotImplemented, "the build center is not available on this control plane")
		return false
	}
	return true
}

// handleListBuilds handles GET /api/v1/builds: queue, running and finished builds with their stats.
func (rt *Router) handleListBuilds(w http.ResponseWriter, r *http.Request) {
	if !rt.buildCenterReady(w) {
		return
	}
	limit := queryIntOr(r, "limit", buildCenterDefaultLimit)
	if limit <= 0 || limit > buildCenterMaxLimit {
		limit = buildCenterDefaultLimit
	}
	rows, err := rt.buildCenterRows(r, limit)
	if err != nil {
		rt.internalError(w, "api: list builds failed", err)
		return
	}
	app, status := r.URL.Query().Get("app"), r.URL.Query().Get("status")
	if app != "" || status != "" {
		kept := rows[:0]
		for _, row := range rows {
			if (app == "" || row.App == app) && (status == "" || row.Status == status) {
				kept = append(kept, row)
			}
		}
		rows = kept
	}
	maxConc := rt.buildLimitsView(r.Context()).Effective.MaxConcurrent
	resp := buildCenterResponse{
		Rows: rows, Counts: countBuildRows(rows), Nodes: []buildNodeLoadResource{}, Limit: maxConc,
		Platforms: buildPlatforms{Host: build.HostPlatform(), MultiPlatform: false},
	}
	if rt.buildLoad != nil {
		for _, l := range rt.buildLoad.Load() {
			resp.Nodes = append(resp.Nodes, buildNodeLoadResource{Node: l.Node, Running: l.Running, Waiting: l.Waiting, Limit: l.Limit})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetBuild handles GET /api/v1/builds/{id}.
func (rt *Router) handleGetBuild(w http.ResponseWriter, r *http.Request) {
	if !rt.buildCenterReady(w) {
		return
	}
	rows, err := rt.buildCenterRows(r, buildCenterMaxLimit)
	if err != nil {
		rt.internalError(w, "api: get build failed", err)
		return
	}
	id := r.PathValue("id")
	for _, row := range rows {
		if row.ID == id {
			writeJSON(w, http.StatusOK, row)
			return
		}
	}
	writeError(w, http.StatusNotFound, "build not found")
}

// handleCancelBuild handles POST /api/v1/apps/{name}/builds/{id}/cancel by
// canceling the deploy attempt that owns the build.
func (rt *Router) handleCancelBuild(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("deployId", r.PathValue("id"))
	rt.handleCancelDeploy(w, r)
}

type buildRetryResponse struct {
	Message string `json:"message"`
}

// retryBuild rebuilds the app's connected branch, the same path a scheduled deploy takes.
func (rt *Router) retryBuild(ctx context.Context, app, branch string) (string, error) {
	if rt.buildRetrier != nil {
		return rt.buildRetrier(ctx, app, branch)
	}
	return rt.TriggerScheduledDeploy(ctx, app, branch)
}

// handleRetryBuild handles POST /api/v1/apps/{name}/builds/{id}/retry.
func (rt *Router) handleRetryBuild(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.lookupAppAttempt(w, withDeployID(r))
	if !ok {
		return
	}
	if a.Status != store.DeployAttemptStatusFailed && a.Status != store.DeployAttemptStatusCanceled {
		writeError(w, http.StatusConflict, "only a failed or canceled build can be retried (this one is "+a.Status+")")
		return
	}
	if a.Source == store.DeployAttemptSourceImage || a.Source == store.DeployAttemptSourceCompose {
		writeError(w, http.StatusConflict, "this deploy did not build from source, redeploy it from the app page")
		return
	}
	branch := a.Branch
	if branch == "" {
		gs, err := rt.gitSources.GetGitSource(r.Context(), a.ServiceName)
		if errors.Is(err, store.ErrGitSourceNotFound) {
			writeError(w, http.StatusConflict, "this app has no connected git source to rebuild from")
			return
		}
		if err != nil {
			rt.internalError(w, "api: retry build: load git source failed", err)
			return
		}
		branch = gs.Branch
	}
	msg, err := rt.retryBuild(r.Context(), a.ServiceName, branch)
	if err != nil {
		writeError(w, http.StatusBadGateway, "retry failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, buildRetryResponse{Message: msg})
}

func withDeployID(r *http.Request) *http.Request {
	r.SetPathValue("deployId", r.PathValue("id"))
	return r
}
