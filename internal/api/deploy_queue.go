package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DeployQueueStore is the store surface for queued and canceled deploys.
// *store.DB satisfies this structurally.
type DeployQueueStore interface {
	ListQueuedDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
	ListRunningDeployAttempts(ctx context.Context) ([]store.DeployAttempt, error)
	StartQueuedDeployAttempt(ctx context.Context, id string, now time.Time) (bool, error)
	FinishRunningDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) (bool, error)
	CancelDeployAttempt(ctx context.Context, id, from, by string, now time.Time) (bool, error)
	SupersedeQueuedDeployAttempt(ctx context.Context, id, by string, now time.Time) (bool, error)
	GetServiceCancelSuperseded(ctx context.Context, name string) (bool, error)
	SetServiceCancelSuperseded(ctx context.Context, name string, enabled bool) error
}

type adoptedAttemptKey struct{}

// withAdoptedAttempt makes the next beginBuildDeployAttempt reuse an existing
// attempt row (a queued deploy that just started) instead of minting one.
func withAdoptedAttempt(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, adoptedAttemptKey{}, id)
}

type startReleaseKey struct{}

// withStartRelease hands the deploy the func that ends its "starting" mark,
// called once its attempt row exists and counts as running.
func withStartRelease(ctx context.Context, release func()) context.Context {
	return context.WithValue(ctx, startReleaseKey{}, release)
}

func releaseStartMark(ctx context.Context) {
	if release, ok := ctx.Value(startReleaseKey{}).(func()); ok {
		release()
	}
}

func adoptedAttemptID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(adoptedAttemptKey{}).(string)
	return id, ok && id != ""
}

