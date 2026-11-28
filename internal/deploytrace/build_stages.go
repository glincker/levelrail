package deploytrace

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/secretmask"
	"github.com/glincker/levelrail/internal/store"
)

// StepBuildStage is the trace step name of one build stage row; its key is
// "build_stage.<stage>".
const StepBuildStage = "build_stage"

// Build stages in pipeline order. Roll out is not here: the deploy trace
// already records it with its own steps.
const (
	StageSource  = "source"
	StageDetect  = "detect"
	StageInstall = "install"
	StageBuild   = "build"
	StageExport  = "export"
	StagePush    = "push"
)

// Cache states a stage reports.
const (
	CacheHit     = "hit"
	CachePartial = "partial"
	CacheMiss    = "miss"
)

// StageOrder lists the stages in the order a build runs them.
var StageOrder = []string{StageSource, StageDetect, StageInstall, StageBuild, StageExport, StagePush}

// StageKey is the trace row key of stage.
func StageKey(stage string) string { return StepBuildStage + "." + stage }

// IsStage reports whether s names a build stage.
func IsStage(s string) bool { return stageRank(s) >= 0 }

func stageRank(s string) int {
	for i, n := range StageOrder {
		if n == s {
			return i
		}
	}
	return -1
}

// ClassifyStage maps a BuildKit step name to its stage; "" for an empty name,
// so a bare log line stays with the stage that was running.
func ClassifyStage(step string) string {
	s := strings.ToLower(strings.TrimSpace(step))
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "pushing") || strings.HasPrefix(s, "push") || strings.Contains(s, "load image"):
		return StagePush
	case strings.Contains(s, "exporting") || strings.Contains(s, "export "):
		return StageExport
	case strings.HasPrefix(s, "[internal] load") || strings.Contains(s, "load build") || s == "queue" || strings.Contains(s, "git clone") || strings.Contains(s, "fetch source"):
		return StageSource
	case s == "builder" || strings.Contains(s, "railpack") || strings.Contains(s, "detect") || strings.Contains(s, "build plan"):
		return StageDetect
	case containsAny(s, "npm ci", "npm install", "pnpm install", "yarn install", "yarn --", "bun install", "pip install", "go mod download", "bundle install", "composer install", "cargo fetch", "apt-get", "apk add", "mise install", "poetry install", "uv sync", " install"):
		return StageInstall
	default:
		return StageBuild
	}
}

func containsAny(s string, subs ...string) bool {
	for _, p := range subs {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// StageObs is one build progress update as the tracker needs it.
type StageObs struct {
	Step      string
	Cached    bool
	Completed bool
	Error     string
	Log       string
}

type stageState struct {
	span      *Span
	started   time.Time
	vertices  int
	cached    int
	bytes     int64
	tail      []string
	failed    bool
	closed    bool
	missStep  string
	missCause string
}

// StageTracker turns one build's progress events into per-stage trace rows
// with start, end, cache state and a short log tail. A nil tracker is valid.
type StageTracker struct {
	mu       sync.Mutex
	tr       *Tracer
	tailN    func() int
	stages   map[string]*stageState
	current  string
	maxRank  int
	finished bool
	// copyMissed and anyMissed carry across stages so a later install miss
	// can be blamed on the earlier COPY that changed.
	copyMissed bool
	anyMissed  bool
}

// NewStageTracker returns a tracker writing to tr, or nil when tr is nil.
// tailLines gives the live per-stage tail size.
func NewStageTracker(tr *Tracer, tailLines func() int) *StageTracker {
	if tr == nil {
		return nil
	}
	return &StageTracker{tr: tr, tailN: tailLines, stages: map[string]*stageState{}, maxRank: -1}
}

// Observe records o and returns the stage it belongs to.
func (s *StageTracker) Observe(ctx context.Context, o StageObs) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return s.current
	}
	stage := ClassifyStage(o.Step)
	if stage == "" {
		stage = s.current
	}
	if stage == "" {
		stage = StageSource
	}
	st := s.enter(ctx, stage)
	s.current = stage

	if o.Log != "" {
		st.bytes += int64(len(o.Log))
		for _, l := range strings.Split(strings.TrimRight(o.Log, "\n"), "\n") {
			if l = strings.TrimRight(l, "\r"); l != "" {
				st.tail = append(st.tail, l)
			}
		}
		s.trim(st)
	}
	if o.Completed {
		st.vertices++
		if o.Cached {
			st.cached++
		} else {
			s.noteMiss(st, stage, o.Step)
		}
		if o.Error != "" {
			st.failed = true
			st.tail = append(st.tail, "error: "+secretmask.Mask(o.Error))
			s.trim(st)
		}
		st.span.Progress(ctx, summary(st))
	}
	return stage
}

// noteMiss records the first missed step of a stage and its likely cause.
func (s *StageTracker) noteMiss(st *stageState, stage, step string) {
	cause := MissCause(step, stage, s.copyMissed, s.anyMissed)
	if cause == "" {
		return
	}
	if st.missStep == "" {
		st.missStep, st.missCause = step, cause
	}
	if cause == CauseSourceChanged {
		s.copyMissed = true
	}
	s.anyMissed = true
}

func (s *StageTracker) trim(st *stageState) {
	n := 0
	if s.tailN != nil {
		n = s.tailN()
	}
	if n > 0 && len(st.tail) > n {
		st.tail = append([]string(nil), st.tail[len(st.tail)-n:]...)
	}
}

