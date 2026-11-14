package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A stack plan with cloud kinds runs the engine for minutes, so it becomes a job: the
// plan endpoint answers 202 and the caller polls. Job rows keep the masked plan and
// codes only; a job whose process stopped beating is reported interrupted.

const (
	stackEventPlanStarted        = "stacks.plan_started"
	stackEventPlanFinished       = "stacks.plan_finished"
	auditStackApprovalSuperseded = "stack.approval_superseded"

	// Fallbacks when no settings service is wired; the registry holds the real defaults.
	defaultMaxConcurrentPlans  = 2
	defaultPlanJobRetention    = 24 * time.Hour
	defaultCloudEngineTimeout  = 1800 * time.Second
	supersededApprovalErrorTag = "superseded"
)

// planJobNow is a test seam for heartbeat staleness, retention and expiry.
var planJobNow = time.Now

type stackPlanJobStore interface {
	InsertStackPlanJob(ctx context.Context, j store.StackPlanJob) error
	GetStackPlanJob(ctx context.Context, stack, id string) (store.StackPlanJob, error)
	ListStackPlanJobsSince(ctx context.Context, stack string, since time.Time) ([]store.StackPlanJob, error)
	StartStackPlanJob(ctx context.Context, id string, now time.Time) (bool, error)
	HeartbeatStackPlanJob(ctx context.Context, id string, now time.Time) error
	FinishStackPlanJob(ctx context.Context, j store.StackPlanJob) (bool, error)
	InterruptStalePlanJobs(ctx context.Context, code, msg string, staleBefore, now time.Time) error
	PruneStackPlanJobs(ctx context.Context, before time.Time) error
}

type stackPlanJobResponse struct {
	ID         string             `json:"id"`
	Stack      string             `json:"stack"`
	Status     string             `json:"status"`
	Code       string             `json:"code,omitempty"`
	Error      string             `json:"error,omitempty"`
	CreatedAt  string             `json:"created_at"`
	StartedAt  string             `json:"started_at,omitempty"`
	FinishedAt string             `json:"finished_at,omitempty"`
	PollURL    string             `json:"poll_url,omitempty"`
	Result     *stackPlanResponse `json:"result,omitempty"`
}

// planJobGate caps cloud plans across stacks and runs one plan per stack at a time.
type planJobGate struct {
	mu      sync.Mutex
	cond    *sync.Cond
	running int
	stacks  map[string]bool
	jobs    sync.WaitGroup
}

var planGates sync.Map

func (rt *Router) planGate() *planJobGate {
	if g, ok := planGates.Load(rt); ok {
		return g.(*planJobGate)
	}
	g := &planJobGate{stacks: map[string]bool{}}
	g.cond = sync.NewCond(&g.mu)
	actual, _ := planGates.LoadOrStore(rt, g)
	return actual.(*planJobGate)
}

// acquire re-reads the cap on every wake so a changed setting applies to queued jobs.
func (g *planJobGate) acquire(stack string, capOf func() int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.running >= capOf() || g.stacks[stack] {
		g.cond.Wait()
	}
	g.running++
	g.stacks[stack] = true
}

func (g *planJobGate) release(stack string) {
	g.mu.Lock()
	g.running--
	delete(g.stacks, stack)
	g.mu.Unlock()
	g.cond.Broadcast()
}

// waitPlanJobs blocks until every job this router started has ended; tests use it.
func (rt *Router) waitPlanJobs() { rt.planGate().jobs.Wait() }

func (rt *Router) settingInt(ctx context.Context, key string, def int) int {
	if rt.runtimeSettings != nil {
		if v := rt.runtimeSettings.Int(ctx, key); v > 0 {
			return v
		}
	}
	return def
}

func (rt *Router) planJobRetention(ctx context.Context) time.Duration {
	return time.Duration(rt.settingInt(ctx, runtimesettings.KeyCloudPlanJobRetentionHours, int(defaultPlanJobRetention/time.Hour))) * time.Hour
}

func (rt *Router) cloudEngineTimeout(ctx context.Context) time.Duration {
	return time.Duration(rt.settingInt(ctx, runtimesettings.KeyCloudEngineTimeoutSeconds, int(defaultCloudEngineTimeout/time.Second))) * time.Second
}

