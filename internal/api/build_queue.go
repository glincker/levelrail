package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/attention"
	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// Build queue settings. The env vars only seed the defaults.
const (
	KeyBuildQueueAutoResumeAfter = "build.queue_auto_resume_after"
	KeyBuildQueuePausedWarnAfter = "build.queue_paused_warn_after"

	buildQueueStepUpApp     = "build-queue-kill-all"
	buildQueueReasonMaxLen  = 500
	buildQueueRecentChanges = 10
	buildQueueSystemActor   = "system"
)

func init() {
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildQueueAutoResumeAfter, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: "0s", Env: "APP_BUILD_QUEUE_AUTO_RESUME_AFTER", Apply: runtimesettings.ApplyLive, CheckValue: checkQueueDuration,
		Title:       "Build queue auto resume",
		Description: "How long a paused or draining build queue stays held before it resumes on its own. 0 means never: an operator resumes it. Read on every queue decision.",
	})
	runtimesettings.Register(runtimesettings.Definition{
		Key: KeyBuildQueuePausedWarnAfter, Category: runtimesettings.CategoryDeploys, Kind: runtimesettings.KindString,
		Default: "1h", Env: "APP_BUILD_QUEUE_PAUSED_WARN_AFTER", Apply: runtimesettings.ApplyLive, CheckValue: checkQueueDuration,
		Title:       "Paused build queue warning",
		Description: "How long the build queue may stay paused or draining before Doctor warns and an attention item asks to resume it. 0 turns the warning off.",
	})
}

// checkQueueDuration accepts a duration such as 30m, or 0 to turn the behaviour off.
func checkQueueDuration(canonical string) error {
	if d, err := time.ParseDuration(strings.TrimSpace(canonical)); err != nil || d < 0 {
		return fmt.Errorf("value must be a duration such as 30m, or 0 to turn it off")
	}
	return nil
}

// queueDuration reads one of the queue duration settings; a bad value reads as off.
func (rt *Router) queueDuration(ctx context.Context, key string) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(rt.runtimeSettings.String(ctx, key)))
	if err != nil || d < 0 {
		return 0
	}
	return d
}

// BuildQueueStore persists the build queue switch. *store.DB satisfies it.
type BuildQueueStore interface {
	GetBuildQueueState(ctx context.Context) (store.BuildQueueState, error)
	SetBuildQueueState(ctx context.Context, st store.BuildQueueState) error
	ListBuildQueueEvents(ctx context.Context, limit int) ([]store.BuildQueueEvent, error)
}

// buildActiveSource is the limiter's view of builds in flight.
type buildActiveSource interface {
	Active() []build.ActiveBuild
}

// buildQueueControl caches the persisted switch; changed is closed on every change so held builds wake.
type buildQueueControl struct {
	mu      sync.Mutex
	loaded  bool
	st      store.BuildQueueState
	changed chan struct{}
}

func (rt *Router) buildQueueStore() BuildQueueStore {
	if s, ok := rt.deploySafety.(BuildQueueStore); ok {
		return s
	}
	return nil
}

// buildQueueState returns the current switch, applying auto resume when it is due.
func (rt *Router) buildQueueState(ctx context.Context) (store.BuildQueueState, <-chan struct{}) {
	q := &rt.buildQueue
	q.mu.Lock()
	if q.changed == nil {
		q.changed = make(chan struct{})
	}
	if !q.loaded {
		q.st = store.BuildQueueState{State: store.BuildQueueRunning}
		if s := rt.buildQueueStore(); s != nil {
			if st, err := s.GetBuildQueueState(ctx); err == nil {
				q.st = st
				q.loaded = true
			} else {
				rt.logger.Warn("api: build queue: load state failed", slog.String("error", err.Error()))
			}
		} else {
			q.loaded = true
		}
	}
	st, ch := q.st, q.changed
	q.mu.Unlock()
	if st.State != store.BuildQueueRunning {
		if after := rt.queueDuration(ctx, KeyBuildQueueAutoResumeAfter); after > 0 && !st.Since.IsZero() && time.Since(st.Since) >= after {
			next := store.BuildQueueState{State: store.BuildQueueRunning, Actor: buildQueueSystemActor, Reason: "auto resume after " + after.String(), Since: time.Now()}
			if err := rt.setBuildQueueState(ctx, next); err == nil {
				go rt.DrainDeployQueue(context.WithoutCancel(ctx))
				return rt.buildQueueState(ctx)
			}
		}
	}
	return st, ch
}