// deployBlocker reports whether a new deploy of name must wait, and why. The
// caller holds buildStartMu so the answer and the row insert are one step.
func (rt *Router) deployBlocker(ctx context.Context, name string) bool {
	running, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
	if err != nil {
		rt.logger.Error("api: deploy queue: list running attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		return false
	}
	if rt.startingDeploys[name] > 0 {
		return true
	}
	total := len(running)
	for _, n := range rt.startingDeploys {
		total += n
	}
	for _, a := range running {
		if a.ServiceName == name {
			return true
		}
	}
	queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
	if err != nil {
		rt.logger.Error("api: deploy queue: list queued attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		return false
	}
	for _, a := range queued {
		if a.ServiceName == name {
			return true
		}
	}
	if rt.deployMaxConcurrent > 0 && total >= rt.deployMaxConcurrent {
		return true
	}
	return false
}

// queueIfBusy parks a webhook deploy behind a running one. When it does not
// queue, the returned release must be called once the deploy has recorded its
// attempt, so the fetch window still counts as busy for concurrent requests.
func (rt *Router) queueIfBusy(ctx context.Context, name string, gs store.GitSource, req deploy.HeldRequest, image string) (queued bool, msg string, release func()) {
	release = func() {}
	if rt.deploySafety == nil || len(gs.Services) > 0 {
		return false, "", release
	}
	rt.buildStartMu.Lock()
	defer rt.buildStartMu.Unlock()
	blocked := rt.deployBlocker(ctx, name)
	if !blocked {
		rt.startingDeploys[name]++
		var once sync.Once
		return false, "", func() {
			once.Do(func() {
				rt.buildStartMu.Lock()
				defer rt.buildStartMu.Unlock()
				if rt.startingDeploys[name]--; rt.startingDeploys[name] <= 0 {
					delete(rt.startingDeploys, name)
				}
			})
		}
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.logger.Error("api: deploy queue: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		return false, "", release
	}
	id, err := rt.enqueueDeploy(ctx, *existing, store.DeployAttemptSourceWebhook, image, req)
	if err != nil {
		rt.logger.Error("api: deploy queue: enqueue failed", slog.String("error", err.Error()), slog.String("name", name))
		return false, "", release
	}
	return true, fmt.Sprintf("queued: deploy %s will run when the app's current deploy finishes\n", id), release
}

// enqueueDeploy saves a queued attempt for hr and supersedes older queued
// deploys of the same branch when the app opted in. The caller holds buildStartMu.
func (rt *Router) enqueueDeploy(ctx context.Context, svc store.DesiredService, source, image string, hr deploy.HeldRequest) (string, error) {
	payload, err := json.Marshal(hr)
	if err != nil {
		return "", fmt.Errorf("marshal queued request: %w", err)
	}
	id, err := store.NewDeployAttemptID()
	if err != nil {
		return "", err
	}
	now := time.Now()
	meta := commitMetaFrom(ctx)
	if err := rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: svc.Name, Image: image, CommitSHA: hr.CommitSHA, Source: source,
		Status: store.DeployAttemptStatusQueued, StartedAt: now, QueuedAt: &now,
		Snapshot: store.NewDeployAttemptSnapshot(svc), HeldRequest: string(payload),
		Branch: meta.Branch, CommitMessage: meta.Message, Author: meta.Author,
	}); err != nil {
		return "", fmt.Errorf("save queued deploy attempt: %w", err)
	}
	rt.supersedeQueued(ctx, svc.Name, id, hr.Branch)
	return id, nil
}

// supersedeQueued replaces older queued deploys of branch with newID. Only
// queued rows move: a deploy that already started is never auto-canceled.
func (rt *Router) supersedeQueued(ctx context.Context, name, newID, branch string) {
	if branch == "" {
		return
	}
	on, err := rt.deploySafety.GetServiceCancelSuperseded(ctx, name)
	if err != nil || !on {
		if err != nil {
			rt.logger.Warn("api: deploy queue: read cancel_superseded failed", slog.String("error", err.Error()), slog.String("name", name))
		}
		return
	}
	queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
	if err != nil {
		rt.logger.Warn("api: deploy queue: list queued for supersede failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	for _, old := range queued {
		if old.ID == newID || old.ServiceName != name || queuedBranch(old) != branch {
			continue
		}
		if _, err := rt.deploySafety.SupersedeQueuedDeployAttempt(ctx, old.ID, newID, time.Now()); err != nil {
			rt.logger.Warn("api: deploy queue: supersede failed", slog.String("attempt_id", old.ID), slog.String("error", err.Error()))
		}
	}
}

func queuedRequest(a store.DeployAttempt) (deploy.HeldRequest, bool) {
	var hr deploy.HeldRequest
	if a.HeldRequest == "" || json.Unmarshal([]byte(a.HeldRequest), &hr) != nil {
		return hr, false
	}
	return hr, true
}

func queuedBranch(a store.DeployAttempt) string {
	hr, _ := queuedRequest(a)
	return hr.Branch
}

// enqueueManualBuild queues a manual build behind the app's current deploy.
// The caller holds buildStartMu.
func (rt *Router) enqueueManualBuild(ctx context.Context, existing store.DesiredService, req triggerBuildRequest, buildReq deploy.Request, freezeNote string, allowPrivate bool) (triggerBuildResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return triggerBuildResponse{}, fmt.Errorf("marshal build request: %w", err)
	}
	hr := deploy.HeldRequest{
		Kind: deploy.HeldKindBuild, Build: body, AllowPrivateRepoAuth: allowPrivate,
		FreezeNote: freezeNote, CommitLabel: req.Ref,
	}
	if req.Build.Type != spec.BuildImage {
		hr.Branch = req.Ref
	}
	id, err := rt.enqueueDeploy(ctx, existing, store.DeployAttemptSourceManual, buildReq.ImageRepo+":"+buildReq.CommitSHA, hr)
	if err != nil {
		return triggerBuildResponse{}, err
	}
	resp := triggerBuildResponse{ID: id, Status: store.DeployAttemptStatusQueued}
	if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, existing.Name); err == nil {
		for _, a := range attempts {
			if a.ID == id {
				w := rt.waitFor(ctx, a, attempts)
				resp.QueuePosition, resp.WaitReason = w.Position, w.Reason
			}
		}
	}
	return resp, nil
}

// DrainDeployQueue starts every queued deploy that is no longer blocked. It
// runs after each deploy finishes and on a timer, so a queue survives restarts.
func (rt *Router) DrainDeployQueue(ctx context.Context) {
	if rt.deploySafety == nil {
		return
	}
	rt.buildStartMu.Lock()
	started := rt.startQueued(ctx)
	rt.buildStartMu.Unlock()
	for _, a := range started {
		go rt.runQueued(a) //nolint:gosec // queued deploys outlive any request
	}
}