// sweepPlanJobs interrupts jobs whose process stopped beating and prunes old ones, so no daemon is needed.
func (rt *Router) sweepPlanJobs(ctx context.Context, js stackPlanJobStore, now time.Time, prune bool) {
	msg := "the control plane stopped while this plan ran; plan again"
	if err := js.InterruptStalePlanJobs(ctx, iac.CodeInterrupted, msg, now.Add(-rt.stackLockTTL(ctx)), now); err != nil {
		rt.logger.Warn("api: plan job sweep failed", slog.String("error", err.Error()))
	}
	if prune {
		if err := js.PruneStackPlanJobs(ctx, now.Add(-rt.planJobRetention(ctx))); err != nil {
			rt.logger.Warn("api: plan job prune failed", slog.String("error", err.Error()))
		}
	}
}

func newPlanJobID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "pj_" + hex.EncodeToString(b)
}

// planJobInput is everything a queued plan needs after the request returned.
type planJobInput struct {
	stack     store.StackRecord
	exists    bool
	name      string
	project   string
	res       []*iac.Resource
	opts      iac.Options
	req       stackRequest
	resp      stackPlanResponse
	supersede bool
}

// pendingCloudApproval finds a revision awaiting approval whose saved engine plan a new plan would replace.
func (rt *Router) pendingCloudApproval(ctx context.Context, ss StackStore, stack store.StackRecord) (store.StackRevisionRecord, bool, error) {
	rt.expireStackApprovals(ctx, ss, stack)
	revs, err := ss.ListStackRevisions(ctx, stack.ID, rt.stackHistoryLimit(ctx))
	if err != nil {
		return store.StackRevisionRecord{}, false, err
	}
	for _, v := range revs {
		if v.Status != store.StackRevPendingApproval {
			continue
		}
		if _, cloud := revisionCloudPlan(v); cloud {
			return v, true, nil
		}
	}
	return store.StackRevisionRecord{}, false, nil
}

func approvalPendingMessage(stack string, n int) string {
	return fmt.Sprintf("revision %d of stack %s awaits approval; a new cloud plan would replace its saved engine plan. Approve or reject it first, or plan with supersede=true to retire it", n, stack)
}

// supersedePending retires a pending approval so it can no longer be approved, with an audit row.
func (rt *Router) supersedePending(r *http.Request, ss StackStore, stack store.StackRecord, rev store.StackRevisionRecord, jobID string) error {
	result, _ := json.Marshal(map[string]string{"error": supersededApprovalErrorTag, "superseded_by": jobID})
	ok, err := failPending(context.WithoutCancel(r.Context()), ss, stack.ID, rev.Number, result, time.Now())
	if err != nil || !ok {
		return err
	}
	rt.auditStack(r, auditStackApprovalSuperseded, map[string]any{"stack": stack.Name, "revision": rev.Number, "plan_hash": rev.PlanHash, "plan_job": jobID})
	return nil
}

// revisionSuperseded reports a pending approval a later plan retired.
func revisionSuperseded(v store.StackRevisionRecord) bool {
	if v.Status != store.StackRevFailed || len(v.ResultJSON) == 0 {
		return false
	}
	var out struct {
		Error string `json:"error"`
	}
	return json.Unmarshal(v.ResultJSON, &out) == nil && out.Error == supersededApprovalErrorTag
}

func writePlanSuperseded(w http.ResponseWriter, n int) {
	writeIaCError(w, http.StatusConflict, iac.CodePlanSuperseded,
		fmt.Sprintf("a later plan replaced the saved engine plan of revision %d; plan again and request a new approval", n), nil)
}

func cloudHashesOf(plan iac.Plan) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range plan.Changes {
		if c.CloudPlanHash != "" && !seen[c.CloudPlanHash] {
			seen[c.CloudPlanHash] = true
			out = append(out, c.CloudPlanHash)
		}
	}
	return out
}

func hashesOutside(mine map[string]bool, other []string) bool {
	for _, h := range other {
		if !mine[h] {
			return true
		}
	}
	return false
}

