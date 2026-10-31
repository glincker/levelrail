package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gitPushResultMaxLen = 200

// pushReject is a push the remote refuses. A non-zero http answers with that
// status instead of a report, for failures that leave the client mid-upload.
type pushReject struct {
	reason string
	http   int
}

type gitPushPlan struct {
	repo    *git.Repository
	cmd     *packp.Command
	branch  string
	commit  *object.Commit
	release func()
}

// prepareGitPush runs every check that needs no output and stores the pushed
// objects. It never writes to the response: the body must be read first.
func (rt *Router) prepareGitPush(r *http.Request, ws *pushStream, name string, rm *store.GitPushRemote, req *packp.ReferenceUpdateRequest, body *bufio.Reader, cr *capReader, limit int64) (*gitPushPlan, *pushReject) {
	ctx := r.Context()
	reject := func(reason, hint string) *pushReject {
		if hint != "" {
			ws.say(hint)
		}
		_, _ = io.Copy(io.Discard, body)
		return &pushReject{reason: reason}
	}

	if len(req.Commands) != 1 {
		return nil, reject("push one branch at a time", "This remote deploys one branch per push. Push "+rm.Branch+" on its own.")
	}
	cmd := req.Commands[0]
	switch {
	case cmd.Action() == packp.Delete:
		return nil, reject("deleting branches is not allowed", "This remote only deploys. It never deletes anything.")
	case !cmd.Name.IsBranch():
		return nil, reject("only branches can be pushed", "Tags and other refs are not deployed. Push the branch "+rm.Branch+".")
	case cmd.Name.Short() != rm.Branch:
		return nil, reject("branch not allowed, this app deploys "+rm.Branch,
			fmt.Sprintf("Branch %q does not deploy: this app deploys %q. Push it with: git push <remote> %s:%s", cmd.Name.Short(), rm.Branch, cmd.Name.Short(), rm.Branch))
	}
	if msg := rt.gitPushBlockedReason(ctx, name); msg != "" {
		return nil, reject("deploys are paused", msg)
	}
	if rt.builder == nil {
		return nil, reject("builds are not configured", "Builds are not configured on this control plane.")
	}

	mirror := gitPushMirrorURL(name)
	if rt.sourceCache.MirrorSize(mirror) > gitPushLimits().quotaBytes {
		if rt.hasPendingPushHold(ctx, name) {
			return nil, reject("storage quota reached while a push awaits approval", "The server copy is over its size quota and holds a push awaiting approval. Decide that approval, then push again.")
		}
		if err := rt.sourceCache.ResetMirror(mirror); err != nil {
			rt.logger.Error("api: git push: reset over-quota mirror failed", slog.String("name", name), slog.String("error", err.Error()))
		}
		return nil, reject("storage quota reached, retry the push", "The server copy of this repository reached its size quota and was reset. Push again.")
	}
	repo, _, release, err := rt.sourceCache.OpenMirror(mirror)
	if err != nil {
		rt.logger.Error("api: git push: open mirror failed", slog.String("name", name), slog.String("error", err.Error()))
		return nil, reject("server storage is not configured", "git push needs the source cache directory, which is not configured.")
	}
	fail := func(reason, hint string) (*gitPushPlan, *pushReject) {
		release()
		return nil, reject(reason, hint)
	}

	if _, perr := body.Peek(1); perr == nil {
		if err := packfile.UpdateObjectStorage(repo.Storer, body); err != nil {
			release()
			if cr.exceeded {
				return nil, &pushReject{reason: tooLargeMessage(limit), http: http.StatusRequestEntityTooLarge}
			}
			rt.logger.Warn("api: git push: unpack failed", slog.String("name", name), slog.String("error", err.Error()))
			return nil, &pushReject{reason: "the pushed objects could not be read", http: http.StatusBadRequest}
		}
	} else if cr.exceeded {
		release()
		return nil, &pushReject{reason: tooLargeMessage(limit), http: http.StatusRequestEntityTooLarge}
	}

	commit, err := repo.CommitObject(cmd.New)
	if err != nil {
		return fail("the pushed commit is missing from the upload", "The upload did not contain the commit. Push again.")
	}
	if _, err := commit.Tree(); err != nil {
		return fail("the pushed commit is incomplete", "The upload is missing files of the commit. Push again.")
	}
	current := plumbing.ZeroHash
	if ref, err := repo.Storer.Reference(cmd.Name); err == nil {
		current = ref.Hash()
	}
	if current != cmd.Old {
		return fail("the server copy changed, push again", "The server copy changed while you pushed. Run git push again.")
	}
	return &gitPushPlan{repo: repo, cmd: cmd, branch: cmd.Name.Short(), commit: commit, release: release}, nil
}

func (rt *Router) gitPushBlockedReason(ctx context.Context, name string) string {
	if rt.deploySafety != nil {
		st, err := deploy.CheckFreeze(ctx, rt.deploySafety, name, time.Now())
		if err == nil && st.Frozen {
			return (&deploy.FrozenError{Status: st}).Error() + ". Push again when the freeze ends."
		}
	}
	if rt.deployAttempts != nil && rt.hasRunningDeployAttempt(ctx, name) {
		return "A deploy for this app is already running. Push again when it finishes."
	}
	return ""
}

