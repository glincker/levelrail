package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

// protectedEnvironmentOf returns the protected environment a service lives in.
// found is false for an unprotected or untagged service.
func (rt *Router) protectedEnvironmentOf(ctx context.Context, name string) (env store.Environment, found bool, err error) {
	if rt.apps == nil || rt.environments == nil {
		return store.Environment{}, false, nil
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil || svc == nil || svc.EnvironmentID == "" {
		return store.Environment{}, false, nil
	}
	e, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return store.Environment{}, false, nil
	}
	if err != nil {
		return store.Environment{}, false, err
	}
	return e, e.Protected, nil
}

// holdIfProtected parks an automatic deploy for approval before any build
// starts. handled is false when the environment is not protected, so callers
// proceed exactly as before.
func (rt *Router) holdIfProtected(ctx context.Context, name string, hr deploy.HeldRequest, image, source string, actor ApprovalActor) (handled bool, status int, msg string) {
	env, protected, err := rt.protectedEnvironmentOf(ctx, name)
	if err != nil {
		rt.logger.Error("api: check environment protection failed", slog.String("name", name), slog.String("error", err.Error()))
		return true, http.StatusServiceUnavailable, "deferred: environment protection could not be checked, retry later\n"
	}
	if !protected {
		return false, 0, ""
	}
	rt.supersedeHeldDeploys(ctx, name)
	id, err := rt.HoldForApproval(ctx, env, name, image, source, hr, actor)
	if err != nil {
		rt.logger.Error("api: hold protected deploy failed", slog.String("name", name), slog.String("error", err.Error()))
		return true, http.StatusServiceUnavailable, "deferred: the deploy could not be parked for approval, retry later\n"
	}
	return true, http.StatusAccepted, fmt.Sprintf("held: environment %q is protected, the deploy awaits approval %s\n", env.Name, id)
}

// supersedeHeldDeploys expires pending replay approvals for name so only the
// newest hold stays decidable.
func (rt *Router) supersedeHeldDeploys(ctx context.Context, name string) {
	if rt.deployApprovals == nil {
		return
	}
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, name)
	if err != nil {
		rt.logger.Warn("api: list pending approvals failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	now := store.FormatAuditTime(time.Now())
	for _, a := range pending {
		if a.Action != store.DeployApprovalActionReplay || a.HeldAttemptID == "" {
			continue
		}
		ok, err := rt.deployApprovals.DecideDeployApproval(ctx, a.ID, store.DeployApprovalStatusExpired, "", "", "", "Superseded", now)
		if err != nil || !ok {
			continue
		}
		rt.failAwaitingAttempt(ctx, a, "Superseded")
	}
}

// gitPushProtectedReason refuses a push to a protected environment: the
// pushed commit lives only in the push mirror, which an approval replay
// cannot rebuild, so the push must go through a reviewed deploy instead.
func (rt *Router) gitPushProtectedReason(ctx context.Context, name string) string {
	_, protected, err := rt.protectedEnvironmentOf(ctx, name)
	if err != nil {
		return "environment protection could not be checked, push again later"
	}
	if protected {
		return "this app is in a protected environment, deploy it through an approved deploy"
	}
	return ""
}