// cloudPlanReplaced reports whether a newer revision or plan job saved another engine
// plan for the stack after this revision, so its approval would hit an engine mismatch.
func (rt *Router) cloudPlanReplaced(ctx context.Context, ss StackStore, stack store.StackRecord, rev store.StackRevisionRecord, plan iac.Plan) (bool, error) {
	mine := map[string]bool{}
	for _, h := range cloudHashesOf(plan) {
		mine[h] = true
	}
	revs, err := ss.ListStackRevisions(ctx, stack.ID, rt.stackHistoryLimit(ctx))
	if err != nil {
		return false, err
	}
	for _, v := range revs {
		if v.Number <= rev.Number {
			continue
		}
		if p, cloud := revisionCloudPlan(v); cloud && hashesOutside(mine, cloudHashesOf(p)) {
			return true, nil
		}
	}
	js, ok := ss.(stackPlanJobStore)
	if !ok {
		return false, nil
	}
	jobs, err := js.ListStackPlanJobsSince(ctx, stack.Name, rev.CreatedAt.Add(-rt.planJobRetention(ctx)))
	if err != nil {
		return false, err
	}
	for _, j := range jobs {
		if j.StartedAt.IsZero() || j.StartedAt.Before(rev.CreatedAt) {
			continue
		}
		// An interrupted or timed out engine run may have saved its plan before stopping.
		if (j.Status == store.PlanJobDone && hashesOutside(mine, j.CloudHashes)) ||
			(j.Status == store.PlanJobFailed && (j.Code == iac.CodeInterrupted || j.Code == iac.CodePlanTimeout)) {
			return true, nil
		}
	}
	return false, nil
}

// startCloudPlanJob answers 409 approval_pending, or queues the plan and answers 202.
func (rt *Router) startCloudPlanJob(w http.ResponseWriter, r *http.Request, ss StackStore, in planJobInput) {
	js, ok := ss.(stackPlanJobStore)
	if !ok {
		writeIaCError(w, http.StatusNotImplemented, iac.CodeUnavailable, "async cloud plans are not available on this store", nil)
		return
	}
	ctx := r.Context()
	in.supersede = r.URL.Query().Get("supersede") == "true"
	jobID := newPlanJobID()
	if in.exists {
		rev, pending, err := rt.pendingCloudApproval(ctx, ss, in.stack)
		if err != nil {
			rt.writeStackError(w, "plan", err)
			return
		}
		if pending && !in.supersede {
			writeIaCError(w, http.StatusConflict, iac.CodeApprovalPending, approvalPendingMessage(in.name, rev.Number), nil)
			return
		}
		if pending {
			if err := rt.supersedePending(r, ss, in.stack, rev, jobID); err != nil {
				rt.writeStackError(w, "plan", err)
				return
			}
		}
	}
	now := planJobNow()
	rt.sweepPlanJobs(ctx, js, now, true)
	caller := rt.stackCaller(r)
	job := store.StackPlanJob{ID: jobID, Stack: in.name, Actor: caller.name, CreatedAt: now}
	if err := js.InsertStackPlanJob(ctx, job); err != nil {
		rt.internalError(w, "api: plan job insert failed", err)
		return
	}
	g := rt.planGate()
	g.jobs.Add(1)
	go rt.runPlanJob(r.Clone(context.WithoutCancel(ctx)), ss, js, job, in)
	poll := "/api/v1/stacks/" + in.name + "/plan-jobs/" + jobID
	w.Header().Set("Location", poll)
	writeJSON(w, http.StatusAccepted, stackPlanJobResponse{ID: jobID, Stack: in.name, Status: store.PlanJobQueued, CreatedAt: fmtStackTime(now), PollURL: poll})
}

// beatPlanJob keeps the job visibly alive at the stack lease renew rate until stopped.
func (rt *Router) beatPlanJob(ctx context.Context, js stackPlanJobStore, id string) func() {
	every := stackLockRenewEvery(rt.stackLockTTL(ctx), rt.stackLockRenewFraction(ctx))
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				_ = js.HeartbeatStackPlanJob(ctx, id, planJobNow())
			}
		}
	}()
	return func() { close(stop); <-done }
}

type planOutcome struct {
	plan iac.Plan
	err  error
}

