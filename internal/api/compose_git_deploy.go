package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// composeGitLocks serializes compose deploys per app, so two pushes never interleave save and prune.
var composeGitLocks sync.Map

func composeGitLock(name string) *sync.Mutex {
	m, _ := composeGitLocks.LoadOrStore(name, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// composeCursorStore keeps the applied commit for a compose app under its app name.
type composeCursorStore interface {
	GetDeployCursor(ctx context.Context, serviceName string) (store.DeployCursor, error)
	AdvanceDeployCursor(ctx context.Context, serviceName string, order store.DeployOrder) error
}

// deployComposeFromGitSource applies a queued run (adopted attempt) now under the app
// lock; any other caller enqueues, so the queue's per-app and global caps apply.
func (rt *Router) deployComposeFromGitSource(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder, source string) (int, string) {
	attemptID, adopted := adoptedAttemptID(ctx)
	if !adopted && rt.deploySafety != nil {
		hr := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: checkoutRef, CommitLabel: commitLabel, CommitSHA: commitLabel, Branch: gs.Branch}
		if order != nil {
			hr.Before, hr.CommitAt = order.Before, order.CommitAt
		}
		if status, msg, ok := rt.enqueueGitPushDeploy(ctx, name, hr); ok {
			return status, msg
		}
		return http.StatusInternalServerError, "internal error"
	}
	if status, msg, refused := rt.refuseComposeReplay(ctx, name); refused {
		return status, msg
	}
	if !adopted {
		id, ok := rt.saveComposeAttempt(ctx, name, commitLabel, source)
		if !ok {
			return http.StatusInternalServerError, "internal error"
		}
		attemptID = id
	}
	status, msg, stale := rt.runComposeGitDeployLocked(ctx, name, gs, checkoutRef, commitLabel, order)
	switch {
	case stale != nil:
		rt.finishSuperseded(ctx, attemptID, stale)
		if !adopted {
			rt.finishComposeAttempt(ctx, attemptID, store.DeployAttemptStatusSuperseded, stale.Error())
		}
		return http.StatusOK, "not deployed: " + stale.Error() + "\n"
	case status >= http.StatusBadRequest:
		if !adopted {
			rt.finishComposeAttempt(ctx, attemptID, store.DeployAttemptStatusFailed, msg)
		}
		return status, msg + "\n"
	}
	if !adopted {
		rt.finishComposeAttempt(ctx, attemptID, store.DeployAttemptStatusSucceeded, "")
	}
	if rt.auditLog != nil {
		rt.auditSystemEvent(ctx, "git:"+webhookShortSHA(commitLabel), "compose.git_deploy", fmt.Sprintf("%s commit %s file %s", name, commitLabel, gs.ComposePath))
	}
	return http.StatusOK, fmt.Sprintf("compose deploy applied: %s at %s, %s\n", gs.ComposePath, webhookShortSHA(commitLabel), msg)
}

func (rt *Router) saveComposeAttempt(ctx context.Context, name, commit, source string) (string, bool) {
	if rt.deployAttempts == nil {
		return "", true
	}
	id, err := store.NewDeployAttemptID()
	if err == nil {
		err = rt.deployAttempts.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: id, ServiceName: name, Image: name + ":" + commit, CommitSHA: commit, Source: source,
			Status: store.DeployAttemptStatusRunning, StartedAt: time.Now().UTC(),
		})
	}
	if err != nil {
		rt.logger.Error("api: compose git source: save attempt failed", slog.String("error", err.Error()), slog.String("name", name))
		return "", false
	}
	return id, true
}

