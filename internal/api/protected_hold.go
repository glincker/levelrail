package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/storage/memory"

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

// heldRefPrefix namespaces the mirror refs that keep held pushes alive.
const heldRefPrefix = "refs/held/"

// pinBranchSHA resolves the commit a branch points at now, so a hold replays
// that commit even if the branch moves before approval.
func (rt *Router) pinBranchSHA(ctx context.Context, name, branch string) (string, error) {
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return "", errors.New("no git source connected")
	}
	token, err := rt.resolveGitSourceDeployToken(ctx, name)
	if err != nil {
		return "", fmt.Errorf("resolve deploy token: %w", err)
	}
	sha, err := rt.resolveBranchSHA(ctx, gs.RepoURL, branch, token)
	if err != nil {
		return "", fmt.Errorf("resolve latest commit: %w", err)
	}
	return sha, nil
}

// resolveTagSHAFunc is a seam so tests need no network.
type resolveTagSHAFunc func(ctx context.Context, repoURL, tag, token string) (string, error)

var resolveTagSHA resolveTagSHAFunc = resolveRemoteTagSHA

// resolveRemoteTagSHA returns the commit a tag points at, peeling annotated tags.
func resolveRemoteTagSHA(ctx context.Context, repoURL, tag, token string) (string, error) {
	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{repoURL}})
	opts := &git.ListOptions{PeelingOption: git.AppendPeeled}
	if token != "" {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	refs, err := remote.ListContext(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("list refs: %w", err)
	}
	want := "refs/tags/" + tag
	var plain string
	for _, ref := range refs {
		switch ref.Name().String() {
		case want + "^{}":
			return ref.Hash().String(), nil
		case want:
			plain = ref.Hash().String()
		}
	}
	if plain == "" {
		return "", fmt.Errorf("tag %q not found", tag)
	}
	return plain, nil
}

func (rt *Router) pinTagSHA(ctx context.Context, name, repoURL, tag string) (string, error) {
	token, err := rt.resolveGitSourceDeployToken(ctx, name)
	if err != nil {
		return "", fmt.Errorf("resolve deploy token: %w", err)
	}
	return resolveTagSHA(ctx, repoURL, tag, token)
}

// holdGitPush parks a validated push on a protected app. The pushed commit is
// kept under a held ref, so a later push or the quota reset cannot drop it
// before the hold is decided. handled is false for an unprotected app.
func (rt *Router) holdGitPush(ctx context.Context, name string, plan *gitPushPlan) (handled bool, msg string, parked bool) {
	_, protected, err := rt.protectedEnvironmentOf(ctx, name)
	if err != nil {
		rt.logger.Error("api: git push: check environment protection failed", slog.String("name", name), slog.String("error", err.Error()))
		return true, "Environment protection could not be checked. Push again later.", false
	}
	if !protected {
		return false, "", false
	}
	refID, err := store.NewDeployAttemptID()
	if err != nil {
		return true, "The push could not be parked for approval. Push again later.", false
	}
	hash := plan.cmd.New.String()
	heldRef := heldRefPrefix + refID
	if err := plan.repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(heldRef), plan.cmd.New)); err != nil {
		rt.logger.Error("api: git push: keep held commit failed", slog.String("name", name), slog.String("error", err.Error()))
		return true, "The push could not be parked for approval. Push again later.", false
	}
	hr := deploy.HeldRequest{
		Kind: deploy.HeldKindPush, CheckoutRef: hash, CommitLabel: hash, CommitSHA: hash,
		Branch: plan.branch, PushMirror: gitPushMirrorURL(name), HeldRef: heldRef,
	}
	handled, status, hmsg := rt.holdIfProtected(ctx, name, hr, name+":"+hash, store.DeployAttemptSourceGitPush, SystemApprovalActor)
	if status != http.StatusAccepted {
		_ = plan.repo.Storer.RemoveReference(plumbing.ReferenceName(heldRef))
	}
	if status != http.StatusAccepted {
		return handled, strings.TrimSpace(hmsg), false
	}
	return true, "Commit " + hash[:7] + " is held: " + strings.TrimSpace(hmsg), true
}