func (rt *Router) runPlanJob(r *http.Request, ss StackStore, js stackPlanJobStore, job store.StackPlanJob, in planJobInput) {
	g := rt.planGate()
	defer g.jobs.Done()
	ctx := r.Context()
	stopBeat := rt.beatPlanJob(ctx, js, job.ID)
	defer stopBeat()
	g.acquire(job.Stack, func() int {
		return rt.settingInt(ctx, runtimesettings.KeyCloudMaxConcurrentPlans, defaultMaxConcurrentPlans)
	})
	finish := func(status, code, msg string, plan *iac.Plan) {
		out := store.StackPlanJob{ID: job.ID, Status: status, Code: code, Error: msg, FinishedAt: planJobNow()}
		if plan != nil {
			resp := in.resp
			resp.Plan = plan
			out.Result, _ = json.Marshal(resp)
			out.CloudHashes = cloudHashesOf(*plan)
		}
		if ok, err := js.FinishStackPlanJob(context.WithoutCancel(ctx), out); err != nil || !ok {
			return
		}
		rt.emitPlanJobEvent(ctx, stackEventPlanFinished, job, plan, status, code)
	}
	planCtx, endLease := ctx, func() {}
	if in.exists {
		// A pending approval may have appeared while this job waited in the queue.
		rev, pending, err := rt.pendingCloudApproval(ctx, ss, in.stack)
		if err == nil && pending && !in.supersede {
			g.release(job.Stack)
			finish(store.PlanJobFailed, iac.CodeApprovalPending, approvalPendingMessage(in.name, rev.Number), nil)
			return
		}
		if err == nil && pending {
			err = rt.supersedePending(r, ss, in.stack, rev, job.ID)
		}
		holder := "planjob:" + job.ID
		ttl := rt.stackLockTTL(ctx)
		if err == nil {
			_, err = ss.AcquireStackLock(ctx, in.stack.ID, holder, 0, ttl, time.Now())
		}
		if err != nil {
			g.release(job.Stack)
			code := iac.CodeItemFailed
			if errors.Is(err, store.ErrStackLocked) {
				code = iac.CodeStackLocked
			}
			finish(store.PlanJobFailed, code, "the stack is busy or could not be read; plan again", nil)
			return
		}
		lctx, stop := keepStackLease(ctx, ss, in.stack.ID, holder, ttl, rt.stackLockRenewFraction(ctx))
		planCtx = lctx
		endLease = func() {
			stop()
			_ = ss.ReleaseStackLock(context.WithoutCancel(ctx), in.stack.ID, holder)
		}
	}
	if ok, err := js.StartStackPlanJob(ctx, job.ID, planJobNow()); err != nil || !ok {
		endLease()
		g.release(job.Stack)
		return
	}
	rt.emitPlanJobEvent(ctx, stackEventPlanStarted, job, nil, store.PlanJobRunning, "")
	// The job context carries the engine timeout, so the engine stops when the job does and
	// the slot frees at once instead of waiting for the engine to unwind.
	jobCtx, cancelJob := context.WithTimeout(planCtx, rt.planJobTimeout(ctx))
	defer cancelJob()
	free := func() { endLease(); g.release(job.Stack) }
	done := make(chan planOutcome, 1)
	go func() {
		plan, err := rt.computeStackPlan(jobCtx, r, ss, in)
		done <- planOutcome{plan: plan, err: err}
	}()
	select {
	case o := <-done:
		ended := jobCtx.Err() != nil
		free()
		if ended {
			rt.finishPlanJobCtx(jobCtx, finish)
			return
		}
		if o.err != nil {
			rt.logger.Warn("api: cloud plan job failed", slog.String("stack", job.Stack), slog.String("job", job.ID), slog.String("error", o.err.Error()))
			finish(store.PlanJobFailed, iac.CodeItemFailed, "the plan could not be computed; see the control plane log", nil)
			return
		}
		finish(store.PlanJobDone, "", "", &o.plan)
	case <-jobCtx.Done():
		free()
		rt.finishPlanJobCtx(jobCtx, finish)
	}
}

// planJobTimeoutOverride replaces the engine timeout in tests, since the setting floor is a minute.
var planJobTimeoutOverride time.Duration

func (rt *Router) planJobTimeout(ctx context.Context) time.Duration {
	if planJobTimeoutOverride > 0 {
		return planJobTimeoutOverride
	}
	return rt.cloudEngineTimeout(ctx)
}