func (rt *Router) setBuildQueueState(ctx context.Context, st store.BuildQueueState) error {
	if s := rt.buildQueueStore(); s != nil {
		if err := s.SetBuildQueueState(ctx, st); err != nil {
			return err
		}
	}
	q := &rt.buildQueue
	q.mu.Lock()
	q.st, q.loaded = st, true
	if q.changed != nil {
		close(q.changed)
	}
	q.changed = make(chan struct{})
	q.mu.Unlock()
	rt.logger.Info("api: build queue state changed", slog.String("state", st.State), slog.String("by", st.Actor))
	return nil
}

// buildsHeld reports whether new builds must wait in the queue.
func (rt *Router) buildsHeld(ctx context.Context) bool {
	st, _ := rt.buildQueueState(ctx)
	return st.State != store.BuildQueueRunning
}

// buildQueueWaitReason is the reason recorded on a waiting build.
func buildQueueWaitReason(st store.BuildQueueState) string {
	actor := st.Actor
	if actor == "" {
		actor = buildQueueSystemActor
	}
	msg := "builds are paused by " + actor
	if st.State == store.BuildQueueDraining {
		msg = "builds are draining, started by " + actor
	}
	if st.Reason != "" {
		msg += ": " + st.Reason
	}
	return msg
}

// BuildQueueGate is the gate the build limiter consults before every build.
func (rt *Router) BuildQueueGate() build.QueueGate { return routerQueueGate{rt: rt} }

type routerQueueGate struct{ rt *Router }

// Hold waits while the queue is paused or draining, telling the build log why once per change.
func (g routerQueueGate) Hold(ctx context.Context, wait func(msg string)) error {
	said := ""
	for {
		st, changed := g.rt.buildQueueState(ctx)
		if st.State == store.BuildQueueRunning {
			return nil
		}
		if msg := "Waiting: " + buildQueueWaitReason(st); msg != said {
			wait(msg)
			said = msg
		}
		var resume <-chan time.Time
		if after := g.rt.queueDuration(ctx, KeyBuildQueueAutoResumeAfter); after > 0 && !st.Since.IsZero() {
			resume = time.After(time.Until(st.Since.Add(after)))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		case <-resume:
		}
	}
}

type buildQueueChange struct {
	State  string    `json:"state"`
	Reason string    `json:"reason,omitempty"`
	Actor  string    `json:"actor,omitempty"`
	At     time.Time `json:"at"`
}