// enter opens stage and closes every earlier stage once a later one starts.
func (s *StageTracker) enter(ctx context.Context, stage string) *stageState {
	st, ok := s.stages[stage]
	if !ok {
		st = &stageState{started: s.tr.now()}
		st.span = s.tr.BeginKey(ctx, StepBuildStage, StageKey(stage), "")
		s.stages[stage] = st
	}
	if r := stageRank(stage); r > s.maxRank {
		s.maxRank = r
		for name, other := range s.stages {
			if stageRank(name) < r && !other.closed {
				s.close(ctx, other)
			}
		}
	}
	return st
}

func (s *StageTracker) close(ctx context.Context, st *stageState) {
	st.closed = true
	status, reason := store.TraceStatusDone, cacheReason(st)
	if st.failed {
		status, reason = store.TraceStatusFailed, "step_failed"
	}
	st.span.End(ctx, status, reason, summary(st)+"\n"+strings.Join(st.tail, "\n"))
}

// Finish closes every stage still open.
func (s *StageTracker) Finish(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	for _, st := range s.stages {
		if !st.closed {
			s.close(ctx, st)
		}
	}
}

func summary(st *stageState) string {
	base := fmt.Sprintf("vertices=%d cached=%d bytes=%d", st.vertices, st.cached, st.bytes)
	if st.missStep == "" {
		return base
	}
	step := strings.ReplaceAll(secretmask.Mask(st.missStep), "\n", " ")
	return base + " cause=" + st.missCause + " miss=" + step
}

// Likely reasons a build step missed the cache.
const (
	CauseBaseChanged   = "base_changed"
	CauseArgChanged    = "arg_changed"
	CauseSourceChanged = "source_changed"
	CauseCopyBefore    = "copy_before_install"
	CauseEarlierStep   = "earlier_step_changed"
	CauseStepChanged   = "step_changed"
)

// MissCause names the likely reason step missed the cache; "" for a step
// that is never cacheable (BuildKit internal bookkeeping other than the
// base image metadata lookup).
func MissCause(step, stage string, copyMissed, anyMissed bool) string {
	s := strings.ToLower(strings.TrimSpace(step))
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "load metadata") || strings.Contains(s, "] from "):
		return CauseBaseChanged
	case strings.HasPrefix(s, "[internal]"):
		return ""
	case strings.Contains(s, "] arg ") || strings.Contains(s, "--build-arg"):
		return CauseArgChanged
	case strings.Contains(s, "] copy ") || strings.Contains(s, "] add "):
		return CauseSourceChanged
	case copyMissed && stage == StageInstall:
		return CauseCopyBefore
	case anyMissed:
		return CauseEarlierStep
	}
	return CauseStepChanged
}

func cacheReason(st *stageState) string {
	if c := cacheState(st.vertices, st.cached); c != "" {
		return "cache_" + c
	}
	return ""
}

func cacheState(vertices, cached int) string {
	switch {
	case vertices > 0 && cached >= vertices:
		return CacheHit
	case cached > 0:
		return CachePartial
	case vertices > 0:
		return CacheMiss
	}
	return ""
}

// SummaryLine is the first line of a stage row's detail, the part safe to
// show in step and log feeds; the rest is the stage's log tail.
func SummaryLine(detail string) string {
	if i := strings.IndexByte(detail, '\n'); i >= 0 {
		return detail[:i]
	}
	return detail
}

// StageView is the API and CLI shape of one build stage.
type StageView struct {
	Stage          string     `json:"stage"`
	Status         string     `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	EndedAt        *time.Time `json:"ended_at,omitempty"`
	DurationMS     int64      `json:"duration_ms"`
	Cache          string     `json:"cache,omitempty"`
	Vertices       int        `json:"vertices"`
	CachedVertices int        `json:"cached_vertices"`
	LogBytes       int64      `json:"log_bytes"`
	// MissStep and MissCause name the first step that missed the cache and
	// why it likely did; empty when nothing missed.
	MissStep  string `json:"miss_step,omitempty"`
	MissCause string `json:"miss_cause,omitempty"`
}

// StagesFromRows picks the build stage rows out of a trace, in pipeline order.
func StagesFromRows(rows []store.DeployTraceStep) []StageView {
	var out []StageView
	for _, r := range rows {
		if r.Step != StepBuildStage {
			continue
		}
		stage := strings.TrimPrefix(r.Key, StepBuildStage+".")
		if !IsStage(stage) {
			continue
		}
		v := StageView{Stage: stage, Status: r.Status, StartedAt: r.StartedAt, DurationMS: r.DurationMS}
		if !r.EndedAt.IsZero() {
			e := r.EndedAt
			v.EndedAt = &e
		}
		_, _ = fmt.Sscanf(SummaryLine(r.Detail), "vertices=%d cached=%d bytes=%d", &v.Vertices, &v.CachedVertices, &v.LogBytes)
		v.Cache = cacheState(v.Vertices, v.CachedVertices)
		v.MissCause, v.MissStep = parseMiss(SummaryLine(r.Detail))
		out = append(out, v)
	}
	sort.SliceStable(out, func(i, j int) bool { return stageRank(out[i].Stage) < stageRank(out[j].Stage) })
	return out
}

// StageTail returns the stored log tail of stage, empty when none was kept.
func StageTail(rows []store.DeployTraceStep, stage string) []string {
	for _, r := range rows {
		if r.Key != StageKey(stage) {
			continue
		}
		_, rest, ok := strings.Cut(r.Detail, "\n")
		if !ok || rest == "" {
			return nil
		}
		return strings.Split(rest, "\n")
	}
	return nil
}

func parseMiss(line string) (cause, step string) {
	_, rest, ok := strings.Cut(line, " cause=")
	if !ok {
		return "", ""
	}
	cause, step, _ = strings.Cut(rest, " miss=")
	return cause, step
}
