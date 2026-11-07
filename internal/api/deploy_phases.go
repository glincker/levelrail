package api

import (
	"context"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// Phase keys and statuses on GET /api/v1/deployments/{id}/phases.
const (
	phaseUpload     = "upload"
	phaseInstall    = "install"
	phaseBuild      = "build"
	phaseExport     = "export_image"
	phaseHealth     = "health_check"
	phaseCutoverKey = "cutover"

	phaseStatusPending = "pending"
	phaseStatusRunning = "running"
	phaseStatusDone    = "done"
	phaseStatusFailed  = "failed"
	phaseStatusSkipped = "skipped"
)

var (
	installLineRe = regexp.MustCompile(`(?i)\b(npm (ci|install|i)|yarn install|pnpm (i|install)|pip3? install|bundle install|go mod download|composer install|poetry install)\b`)
	buildLineRe   = regexp.MustCompile(`(?i)\b(next build|npm run build|yarn build|pnpm( run)? build|go build|cargo build|vite build|gradle|mvn)\b|creating an optimized production build`)
	exportLineRe  = regexp.MustCompile(`(?i)exporting to (image|oci|docker)|exporting layers|writing image|naming to`)
)

// deployPhase is one stage of a deploy attempt. Times are null when the
// stage has no timestamped evidence yet.
type deployPhase struct {
	Phase      string     `json:"phase"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	DurationMS *int64     `json:"duration_ms"`
	TypicalMS  *int64     `json:"typical_ms"`
}

type deployPhasesResponse struct {
	DeployID       string        `json:"deploy_id"`
	App            string        `json:"app"`
	Status         string        `json:"status"`
	StartedAt      time.Time     `json:"started_at"`
	FinishedAt     *time.Time    `json:"finished_at"`
	ElapsedMS      int64         `json:"elapsed_ms"`
	Phases         []deployPhase `json:"phases"`
	ETAMS          *int64        `json:"eta_ms"`
	TypicalTotalMS *int64        `json:"typical_total_ms"`
	Samples        int           `json:"samples"`
}

// phaseEvidence is what the stored trace rows and build log say about one phase.
type phaseEvidence struct {
	reached bool
	failed  bool
	start   time.Time
	end     time.Time // explicit end, zero when unknown
}

func traceByKeys(trace []store.DeployTraceStep, steps ...string) phaseEvidence {
	var ev phaseEvidence
	allDone := true
	for _, s := range trace {
		if !slices.Contains(steps, s.Step) {
			continue
		}
		ev.reached = true
		if ev.start.IsZero() || (!s.StartedAt.IsZero() && s.StartedAt.Before(ev.start)) {
			ev.start = s.StartedAt
		}
		switch s.Status {
		case store.TraceStatusFailed:
			ev.failed = true
		case store.TraceStatusRunning:
			allDone = false
		}
		if s.EndedAt.After(ev.end) {
			ev.end = s.EndedAt
		}
	}
	if !allDone {
		ev.end = time.Time{}
	}
	return ev
}

// logEvidence scans the build log once, moving forward only, so a later
// mention of "npm install" in app output cannot pull install back.
func logEvidence(logs []telemetry.DeployLogEntry) (install, build, export phaseEvidence) {
	stage := 0
	mark := func(ev *phaseEvidence, at time.Time) {
		if !ev.reached {
			ev.reached, ev.start = true, at
		}
	}
	for _, l := range logs {
		m := l.Message
		switch {
		case stage < 3 && exportLineRe.MatchString(m):
			stage = 3
			mark(&export, l.Timestamp)
		case stage < 2 && buildLineRe.MatchString(m):
			stage = 2
			mark(&build, l.Timestamp)
		case stage < 1 && installLineRe.MatchString(m):
			stage = 1
			mark(&install, l.Timestamp)
		}
	}
	if len(logs) > 0 && !build.reached && !install.reached && !export.reached {
		build.reached, build.start = true, logs[0].Timestamp
	}
	return install, build, export
}

// derivePhases folds the attempt, its trace rows and its build log into the
// ordered phase list. Pure, so a running and a settled deploy share one path.
func derivePhases(a store.DeployAttempt, trace []store.DeployTraceStep, logs []telemetry.DeployLogEntry, now time.Time) []deployPhase {
	building := traceByKeys(trace, "building")
	install, build, export := logEvidence(logs)
	hasBuild := building.reached || len(logs) > 0
	type slot struct {
		key  string
		ev   phaseEvidence
		show bool
	}
	upload := phaseEvidence{}
	if a.Source == store.DeployAttemptSourceUpload {
		upload = phaseEvidence{reached: true, start: a.StartedAt}
		switch {
		case building.reached:
			upload.end = building.start
		case len(logs) > 0 && !logs[0].Timestamp.IsZero():
			upload.end = logs[0].Timestamp
		}
	}
	slots := []slot{
		{phaseUpload, upload, a.Source == store.DeployAttemptSourceUpload},
		{phaseInstall, install, hasBuild},
		{phaseBuild, build, hasBuild},
		{phaseExport, export, hasBuild},
		{phaseHealth, traceByKeys(trace, deploytrace.StepContainerStart, deploytrace.StepReadinessProbe, deploytrace.StepReadinessGate, deploytrace.StepStabilityGate), true},
		{phaseCutoverKey, traceByKeys(trace, deploytrace.StepCutover, deploytrace.StepDrainOld), true},
	}
	finished := a.Status != store.DeployAttemptStatusRunning && a.Status != store.DeployAttemptStatusQueued
	lastReached := -1
	for i, s := range slots {
		if s.show && s.ev.reached {
			lastReached = i
		}
	}
	out := make([]deployPhase, 0, len(slots))
	for i, s := range slots {
		if !s.show {
			continue
		}
		p := deployPhase{Phase: s.key}
		switch {
		case !s.ev.reached:
			p.Status = phaseStatusPending
			if i < lastReached || a.Status == store.DeployAttemptStatusSucceeded {
				p.Status = phaseStatusSkipped
			}
		case s.ev.failed || (i == lastReached && a.Status == store.DeployAttemptStatusFailed):
			p.Status = phaseStatusFailed
		case i == lastReached && !finished && s.ev.end.IsZero():
			p.Status = phaseStatusRunning
		default:
			p.Status = phaseStatusDone
		}
		if s.ev.reached && !s.ev.start.IsZero() {
			st := s.ev.start
			p.StartedAt = &st
			end := s.ev.end
			if end.IsZero() {
				for _, n := range slots[i+1:] {
					if n.show && n.ev.reached && !n.ev.start.IsZero() {
						end = n.ev.start
						break
					}
				}
			}
			switch {
			case end.IsZero() && a.FinishedAt != nil:
				end = *a.FinishedAt
			case end.IsZero() && p.Status == phaseStatusRunning:
				end = now
			}
			if !end.IsZero() && !end.Before(st) {
				d := end.Sub(st).Milliseconds()
				p.DurationMS = &d
				if p.Status != phaseStatusRunning {
					p.FinishedAt = &end
				}
			}
		}
		out = append(out, p)
	}
	return out
}

func medianMS(v []int64) int64 {
	s := slices.Clone(v)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// phaseBaselines takes the median total and per-phase duration over the app's
// last successful attempts. ok is false below two samples.
func (rt *Router) phaseBaselines(ctx context.Context, a store.DeployAttempt) (total int64, perPhase map[string]int64, n int, ok bool) {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, a.ServiceName)
	if err != nil {
		return 0, nil, 0, false
	}
	limit := rt.phaseSampleCount(ctx)
	var totals []int64
	byPhase := map[string][]int64{}
	for _, s := range attempts {
		if len(totals) >= limit {
			break
		}
		if s.ID == a.ID || s.Status != store.DeployAttemptStatusSucceeded || s.FinishedAt == nil {
			continue
		}
		totals = append(totals, s.FinishedAt.Sub(s.StartedAt).Milliseconds())
		var logs []telemetry.DeployLogEntry
		if rt.deployLogStore != nil {
			logs, _ = rt.deployLogStore.QueryDeployLog(ctx, s.ID)
		}
		for _, p := range derivePhases(s, rt.traceRows(ctx, s.ID), logs, *s.FinishedAt) {
			if p.DurationMS != nil && *p.DurationMS > 0 {
				byPhase[p.Phase] = append(byPhase[p.Phase], *p.DurationMS)
			}
		}
	}
	if len(totals) < 2 {
		return 0, nil, len(totals), false
	}
	perPhase = map[string]int64{}
	for k, v := range byPhase {
		if len(v) >= 2 {
			perPhase[k] = medianMS(v)
		}
	}
	return medianMS(totals), perPhase, len(totals), true
}

// handleGetDeployPhases handles GET /api/v1/deployments/{id}/phases: the
// stages of one deploy with status, timing and an ETA while it runs. It is
// derived from stored trace rows and the build log; nothing new is written.
func (rt *Router) handleGetDeployPhases(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	now := time.Now().UTC()
	trace := rt.traceRows(r.Context(), a.ID)
	logs := rt.phaseLogs(r.Context(), a, now)
	phases := derivePhases(a, trace, logs, now)
	end := now
	if a.FinishedAt != nil {
		end = *a.FinishedAt
	}
	resp := deployPhasesResponse{
		DeployID: a.ID, App: a.ServiceName, Status: a.Status, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
		ElapsedMS: max(end.Sub(a.StartedAt).Milliseconds(), 0), Phases: phases,
	}
	if total, per, n, ok := rt.phaseBaselines(r.Context(), a); ok {
		resp.Samples, resp.TypicalTotalMS = n, &total
		for i := range resp.Phases {
			if v, has := per[resp.Phases[i].Phase]; has {
				resp.Phases[i].TypicalMS = &v
			}
		}
		if a.FinishedAt == nil {
			eta := max(total-resp.ElapsedMS, 0)
			resp.ETAMS = &eta
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, resp)
}

// phaseLogs prefers the recorder's timestamped live lines while the attempt
// runs, so install/build/export get times before the log is persisted.
func (rt *Router) phaseLogs(ctx context.Context, a store.DeployAttempt, now time.Time) []telemetry.DeployLogEntry {
	if rt.deployRecorder != nil && a.FinishedAt == nil {
		lines, _, unsubscribe, ok := rt.deployRecorder.Snapshot(a.ID)
		unsubscribe()
		if ok {
			out := make([]telemetry.DeployLogEntry, 0, len(lines))
			for _, l := range lines {
				at := l.At
				if at.IsZero() {
					at = now
				}
				out = append(out, telemetry.DeployLogEntry{AttemptID: a.ID, Stream: l.Stream, Timestamp: at, Message: l.Line})
			}
			return out
		}
	}
	return rt.persistedDeployLog(ctx, a.ID)
}

func (rt *Router) persistedDeployLog(ctx context.Context, id string) []telemetry.DeployLogEntry {
	if rt.deployLogStore == nil {
		return nil
	}
	logs, err := rt.deployLogStore.QueryDeployLog(ctx, id)
	if err != nil {
		return nil
	}
	return logs
}