func (rt *Router) finishComposeAttempt(ctx context.Context, id, status, msg string) {
	if rt.deployAttempts == nil || id == "" {
		return
	}
	if err := rt.deployAttempts.FinishDeployAttempt(context.WithoutCancel(ctx), id, status, time.Now().UTC(), msg); err != nil {
		rt.logger.Warn("api: compose git source: finish attempt failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
	}
}

// runComposeGitDeployLocked bounds the apply by the live timeout and holds the app lock,
// so manual compose writers and other pushes never interleave save and prune.
func (rt *Router) runComposeGitDeployLocked(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder) (status int, msg string, stale error) {
	timeout := rt.runtimeSettings.Duration(ctx, runtimesettings.KeyComposeRepoDeployTimeout)
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	mu := composeGitLock(name)
	mu.Lock()
	defer mu.Unlock()
	defer func() {
		if p := recover(); p != nil {
			rt.logger.Error("api: compose git source: deploy panicked", slog.String("name", name), slog.Any("panic", p))
			status, msg, stale = http.StatusInternalServerError, "deploy panicked", nil
		}
	}()
	return rt.applyComposeGitDeploy(dctx, name, gs, checkoutRef, commitLabel, order)
}

// maxCommitClockSkew is how far ahead of the server clock a commit time may be and
// still order deploys; a later one is a forged or broken clock and is ignored.
const maxCommitClockSkew = 24 * time.Hour

func plausibleCommitAt(at time.Time) time.Time {
	if at.After(time.Now().Add(maxCommitClockSkew)) {
		return time.Time{}
	}
	return at
}

// supersedeComposeQueued supersedes queued deploys of name proven older than hr. When a
// queued one is proven newer, the new attempt is superseded instead and that id returned.
// Unknown order keeps both; the run-time order check decides. Caller holds buildStartMu.
func (rt *Router) supersedeComposeQueued(ctx context.Context, name, newID string, hr deploy.HeldRequest) string {
	queued, err := rt.deploySafety.ListQueuedDeployAttempts(ctx)
	if err != nil {
		rt.logger.Warn("api: compose git source: list queued failed", slog.String("error", err.Error()), slog.String("name", name))
		return ""
	}
	newAt := plausibleCommitAt(hr.CommitAt)
	newOrder := store.DeployOrder{Automatic: true, CommitSHA: hr.CommitSHA, Before: hr.Before, CommitAt: newAt}
	newCursor := store.DeployCursor{CommitSHA: hr.CommitSHA, Before: hr.Before, CommitAt: newAt}
	var older []string
	for _, old := range queued {
		if old.ID == newID || old.ServiceName != name {
			continue
		}
		oldHR, ok := queuedRequest(old)
		if !ok || oldHR.CommitSHA == hr.CommitSHA {
			continue
		}
		oldAt := plausibleCommitAt(oldHR.CommitAt)
		oldCursor := store.DeployCursor{CommitSHA: oldHR.CommitSHA, Before: oldHR.Before, CommitAt: oldAt}
		if deploy.CheckOrder(oldCursor, newOrder) != nil {
			if _, err := rt.deploySafety.SupersedeQueuedDeployAttempt(ctx, newID, old.ID, time.Now()); err != nil {
				rt.logger.Warn("api: compose git source: supersede new attempt failed", slog.String("attempt_id", newID), slog.String("error", err.Error()))
			}
			return old.ID
		}
		oldOrder := store.DeployOrder{Automatic: true, CommitSHA: oldHR.CommitSHA, Before: oldHR.Before, CommitAt: oldAt}
		if deploy.CheckOrder(newCursor, oldOrder) != nil {
			older = append(older, old.ID)
		}
	}
	for _, id := range older {
		if _, err := rt.deploySafety.SupersedeQueuedDeployAttempt(ctx, id, newID, time.Now()); err != nil {
			rt.logger.Warn("api: compose git source: supersede failed", slog.String("attempt_id", id), slog.String("error", err.Error()))
		}
	}
	return ""
}

// lockedComposeDeploy is deployComposeBody under the app's compose lock, for every
// writer that does not already hold it; it is the only lock taken on these paths.
func (rt *Router) lockedComposeDeploy(w http.ResponseWriter, r *http.Request, name string, body []byte, opts composeDeployOpts) (composeDeployResponse, bool) {
	mu := composeGitLock(name)
	mu.Lock()
	defer mu.Unlock()
	return rt.deployComposeBody(w, r, name, body, opts)
}

// applyComposeGitDeploy runs under the app lock: order check against the app cursor,
// fetch, the pasted-compose deploy path, then the cursor advance.
func (rt *Router) applyComposeGitDeploy(ctx context.Context, name string, gs store.GitSource, checkoutRef, commitLabel string, order *store.DeployOrder) (int, string, error) {
	cursors, _ := rt.deploySafety.(composeCursorStore)
	if cursors != nil && order != nil {
		if c, err := cursors.GetDeployCursor(ctx, name); err == nil {
			if err := deploy.CheckOrder(c, *order); err != nil {
				return http.StatusOK, err.Error(), err
			}
		}
	}
	body, status, msg := rt.fetchRepoCompose(ctx, name, gs, checkoutRef)
	if status != 0 {
		return status, msg, nil
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/apps/"+url.PathEscape(name)+"/compose", nil)
	rec := httptest.NewRecorder()
	resp, ok := rt.deployComposeBody(rec, req, name, body, composeDeployOpts{source: store.ComposeRevisionSourceGit, actor: "git:" + commitLabel, noCaller: true})
	if !ok {
		rt.logger.Warn("api: compose git source: deploy refused", slog.String("name", name), slog.Int("status", rec.Code))
		return rec.Code, "compose deploy refused: " + importErrorText(rec.Body.Bytes()), nil
	}
	if cursors != nil && order != nil {
		if err := cursors.AdvanceDeployCursor(ctx, name, *order); err != nil {
			rt.logger.Error("api: compose git source: advance cursor failed", slog.String("error", err.Error()), slog.String("name", name))
			return http.StatusInternalServerError, "deploy applied but its commit could not be recorded", nil
		}
	}
	rt.logger.Info("api: compose git source: deployed", slog.String("name", name), slog.String("commit", commitLabel), slog.Int("services", len(resp.Services)))
	return http.StatusOK, fmt.Sprintf("%d service(s)", len(resp.Services)), nil
}

// refuseForeignServiceNames is authorizeServiceNames for a deploy with no caller:
// any existing app outside this stack is a 409, never an ownership lookup.
func (rt *Router) refuseForeignServiceNames(ctx context.Context, w http.ResponseWriter, appName string, previous []store.DesiredService, names []string) bool {
	member := map[string]bool{appName: true}
	for _, p := range previous {
		member[p.Name] = true
	}
	for _, n := range names {
		if member[n] {
			continue
		}
		_, err := rt.apps.GetDesiredService(ctx, n)
		if errors.Is(err, store.ErrServiceNotFound) {
			continue
		}
		if err != nil {
			rt.internalError(w, "api: compose git source: check existing service", err)
			return false
		}
		writeError(w, http.StatusConflict, "an app named "+n+" already exists")
		return false
	}
	return true
}

// composeProtectedEnvironment reports a compose app protected when any member's environment is.
func (rt *Router) composeProtectedEnvironment(ctx context.Context, appID string) (store.Environment, bool, error) {
	members, err := rt.composeMembers(ctx, appID)
	if errors.Is(err, store.ErrServiceNotFound) {
		return store.Environment{}, false, nil
	}
	if err != nil {
		return store.Environment{}, false, fmt.Errorf("list compose members: %w", err)
	}
	var first store.Environment
	for _, m := range members {
		if m.EnvironmentID == "" {
			continue
		}
		e, err := rt.environments.GetEnvironment(ctx, m.EnvironmentID)
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			continue
		}
		if err != nil {
			return store.Environment{}, false, err
		}
		if e.Protected {
			return e, true, nil
		}
		if first.ID == "" {
			first = e
		}
	}
	return first, false, nil
}

// refuseComposeReplay stops a released, approved or queued run from recreating a
// removed stack or applying during a freeze (app, global or any member) without an override.
func (rt *Router) refuseComposeReplay(ctx context.Context, name string) (int, string, bool) {
	if _, err := rt.composeMembers(ctx, name); errors.Is(err, store.ErrServiceNotFound) {
		return http.StatusGone, "not deployed: the stack was removed; deploy it again before pushes apply\n", true
	}
	if rt.deploySafety == nil || freezeOverrideNoteFrom(ctx) != "" {
		return 0, "", false
	}
	status, err := deploy.CheckFreeze(ctx, rt.deploySafety, name, time.Now())
	if err != nil {
		status = deploy.FreezeStatus{Frozen: true, Reason: "freeze check unavailable"}
	}
	if !status.Frozen {
		status = rt.composeMemberFreeze(ctx, name, status)
	}
	if status.Frozen {
		return http.StatusLocked, "not deployed: " + (&deploy.FrozenError{Status: status}).Error() + "\n", true
	}
	return 0, "", false
}

// composeMemberFreeze holds a compose app deploy when any member is frozen. It fails
// closed only for a compose app; a single-service app keeps its own freeze status.
func (rt *Router) composeMemberFreeze(ctx context.Context, appID string, status deploy.FreezeStatus) deploy.FreezeStatus {
	members, err := rt.composeMembers(ctx, appID)
	if errors.Is(err, store.ErrServiceNotFound) {
		return status
	}
	if err != nil {
		if !rt.isComposeSource(ctx, appID) {
			rt.logger.Warn("api: freeze: list app members failed", slog.String("name", appID), slog.String("error", err.Error()))
			return status
		}
		rt.logger.Error("api: compose git source: list members for freeze failed", slog.String("name", appID), slog.String("error", err.Error()))
		return deploy.FreezeStatus{Frozen: true, Reason: "freeze check unavailable"}
	}
	for _, m := range members {
		s, err := deploy.CheckFreeze(ctx, rt.deploySafety, m.Name, time.Now())
		if err != nil {
			rt.logger.Error("api: compose git source: member freeze check failed", slog.String("service", m.Name), slog.String("error", err.Error()))
			return deploy.FreezeStatus{Frozen: true, Reason: "freeze check unavailable"}
		}
		if s.Frozen {
			return s
		}
	}
	return status
}

// isComposeSource reports whether name has a compose git source; an unreadable
// source counts as compose, so an unclear case holds rather than skips a freeze.
func (rt *Router) isComposeSource(ctx context.Context, name string) bool {
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return false
	}
	return err != nil || gs.ComposePath != ""
}

// cancelWaitingComposeAttempts cancels queued, held and approval-waiting deploys of a removed stack.
func (rt *Router) cancelWaitingComposeAttempts(ctx context.Context, appID string) {
	if rt.deploySafety == nil || rt.deployAttempts == nil {
		return
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, appID)
	if err != nil {
		rt.logger.Warn("api: compose: down: list attempts failed", slog.String("app", appID), slog.String("error", err.Error()))
		return
	}
	for _, a := range attempts {
		switch a.Status {
		case store.DeployAttemptStatusQueued, store.DeployAttemptStatusHeld, store.DeployAttemptStatusAwaitingApproval:
			if _, err := rt.deploySafety.TransitionDeployAttempt(ctx, a.ID, a.Status, store.DeployAttemptStatusCanceled, "the stack was removed"); err != nil {
				rt.logger.Warn("api: compose: down: cancel attempt failed", slog.String("attempt_id", a.ID), slog.String("error", err.Error()))
			}
		}
	}
}