type buildQueueRunning struct {
	DeployID       string     `json:"deploy_id,omitempty"`
	App            string     `json:"app"`
	Node           string     `json:"node,omitempty"`
	Phase          string     `json:"phase"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	ElapsedSeconds float64    `json:"elapsed_seconds"`
}

type buildQueueQueued struct {
	DeployID    string     `json:"deploy_id"`
	App         string     `json:"app"`
	Position    int        `json:"position"`
	QueuedAt    *time.Time `json:"queued_at,omitempty"`
	WaitSeconds float64    `json:"wait_seconds"`
	WaitReason  string     `json:"wait_reason,omitempty"`
}

type buildQueueSlots struct {
	Used  int `json:"used"`
	Total int `json:"total"`
}

type buildQueueResource struct {
	State        string              `json:"state"`
	Reason       string              `json:"reason,omitempty"`
	Actor        string              `json:"actor,omitempty"`
	Since        *time.Time          `json:"since,omitempty"`
	AutoResumeAt *time.Time          `json:"auto_resume_at,omitempty"`
	Drained      bool                `json:"drained"`
	Slots        buildQueueSlots     `json:"slots"`
	Running      []buildQueueRunning `json:"running"`
	Queued       []buildQueueQueued  `json:"queued"`
	Recent       []buildQueueChange  `json:"recent"`
}

// buildAttemptSources start a build; image, restart and rollback sources never do.
var buildAttemptSources = map[string]bool{
	store.DeployAttemptSourceWebhook: true, store.DeployAttemptSourceGitPush: true, store.DeployAttemptSourceManual: true,
	store.DeployAttemptSourcePoll: true, store.DeployAttemptSourceSchedule: true, store.DeployAttemptSourceUpload: true,
}

// isBuildAttempt reports whether a running attempt is a build, so kill-all leaves image rollouts alone.
func isBuildAttempt(a store.DeployAttempt, building map[string]bool) bool {
	if building[a.ServiceName] {
		return true
	}
	return buildAttemptSources[a.Source] && !strings.HasPrefix(a.Reason, "RollbackTo")
}

func (rt *Router) activeBuilds() []build.ActiveBuild {
	if s, ok := rt.buildLoad.(buildActiveSource); ok {
		return s.Active()
	}
	return nil
}

func (rt *Router) buildQueueView(ctx context.Context) (buildQueueResource, error) {
	st, _ := rt.buildQueueState(ctx)
	now := time.Now()
	res := buildQueueResource{State: st.State, Reason: st.Reason, Actor: st.Actor,
		Running: []buildQueueRunning{}, Queued: []buildQueueQueued{}, Recent: []buildQueueChange{}}
	if !st.Since.IsZero() {
		since := st.Since
		res.Since = &since
		if after := rt.queueDuration(ctx, KeyBuildQueueAutoResumeAfter); after > 0 && st.State != store.BuildQueueRunning {
			at := since.Add(after)
			res.AutoResumeAt = &at
		}
	}
	active := rt.activeBuilds()
	byApp := map[string]build.ActiveBuild{}
	building := map[string]bool{}
	for _, b := range active {
		byApp[b.App] = b
		if b.Phase == build.ActiveBuilding {
			building[b.App] = true
		}
	}
	if rt.deploySafety != nil {
		running, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
		if err != nil {
			return res, err
		}
		for _, a := range running {
			if !isBuildAttempt(a, building) {
				continue
			}
			row := buildQueueRunning{DeployID: a.ID, App: a.ServiceName, Phase: "preparing"}
			started := a.StartedAt
			if b, ok := byApp[a.ServiceName]; ok {
				row.Node, row.Phase = b.Node, b.Phase
				delete(byApp, a.ServiceName)
			}
			row.StartedAt, row.ElapsedSeconds = &started, now.Sub(started).Seconds()
			res.Running = append(res.Running, row)
		}
		queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
		if err != nil {
			return res, err
		}
		sortQueued(queued)
		for i, a := range queued {
			q := buildQueueQueued{DeployID: a.ID, App: a.ServiceName, Position: i + 1, QueuedAt: a.QueuedAt}
			if a.QueuedAt != nil {
				q.WaitSeconds = now.Sub(*a.QueuedAt).Seconds()
			}
			q.WaitReason = rt.waitFor(ctx, a, append(sameApp(queued, a.ServiceName), sameApp(running, a.ServiceName)...)).Reason
			res.Queued = append(res.Queued, q)
		}
	}
	for _, b := range byApp {
		row := buildQueueRunning{App: b.App, Node: b.Node, Phase: b.Phase}
		at := b.QueuedAt
		if !b.StartedAt.IsZero() {
			at = b.StartedAt
		}
		row.StartedAt, row.ElapsedSeconds = &at, now.Sub(at).Seconds()
		res.Running = append(res.Running, row)
	}
	sort.SliceStable(res.Running, func(i, j int) bool { return res.Running[i].ElapsedSeconds > res.Running[j].ElapsedSeconds })
	res.Drained = len(building) == 0
	for _, r := range res.Running {
		if r.Phase == build.ActiveBuilding || r.Phase == "preparing" {
			res.Drained = false
		}
	}
	perNode := rt.buildLimitsView(ctx).Effective.MaxConcurrent
	nodes := 1
	if rt.buildLoad != nil {
		load := rt.buildLoad.Load()
		nodes = max(nodes, len(load))
		for _, l := range load {
			res.Slots.Used += l.Running
		}
	}
	res.Slots.Total = perNode * nodes
	if s := rt.buildQueueStore(); s != nil {
		if events, err := s.ListBuildQueueEvents(ctx, buildQueueRecentChanges); err == nil {
			for _, e := range events {
				res.Recent = append(res.Recent, buildQueueChange{State: e.State, Reason: e.Reason, Actor: e.Actor, At: e.At})
			}
		}
	}
	return res, nil
}

func sameApp(as []store.DeployAttempt, app string) []store.DeployAttempt {
	var out []store.DeployAttempt
	for _, a := range as {
		if a.ServiceName == app {
			out = append(out, a)
		}
	}
	return out
}

// filterBuildQueue keeps only the apps the caller may see; a nil list means all.
func filterBuildQueue(res *buildQueueResource, visible []string) {
	if visible == nil {
		return
	}
	allowed := make(map[string]bool, len(visible))
	for _, n := range visible {
		allowed[n] = true
	}
	running := res.Running[:0]
	for _, b := range res.Running {
		if allowed[b.App] {
			running = append(running, b)
		}
	}
	res.Running = running
	queued := res.Queued[:0]
	for _, q := range res.Queued {
		if allowed[q.App] {
			queued = append(queued, q)
		}
	}
	res.Queued = queued
}

// handleGetBuildQueue handles GET /api/v1/builds/queue.
func (rt *Router) handleGetBuildQueue(w http.ResponseWriter, r *http.Request) {
	res, err := rt.buildQueueView(r.Context())
	if err != nil {
		rt.internalError(w, "api: build queue: read failed", err)
		return
	}
	visible, err := rt.visibleAppNames(r)
	if err != nil {
		rt.internalError(w, "api: build queue: resolve visibility failed", err)
		return
	}
	filterBuildQueue(&res, visible)
	writeJSON(w, http.StatusOK, res)
}

type buildQueueChangeRequest struct {
	Reason string `json:"reason,omitempty"`
}

func readBuildQueueReason(w http.ResponseWriter, r *http.Request) (string, bool) {
	var req buildQueueChangeRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return "", false
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > buildQueueReasonMaxLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("reason must be at most %d characters", buildQueueReasonMaxLen))
		return "", false
	}
	return reason, true
}

func (rt *Router) changeBuildQueue(w http.ResponseWriter, r *http.Request, state string) {
	reason, ok := readBuildQueueReason(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	cur, _ := rt.buildQueueState(ctx)
	if cur.State == state {
		writeError(w, http.StatusConflict, "the build queue is already "+state)
		return
	}
	next := store.BuildQueueState{State: state, Reason: reason, Actor: rt.eventActor(r), Since: time.Now()}
	if err := rt.setBuildQueueState(ctx, next); err != nil {
		rt.internalError(w, "api: build queue: save state failed", err)
		return
	}
	if state == store.BuildQueueRunning {
		rt.DrainDeployQueue(context.WithoutCancel(ctx))
	}
	rt.handleGetBuildQueue(w, r)
}

// handleBuildQueuePause handles POST /api/v1/builds/queue/pause: new builds wait, running ones finish.
func (rt *Router) handleBuildQueuePause(w http.ResponseWriter, r *http.Request) {
	rt.changeBuildQueue(w, r, store.BuildQueuePaused)
}

// handleBuildQueueDrain handles POST /api/v1/builds/queue/drain.
func (rt *Router) handleBuildQueueDrain(w http.ResponseWriter, r *http.Request) {
	rt.changeBuildQueue(w, r, store.BuildQueueDraining)
}

// handleBuildQueueResume handles POST /api/v1/builds/queue/resume and starts what waited.
func (rt *Router) handleBuildQueueResume(w http.ResponseWriter, r *http.Request) {
	rt.changeBuildQueue(w, r, store.BuildQueueRunning)
}

type killAllBuildsRequest struct {
	Reason string       `json:"reason,omitempty"`
	Pause  bool         `json:"pause,omitempty"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type killAllBuildsItem struct {
	DeployID string `json:"deploy_id"`
	App      string `json:"app"`
	Was      string `json:"was"`
	Error    string `json:"error,omitempty"`
}

type killAllBuildsResponse struct {
	Canceled []killAllBuildsItem `json:"canceled"`
	Failed   []killAllBuildsItem `json:"failed"`
	Paused   bool                `json:"paused"`
}

// handleKillAllBuilds handles POST /api/v1/builds/kill-all: every queued and
// running build goes through the per-build cancel path, behind a fresh step-up.
func (rt *Router) handleKillAllBuilds(w http.ResponseWriter, r *http.Request) {
	if rt.deploySafety == nil {
		writeError(w, http.StatusNotImplemented, "deploy cancellation is not configured on this control plane")
		return
	}
	var req killAllBuildsRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Reason = strings.TrimSpace(req.Reason)
	if len(req.Reason) > buildQueueReasonMaxLen {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("reason must be at most %d characters", buildQueueReasonMaxLen))
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, buildQueueStepUpApp, "cancel every build", "canceling every build"); !ok {
		return
	}
	ctx := r.Context()
	by := rt.eventActor(r)
	out := killAllBuildsResponse{Canceled: []killAllBuildsItem{}, Failed: []killAllBuildsItem{}}
	if req.Pause {
		if cur, _ := rt.buildQueueState(ctx); cur.State == store.BuildQueueRunning {
			if err := rt.setBuildQueueState(ctx, store.BuildQueueState{State: store.BuildQueuePaused, Reason: req.Reason, Actor: by, Since: time.Now()}); err != nil {
				rt.internalError(w, "api: kill all builds: pause failed", err)
				return
			}
		}
		out.Paused = true
	}
	queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
	if err != nil {
		rt.internalError(w, "api: kill all builds: list queued failed", err)
		return
	}
	for _, a := range queued {
		item := killAllBuildsItem{DeployID: a.ID, App: a.ServiceName, Was: store.DeployAttemptStatusQueued}
		moved, cerr := rt.deploySafety.CancelDeployAttempt(ctx, a.ID, store.DeployAttemptStatusQueued, by, time.Now())
		switch {
		case cerr != nil:
			item.Error = "cancel failed"
			rt.logger.Warn("api: kill all builds: cancel queued failed", slog.String("attempt_id", a.ID), slog.String("error", cerr.Error()))
		case !moved:
			item.Error = "the build started or finished before the cancel landed"
		}
		out.add(item)
	}
	building := map[string]bool{}
	for _, b := range rt.activeBuilds() {
		building[b.App] = true
	}
	running, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
	if err != nil {
		rt.internalError(w, "api: kill all builds: list running failed", err)
		return
	}
	for _, a := range running {
		if !isBuildAttempt(a, building) {
			continue
		}
		out.add(rt.killRunningBuild(ctx, a, by))
	}
	rt.logger.Info("api: kill all builds", slog.Int("canceled", len(out.Canceled)), slog.Int("failed", len(out.Failed)), slog.String("by", by))
	rt.DrainDeployQueue(context.WithoutCancel(ctx))
	writeJSON(w, http.StatusOK, out)
}