// replayGitPush builds a held push from the push mirror at the pushed commit.
func (rt *Router) replayGitPush(ctx context.Context, name string, req deploy.HeldRequest) error {
	defer rt.dropHeldRef(req)
	if rt.builder == nil {
		return errors.New("builds are not configured on this control plane")
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return fmt.Errorf("load app: %w", err)
	}
	repo, _, release, err := rt.sourceCache.OpenMirror(req.PushMirror)
	if err != nil {
		return fmt.Errorf("open push mirror: %w", err)
	}
	defer release()
	commit, err := repo.CommitObject(plumbing.NewHash(req.CommitSHA))
	if err != nil {
		return errors.New("the held commit is no longer stored, push again")
	}
	buildType, path, base := rt.gitPushBuildConfig(ctx, name, commit)
	treq := triggerBuildRequest{
		RepoURL: req.PushMirror, Ref: req.CommitSHA,
		Build: triggerBuildBuildInput{Type: buildType, Path: path, BaseDirectory: base},
	}
	buildReq := manualBuildRequest(name, *existing, treq, buildType)
	dctx := context.WithValue(ctx, commitMetaKey{}, commitMeta{Branch: req.Branch, Message: pushCommitSubject(commit.Message), Author: commit.Author.Name})
	run := rt.beginManualBuild(dctx, *existing, buildReq, buildType, store.DeployAttemptSourceGitPush, "", "", false)
	run.repoURL, run.ref, run.fromCache = req.PushMirror, req.CommitSHA, true
	var buildErr error
	finish := run.finish
	run.finish = func(err error) {
		finish(err)
		buildErr = err
		if err == nil {
			ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(req.Branch), plumbing.NewHash(req.CommitSHA))
			if serr := repo.Storer.SetReference(ref); serr != nil {
				rt.logger.Error("api: git push: advance ref failed", slog.String("name", name), slog.String("error", serr.Error()))
			}
		}
	}
	rt.runManualBuild(run)
	return buildErr
}

// dropHeldRef removes the ref keeping a held push alive once it is decided.
func (rt *Router) dropHeldRef(req deploy.HeldRequest) {
	if req.Kind != deploy.HeldKindPush || !strings.HasPrefix(req.HeldRef, heldRefPrefix) {
		return
	}
	repo, _, release, err := rt.sourceCache.OpenMirror(req.PushMirror)
	if err != nil {
		return
	}
	defer release()
	if err := repo.Storer.RemoveReference(plumbing.ReferenceName(req.HeldRef)); err != nil {
		rt.logger.Warn("api: remove held push ref failed", slog.String("error", err.Error()))
	}
}

// heldRequestOf reads the held request saved on an attempt.
func (rt *Router) heldRequestOf(ctx context.Context, attemptID string) (deploy.HeldRequest, bool) {
	var req deploy.HeldRequest
	if rt.deployAttempts == nil || attemptID == "" {
		return req, false
	}
	a, err := rt.deployAttempts.GetDeployAttempt(ctx, attemptID)
	if err != nil || a == nil || json.Unmarshal([]byte(a.HeldRequest), &req) != nil {
		return req, false
	}
	return req, true
}

// hasPendingPushHold reports whether a held push of name still needs the mirror.
func (rt *Router) hasPendingPushHold(ctx context.Context, name string) bool {
	if rt.deployApprovals == nil {
		return false
	}
	pending, err := rt.deployApprovals.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, name)
	if err != nil {
		return true
	}
	for _, a := range pending {
		if req, ok := rt.heldRequestOf(ctx, a.HeldAttemptID); ok && req.Kind == deploy.HeldKindPush {
			return true
		}
	}
	return false
}