func (rt *Router) gitPushBuildConfig(ctx context.Context, name string, c *object.Commit) (buildType, path, base string) {
	if rt.gitSources != nil {
		if gs, err := rt.gitSources.GetGitSource(ctx, name); err == nil && gs != nil {
			switch gs.BuildType {
			case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic:
				return gs.BuildType, gs.BuildPath, gs.BaseDirectory
			}
		}
	}
	if tree, err := c.Tree(); err == nil {
		if _, err := tree.File("Dockerfile"); err == nil {
			return spec.BuildDockerfile, "", ""
		}
	}
	return spec.BuildRailpack, "", ""
}

// deployGitPush builds and deploys the pushed commit through the same pipeline
// as a manual build and waits for the outcome, streaming the build output to
// the client. The branch ref advances only when the deploy succeeds, so a
// failed push can be retried as is.
func (rt *Router) deployGitPush(r *http.Request, ws *pushStream, name string, plan *gitPushPlan, timeout time.Duration) string {
	ctx := r.Context()
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.logger.Error("api: git push: load app failed", slog.String("name", name), slog.String("error", err.Error()))
		ws.say("The app could not be loaded.")
		return "app not found"
	}
	hash := plan.cmd.New.String()
	short := hash[:7]
	if handled, msg, parked := rt.holdGitPush(ctx, name, plan); handled {
		ws.say(msg)
		if !parked {
			return "the push could not be parked for approval"
		}
		return ""
	}
	buildType, path, base := rt.gitPushBuildConfig(ctx, name, plan.commit)
	treq := triggerBuildRequest{
		RepoURL: gitPushMirrorURL(name), Ref: hash,
		Build: triggerBuildBuildInput{Type: buildType, Path: path, BaseDirectory: base},
	}
	buildReq := manualBuildRequest(name, *existing, treq, buildType)

	dctx := context.WithValue(ctx, commitMetaKey{}, commitMeta{
		Branch:  plan.branch,
		Message: pushCommitSubject(plan.commit.Message),
		Author:  plan.commit.Author.Name,
	})
	rt.buildStartMu.Lock()
	if rt.deploySafety != nil && rt.deployBlocker(ctx, name) || rt.hasRunningDeployAttempt(ctx, name) {
		rt.buildStartMu.Unlock()
		ws.say("A deploy for this app is already running or queued. Push again when it finishes.")
		return "another deploy is running"
	}
	run := rt.beginManualBuild(dctx, *existing, buildReq, buildType, store.DeployAttemptSourceGitPush, "", "", false)
	rt.buildStartMu.Unlock()
	run.repoURL, run.ref, run.fromCache = gitPushMirrorURL(name), hash, true

	done := make(chan error, 1)
	finish, progress := run.finish, run.progress
	run.finish = func(err error) {
		finish(err)
		if err == nil {
			if serr := plan.repo.Storer.SetReference(plumbing.NewHashReference(plan.cmd.Name, plan.cmd.New)); serr != nil {
				rt.logger.Error("api: git push: advance ref failed", slog.String("name", name), slog.String("error", serr.Error()))
			}
		}
		done <- err
	}
	run.progress = func(ev build.ProgressEvent) {
		progress(ev)
		switch {
		case ev.Log != "":
			ws.say(ev.Log)
		case ev.Error != "":
			ws.say("error: " + ev.Error)
		}
	}
	progress(build.ProgressEvent{Log: fmt.Sprintf("git push: received commit %s on branch %s\n", short, plan.branch), Stream: "stdout"})
	ws.say(fmt.Sprintf("Received commit %s on %s, starting the build.", short, plan.branch))
	if run.id != "" {
		ws.say("Deploy " + run.id + " is also visible in the dashboard.")
	}
	go rt.runManualBuild(run) //nolint:gosec // the build outlives the request; runManualBuild derives its own context

	select {
	case err := <-done:
		if err != nil {
			ws.say("Deploy failed: " + err.Error())
			ws.say("The previous release is still running. Fix the build and push again.")
			return "deploy failed, the previous release is still running"
		}
		ws.say("Deploy succeeded: " + short + " is rolling out.")
		return ""
	case <-time.After(timeout):
		ws.say("Still building after " + timeout.String() + ". The deploy continues, follow it in the dashboard.")
		return "timed out waiting for the deploy, it may still finish"
	case <-ctx.Done():
		return "client disconnected"
	}
}

func pushCommitSubject(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// recordGitPushOutcome stores the last push result and writes an audit entry.
func (rt *Router) recordGitPushOutcome(r *http.Request, name string, cmd *packp.Command, reason string) {
	result := "ok"
	status := http.StatusOK
	if reason != "" {
		result = reason
		status = http.StatusUnprocessableEntity
	}
	if len(result) > gitPushResultMaxLen {
		result = result[:gitPushResultMaxLen]
	}
	ctx := context.WithoutCancel(r.Context())
	if err := rt.gitPushStore.RecordGitPush(ctx, name, cmd.New.String(), result, time.Now()); err != nil {
		rt.logger.Warn("api: git push: record outcome failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "git_remote", ActorID: name, ActorName: "git push token",
		Ability: AbilityDeploy, Method: r.Method, Path: r.URL.Path, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: "git_push",
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: git push: save audit entry failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}