func (o *killAllBuildsResponse) add(it killAllBuildsItem) {
	if it.Error != "" {
		o.Failed = append(o.Failed, it)
		return
	}
	o.Canceled = append(o.Canceled, it)
}

// killRunningBuild mirrors handleCancelDeploy for one running attempt without writing a response.
func (rt *Router) killRunningBuild(ctx context.Context, a store.DeployAttempt, by string) killAllBuildsItem {
	item := killAllBuildsItem{DeployID: a.ID, App: a.ServiceName, Was: store.DeployAttemptStatusRunning}
	switch rt.cancels.Cancel(a.ID, by) {
	case deploy.CancelTooLate:
		item.Error = "already cut over to the new release; roll back instead"
	case deploy.CancelUnknown:
		grace := time.Duration(rt.runtimeSettings.Limit(ctx, runtimesettings.KeyDeployOrphanGraceSeconds)) * time.Second
		rt.buildStartMu.Lock()
		starting := rt.startingDeploys[a.ServiceName] > 0
		rt.buildStartMu.Unlock()
		if starting || time.Since(a.StartedAt) < grace {
			item.Error = "not in progress on this control plane yet; retry shortly"
			return item
		}
		moved, err := rt.deploySafety.CancelOrphanedDeployAttempt(ctx, a.ID, by, time.Now())
		if err != nil || !moved {
			item.Error = "the build finished before the cancel landed"
		}
	default:
		if _, err := rt.deploySafety.CancelDeployAttempt(ctx, a.ID, store.DeployAttemptStatusRunning, by, time.Now()); err != nil {
			rt.logger.Warn("api: kill all builds: record cancel failed", slog.String("attempt_id", a.ID), slog.String("error", err.Error()))
		}
	}
	return item
}