// finishPlanJobCtx reports a job whose context ended: past the engine timeout or canceled.
func (rt *Router) finishPlanJobCtx(ctx context.Context, finish func(status, code, msg string, plan *iac.Plan)) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		finish(store.PlanJobFailed, iac.CodePlanTimeout, "the plan ran past cloud.engine_timeout_seconds; plan again", nil)
		return
	}
	finish(store.PlanJobFailed, iac.CodeInterrupted, "the plan was canceled; plan again", nil)
}

func (rt *Router) computeStackPlan(ctx context.Context, r *http.Request, ss StackStore, in planJobInput) (iac.Plan, error) {
	lctx, err := rt.stackLedgerContext(ctx, ss, in.stack.ID, in.name, in.project, in.stack.Scope, in.res, in.req)
	if err != nil {
		return iac.Plan{}, err
	}
	st, err := iac.Load(lctx, rt.iacDoer(r), in.res, in.opts)
	if err != nil {
		return iac.Plan{}, err
	}
	return iac.PlanForContext(ctx, st, in.res, in.opts), nil
}

// emitPlanJobEvent carries the job id, status, code and counts, never a field value.
func (rt *Router) emitPlanJobEvent(ctx context.Context, typ string, job store.StackPlanJob, plan *iac.Plan, status, code string) {
	if rt.eventWebhooks == nil {
		return
	}
	var names []string
	var counts map[string]int
	hash := ""
	if plan != nil {
		for _, c := range plan.Changes {
			if c.Action != iac.ActionNoop {
				names = append(names, string(c.Kind)+"/"+c.Name)
			}
		}
		counts = map[string]int{"create": plan.Summary.Create, "update": plan.Summary.Update, "delete": plan.Summary.Delete, "error": plan.Summary.Error}
		hash = plan.Hash
	}
	maxNames := 0
	if rt.runtimeSettings != nil {
		maxNames = rt.runtimeSettings.Int(ctx, runtimesettings.KeyStackDriftSummaryMax)
	}
	data := stackEventPayload(job.Stack, job.Actor, hash, 0, names, counts, maxNames)
	data["plan_job"], data["status"] = job.ID, status
	if code != "" {
		data["code"] = code
	}
	rt.eventWebhooks.Emit(eventhook.Event{Type: typ, Data: data})
}

func (rt *Router) planJobView(ctx context.Context, j store.StackPlanJob, now time.Time) stackPlanJobResponse {
	out := stackPlanJobResponse{ID: j.ID, Stack: j.Stack, Status: j.Status, Code: j.Code, Error: j.Error, CreatedAt: fmtStackTime(j.CreatedAt)}
	if !j.StartedAt.IsZero() {
		out.StartedAt = fmtStackTime(j.StartedAt)
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = fmtStackTime(j.FinishedAt)
	}
	if j.Status == store.PlanJobDone {
		maxAge := time.Duration(rt.settingInt(ctx, runtimesettings.KeyCloudPlanMaxAgeMinutes, 0)) * time.Minute
		if maxAge > 0 && now.Sub(j.FinishedAt) > maxAge {
			out.Status = store.PlanJobExpired
		}
		var resp stackPlanResponse
		if json.Unmarshal(j.Result, &resp) == nil {
			out.Result = &resp
		}
	}
	return out
}

func (rt *Router) handleStackPlanJob(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	js, ok := ss.(stackPlanJobStore)
	if !ok {
		writeIaCError(w, http.StatusNotImplemented, iac.CodeUnavailable, "async cloud plans are not available on this store", nil)
		return
	}
	ctx := r.Context()
	now := planJobNow()
	rt.sweepPlanJobs(ctx, js, now, false)
	j, err := js.GetStackPlanJob(ctx, r.PathValue("name"), r.PathValue("id"))
	if errors.Is(err, store.ErrPlanJobNotFound) {
		writeIaCError(w, http.StatusNotFound, iac.CodePlanJobNotFound, "no plan job with this id for the stack; it may have been removed after cloud.plan_job_retention_hours", nil)
		return
	}
	if err != nil {
		rt.internalError(w, "api: plan job read failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.planJobView(ctx, j, now))
}