func (rt *Router) startQueued(ctx context.Context) []store.DeployAttempt {
	queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
	if err != nil || len(queued) == 0 {
		if err != nil {
			rt.logger.Error("api: deploy queue: list queued failed", slog.String("error", err.Error()))
		}
		return nil
	}
	running, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
	if err != nil {
		rt.logger.Error("api: deploy queue: list running failed", slog.String("error", err.Error()))
		return nil
	}
	busy := map[string]bool{}
	total := len(running)
	for _, a := range running {
		busy[a.ServiceName] = true
	}
	for name, n := range rt.startingDeploys {
		busy[name] = true
		total += n
	}
	var started []store.DeployAttempt
	for _, a := range queued {
		if busy[a.ServiceName] {
			continue
		}
		if rt.deployMaxConcurrent > 0 && total >= rt.deployMaxConcurrent {
			break
		}
		if rt.queuedFrozen(ctx, a) {
			busy[a.ServiceName] = true
			continue
		}
		moved, err := rt.deploySafety.StartQueuedDeployAttempt(ctx, a.ID, time.Now())
		if err != nil || !moved {
			if err != nil {
				rt.logger.Error("api: deploy queue: start failed", slog.String("attempt_id", a.ID), slog.String("error", err.Error()))
			}
			continue
		}
		busy[a.ServiceName] = true
		total++
		started = append(started, a)
	}
	return started
}

func (rt *Router) queuedFrozen(ctx context.Context, a store.DeployAttempt) bool {
	if hr, ok := queuedRequest(a); ok && hr.FreezeNote != "" {
		return false
	}
	status, err := deploy.CheckFreeze(ctx, rt.deploySafety, a.ServiceName, time.Now())
	return err == nil && status.Frozen
}

// runQueued executes a queued deploy that startQueued already moved to running.
func (rt *Router) runQueued(a store.DeployAttempt) {
	ctx := withAdoptedAttempt(context.Background(), a.ID) //nolint:gosec // a queued deploy outlives any request
	err := rt.replayQueued(ctx, a)
	status, msg := store.DeployAttemptStatusSucceeded, ""
	if err != nil {
		status, msg = store.DeployAttemptStatusFailed, err.Error()
		rt.logger.Warn("api: deploy queue: queued deploy failed", slog.String("attempt_id", a.ID), slog.String("name", a.ServiceName), slog.String("error", msg))
	}
	if _, ferr := rt.deploySafety.FinishRunningDeployAttempt(ctx, a.ID, status, time.Now(), msg); ferr != nil {
		rt.logger.Error("api: deploy queue: finish queued deploy failed", slog.String("attempt_id", a.ID), slog.String("error", ferr.Error()))
	}
	rt.cancels.Release(a.ID)
	rt.DrainDeployQueue(ctx)
}

func (rt *Router) replayQueued(ctx context.Context, a store.DeployAttempt) error {
	hr, ok := queuedRequest(a)
	if !ok {
		return errors.New("queued request unreadable")
	}
	switch hr.Kind {
	case deploy.HeldKindGit:
		return rt.replayGitDeploy(ctx, a.ServiceName, hr)
	case deploy.HeldKindBuild:
		return rt.replayManualBuild(ctx, a.ServiceName, hr)
	default:
		return fmt.Errorf("no queue handler for %q", hr.Kind)
	}
}

func (rt *Router) replayManualBuild(ctx context.Context, name string, hr deploy.HeldRequest) error {
	if rt.builder == nil {
		return errors.New("manual build trigger is not configured on this control plane")
	}
	var req triggerBuildRequest
	if err := json.Unmarshal(hr.Build, &req); err != nil {
		return fmt.Errorf("decode queued build request: %w", err)
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return fmt.Errorf("load app: %w", err)
	}
	buildType := req.Build.Type
	if buildType == "" {
		buildType = spec.BuildDockerfile
	}
	buildReq := manualBuildRequest(name, *existing, req, buildType)
	run := rt.beginManualBuild(ctx, *existing, buildReq, buildType, store.DeployAttemptSourceManual, req.DetectedFramework, hr.FreezeNote, hr.AllowPrivateRepoAuth)
	run.repoURL, run.ref = req.RepoURL, req.Ref
	rt.runManualBuild(run)
	return nil
}

// drainAfterFinish releases a finished deploy's cancel handle and lets the
// next queued deploy start.
func (rt *Router) drainAfterFinish(ctx context.Context, id string) {
	rt.cancels.Release(id)
	rt.DrainDeployQueue(ctx)
}