// doctorCheckBuildQueue warns when the build queue stayed held past the warning setting.
func (rt *Router) doctorCheckBuildQueue(ctx context.Context) doctorCheckResource {
	const code, name = "build_queue", "Build queue"
	st, _ := rt.buildQueueState(ctx)
	if st.State == store.BuildQueueRunning {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "The build queue is running."}
	}
	held := time.Since(st.Since).Round(time.Minute)
	warn := rt.queueDuration(ctx, KeyBuildQueuePausedWarnAfter)
	msg := fmt.Sprintf("The build queue is %s (%s) for %s; new builds wait until it resumes", st.State, buildQueueWaitReason(st), held)
	if warn > 0 && !st.Since.IsZero() && time.Since(st.Since) >= warn {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg,
			Fix: "Resume the queue on the Deployments page or run levelrail-cli builds resume."}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
}

// buildQueueAttentionItems asks to resume a queue held past the warning setting.
func (rt *Router) buildQueueAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	ctx := r.Context()
	st, _ := rt.buildQueueState(ctx)
	warn := rt.queueDuration(ctx, KeyBuildQueuePausedWarnAfter)
	if st.State == store.BuildQueueRunning || warn <= 0 || st.Since.IsZero() || time.Since(st.Since) < warn {
		return nil
	}
	it := feedItem(attention.Warning, attention.KindBuildQueuePaused, "builds", buildQueueWaitReason(st)+"; new builds wait until the queue resumes", map[string]string{"state": st.State})
	it.Title = "Build queue is " + st.State
	return []attention.Item{it}
}