// finishCanceled records a build that failed because an operator canceled it
// as canceled rather than failed. It reports whether it handled the attempt.
func (rt *Router) finishCanceled(ctx context.Context, id string, deployErr error) bool {
	by, ok := rt.cancels.Canceled(id)
	if !ok || deployErr == nil || rt.deploySafety == nil {
		return false
	}
	if _, err := rt.deploySafety.CancelDeployAttempt(ctx, id, store.DeployAttemptStatusRunning, by, time.Now()); err != nil {
		rt.logger.Error("api: record canceled deploy failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
	}
	return true
}

// waitState is what a queued or held deploy is waiting on.
type waitState struct {
	Position  int
	Reason    string
	BlockedBy string
}

// waitFor computes the wait state of a queued or held attempt from its app's attempts.
func (rt *Router) waitFor(ctx context.Context, a store.DeployAttempt, app []store.DeployAttempt) waitState {
	switch a.Status {
	case store.DeployAttemptStatusHeld:
		return waitState{Reason: heldWaitReason(a.Reason)}
	case store.DeployAttemptStatusQueued:
	default:
		return waitState{}
	}
	var frozenUntil string
	var frozen bool
	if rt.deploySafety != nil && !queuedFreezeExempt(a) {
		if st, err := deploy.CheckFreeze(ctx, rt.deploySafety, a.ServiceName, time.Now()); err == nil && st.Frozen {
			frozen = true
			if !st.Until.IsZero() {
				frozenUntil = st.Until.UTC().Format(time.RFC3339)
			}
		}
	}
	full := false
	if rt.deployMaxConcurrent > 0 && rt.deploySafety != nil {
		if running, err := rt.deploySafety.ListRunningDeployAttempts(ctx); err == nil {
			full = len(running) >= rt.deployMaxConcurrent
		}
	}
	return computeWait(a, app, frozen, frozenUntil, full)
}

func queuedFreezeExempt(a store.DeployAttempt) bool {
	hr, ok := queuedRequest(a)
	return ok && hr.FreezeNote != ""
}

// computeWait is the pure core of waitFor: queued attempts of one app are a
// FIFO line, the head waits on a freeze, the running deploy or capacity.
func computeWait(a store.DeployAttempt, app []store.DeployAttempt, frozen bool, frozenUntil string, capacityFull bool) waitState {
	var line []store.DeployAttempt
	running := ""
	for _, o := range app {
		switch o.Status {
		case store.DeployAttemptStatusQueued:
			line = append(line, o)
		case store.DeployAttemptStatusRunning:
			running = o.ID
		}
	}
	sortQueued(line)
	pos := 0
	for i, o := range line {
		if o.ID == a.ID {
			pos = i + 1
		}
	}
	w := waitState{Position: pos}
	switch {
	case pos > 1:
		w.BlockedBy = line[pos-2].ID
		w.Reason = "waiting for #" + w.BlockedBy
	case frozen && frozenUntil != "":
		w.Reason = "freeze window until " + frozenUntil
	case frozen:
		w.Reason = "freeze window"
	case running != "":
		w.BlockedBy = running
		w.Reason = "waiting for #" + running
	case capacityFull:
		w.Reason = "waiting for build capacity"
	default:
		w.Reason = "starting"
	}
	return w
}

func sortQueued(line []store.DeployAttempt) {
	at := func(a store.DeployAttempt) time.Time {
		if a.QueuedAt != nil {
			return *a.QueuedAt
		}
		return a.StartedAt
	}
	sort.SliceStable(line, func(i, j int) bool { return at(line[i]).Before(at(line[j])) })
}

func heldWaitReason(reason string) string {
	rest := strings.TrimPrefix(reason, store.DeployReasonFrozen)
	if rest == reason {
		return reason
	}
	return "freeze window" + rest
}

// deploymentWaits computes the wait state of every queued or held deployment
// in rows with two queries, however many apps they span.
func (rt *Router) deploymentWaits(ctx context.Context, rows []store.Deployment) map[string]waitState {
	if rt.deploySafety == nil {
		return nil
	}
	need := false
	for _, d := range rows {
		if s := d.Attempt.Status; s == store.DeployAttemptStatusQueued || s == store.DeployAttemptStatusHeld {
			need = true
			break
		}
	}
	if !need {
		return nil
	}
	byApp := map[string][]store.DeployAttempt{}
	if queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx); err == nil {
		for _, a := range queued {
			byApp[a.ServiceName] = append(byApp[a.ServiceName], a)
		}
	}
	if running, err := rt.deploySafety.ListRunningDeployAttempts(ctx); err == nil {
		for _, a := range running {
			byApp[a.ServiceName] = append(byApp[a.ServiceName], a)
		}
	}
	out := map[string]waitState{}
	for _, d := range rows {
		a := d.Attempt
		if a.Status == store.DeployAttemptStatusQueued || a.Status == store.DeployAttemptStatusHeld {
			out[a.ID] = rt.waitFor(ctx, a, byApp[a.ServiceName])
		}
	}
	return out
}

func applyDeploymentWait(res *deploymentResource, w waitState) {
	if w.Position > 0 {
		res.QueuePosition = &w.Position
	}
	res.WaitReason = strOrNil(w.Reason)
	res.BlockedBy = strOrNil(w.BlockedBy)
}
