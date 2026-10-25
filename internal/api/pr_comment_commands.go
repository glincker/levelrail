package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/policy"
	"github.com/GLINCKER/levelrail/internal/prcommand"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// PRCommentCommandStore is the store surface for pull request comment commands.
// *store.DB satisfies it.
type PRCommentCommandStore interface {
	SetGitSourcePRCommentCommands(ctx context.Context, serviceName string, enabled bool) error
	SetGitSourcePRCommentCommandsForks(ctx context.Context, serviceName string, enabled bool) error
	ClaimPRCommentCommand(ctx context.Context, serviceName string, commentID int64, actor, command string, prNumber int, now time.Time, retain time.Duration) (bool, error)
}

// PRCommentCommandConfig is read from the environment by cmd/levelrail. Every
// number must be positive: a zero rate would mean "no limit", so a bad config
// turns the feature off instead.
type PRCommentCommandConfig struct {
	// Disabled is the env kill switch; it wins over every per-app setting.
	Disabled bool
	// PerPRPerMinute and PerUserPerMinute cap accepted commands.
	PerPRPerMinute   int
	PerUserPerMinute int
	// PermissionCacheTTL is how long a permission answer is reused.
	PermissionCacheTTL time.Duration
	// LogRetention is how long handled comment ids are remembered.
	LogRetention time.Duration
	// RunTimeout bounds one command's deploy work.
	RunTimeout time.Duration
}

type permEntry struct {
	allowed bool
	until   time.Time
}

type prCommentState struct {
	cfg     PRCommentCommandConfig
	perPR   *apiRateLimiter
	perUser *apiRateLimiter
	permMu  sync.Mutex
	perm    map[string]permEntry
	now     func() time.Time
	// run starts a command's work. Production detaches it so GitHub's delivery
	// timeout cannot cancel a deploy; tests run it inline.
	run func(func())
}

// WithPRCommentCommands enables /levelrail comment commands on this router.
// Without it, or with a non-positive limit, every comment is ignored.
func WithPRCommentCommands(cfg PRCommentCommandConfig) Option {
	return func(rt *Router) {
		if cfg.PerPRPerMinute <= 0 || cfg.PerUserPerMinute <= 0 || cfg.PermissionCacheTTL <= 0 || cfg.LogRetention <= 0 || cfg.RunTimeout <= 0 {
			return
		}
		rt.prCommands = &prCommentState{
			cfg: cfg, perPR: newAPIRateLimiter(cfg.PerPRPerMinute), perUser: newAPIRateLimiter(cfg.PerUserPerMinute),
			perm: make(map[string]permEntry), now: time.Now, run: func(f func()) { go f() },
		}
	}
}

const (
	prCommentMarker    = "<!-- levelrail-pr-command -->"
	prCommentReplyMax  = 600
	prCommentFieldMax  = 120
	appEventPRCommand  = "pr_comment_command"
	prCommentActorTmpl = "github:"
)

var githubLoginPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)

// sanitizeCommentText makes text safe to echo into a markdown comment: only
// plain word characters survive, so no mention, link, image, HTML or heading
// can be formed from it.
func sanitizeCommentText(s string, limit int) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == ',', r == '-', r == '/', r == ':':
			b.WriteRune(r)
		default:
			b.WriteByte(' ')
		}
	}
	out := strings.Join(strings.Fields(strings.ReplaceAll(b.String(), "://", " ")), " ")
	if len(out) > limit {
		out = strings.TrimSpace(out[:limit]) + "..."
	}
	return out
}

func codeSpan(s string) string { return "`" + sanitizeCommentText(s, prCommentFieldMax) + "`" }

// handleIssueCommentWebhook is called after the delivery signature has been
// verified. It never acts for a commenter it could not prove has write access.
func (rt *Router) handleIssueCommentWebhook(ctx context.Context, name string, gs store.GitSource, body []byte) (int, string) {
	pc := rt.prCommands
	if pc == nil || pc.cfg.Disabled || !gs.PRCommentCommands {
		return http.StatusOK, "ignored: pull request comment commands are off\n"
	}
	ev, err := webhook.ParseIssueCommentEvent(body)
	if err != nil {
		return http.StatusBadRequest, "malformed payload"
	}
	if ev.Action != "created" || !ev.IsPullRequest {
		return http.StatusOK, "ignored: not a new pull request comment\n"
	}
	cmd, parseErr := prcommand.Parse(ev.Body)
	if cmd == "" && parseErr == nil {
		return http.StatusOK, "ignored: no command\n"
	}
	if ev.CommenterType == "Bot" || ev.SenderType == "Bot" || ev.PerformedViaApp || strings.HasSuffix(ev.Commenter, "[bot]") || strings.Contains(ev.Body, prCommentMarker) {
		return http.StatusOK, "ignored: comment is from a bot\n"
	}
	if !githubLoginPattern.MatchString(ev.Commenter) {
		return http.StatusOK, "ignored: unusable commenter login\n"
	}
	inst, token, owner, repo, ok := rt.githubTargetFor(ctx, name, gs)
	if !ok || !strings.EqualFold(ev.RepoFullName, owner+"/"+repo) {
		rt.logger.Info("api: pr comment command ignored: repository does not match the connected one", slog.String("app_name", name))
		return http.StatusOK, "ignored: repository does not match\n"
	}
	now := pc.now()
	first, err := rt.prCommentCommands.ClaimPRCommentCommand(ctx, name, ev.CommentID, ev.Commenter, string(cmd), ev.Number, now, pc.cfg.LogRetention)
	if err != nil {
		rt.logger.Error("api: pr comment command claim failed", slog.String("error", err.Error()), slog.String("app_name", name))
		return http.StatusInternalServerError, "internal error"
	}
	if !first {
		return http.StatusOK, "ignored: comment already handled\n"
	}
	if ok, _ := pc.perUser.allow(strings.ToLower(ev.Commenter)); !ok {
		rt.logger.Warn("api: pr comment command rate limited", slog.String("app_name", name), slog.String("actor", ev.Commenter), slog.String("scope", "user"))
		return http.StatusOK, "ignored: rate limited\n"
	}
	if ok, _ := pc.perPR.allow(fmt.Sprintf("%s|%d", name, ev.Number)); !ok {
		rt.logger.Warn("api: pr comment command rate limited", slog.String("app_name", name), slog.Int("pr_number", ev.Number), slog.String("scope", "pr"))
		return http.StatusOK, "ignored: rate limited\n"
	}

	job := func() {
		jctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), pc.cfg.RunTimeout)
		defer cancel()
		rt.runPRCommentCommand(jctx, name, gs, ev, cmd, parseErr, inst, token, owner, repo)
	}
	pc.run(job)
	return http.StatusAccepted, "pull request command accepted\n"
}

// runPRCommentCommand checks the commenter and the pull request, runs the
// command, and posts one short reply. A commenter without write access gets
// no reply at all.
func (rt *Router) runPRCommentCommand(ctx context.Context, name string, gs store.GitSource, ev webhook.IssueCommentEvent, cmd prcommand.Command, parseErr error, inst, token, owner, repo string) {
	log := rt.logger.With(slog.String("app_name", name), slog.Int("pr_number", ev.Number), slog.String("actor", ev.Commenter))
	if !rt.commenterCanWrite(ctx, inst, token, owner, repo, ev.Commenter) {
		log.Info("api: pr comment command refused: commenter lacks write access or the check failed")
		return
	}
	reply := func(text string) {
		body := prCommentMarker + "\n" + text
		if len(body) > prCommentReplyMax+len(prCommentMarker) {
			body = body[:prCommentReplyMax+len(prCommentMarker)]
		}
		if _, err := rt.githubAppClient.CreateIssueComment(ctx, inst, token, owner, repo, ev.Number, body); err != nil {
			log.Warn("api: pr comment command reply failed", slog.String("error", err.Error()))
		}
	}
	if errors.Is(parseErr, prcommand.ErrMultiple) {
		reply("Send one `/levelrail` command per comment. Nothing was run.")
		return
	}
	pr, err := rt.githubAppClient.GetPullRequest(ctx, inst, token, owner, repo, ev.Number)
	if err != nil {
		log.Warn("api: pr comment command: load pull request failed", slog.String("error", err.Error()))
		return
	}
	if pr.State != "open" || pr.Merged {
		reply("This pull request is not open. Nothing was run.")
		return
	}
	if !strings.EqualFold(pr.BaseRepoFullName, owner+"/"+repo) {
		log.Warn("api: pr comment command: pull request base is not the connected repository")
		return
	}
	if !strings.EqualFold(pr.HeadRepoFullName, pr.BaseRepoFullName) && !gs.PRCommentCommandsForks {
		reply("Commands are off for pull requests from forks. Nothing was run.")
		return
	}

	var text string
	switch cmd {
	case prcommand.Status:
		text = rt.prCommandStatus(ctx, name, ev.Number)
	case prcommand.Redeploy:
		text = rt.prCommandRedeploy(ctx, name, gs, ev, pr)
	case prcommand.Rollback:
		text = rt.prCommandRollback(ctx, name, ev, pr, gs)
	case prcommand.PreviewRedeploy:
		text = rt.prCommandPreviewRedeploy(ctx, name, gs, ev, pr)
	default:
		return
	}
	rt.recordPRCommandEvent(ctx, name, ev, cmd)
	reply(fmt.Sprintf("%s (requested by %s)", text, codeSpan(ev.Commenter)))
}

// commenterCanWrite asks GitHub for the commenter's permission and fails
// closed on any error. Only a positive answer is cached, and only briefly.
func (rt *Router) commenterCanWrite(ctx context.Context, inst, token, owner, repo, login string) bool {
	pc := rt.prCommands
	key := strings.ToLower(owner + "/" + repo + "/" + login)
	now := pc.now()
	pc.permMu.Lock()
	if e, ok := pc.perm[key]; ok && now.Before(e.until) {
		pc.permMu.Unlock()
		return e.allowed
	}
	pc.permMu.Unlock()

	perm, err := rt.githubAppClient.GetCollaboratorPermission(ctx, inst, token, owner, repo, login)
	if err != nil {
		rt.logger.Warn("api: pr comment command: permission lookup failed, refusing", slog.String("error", err.Error()))
		return false
	}
	allowed := perm == "admin" || perm == "write"
	if !allowed {
		return false
	}
	pc.permMu.Lock()
	if len(pc.perm) >= defaultLimiterMaxKeys {
		for k, e := range pc.perm {
			if !now.Before(e.until) {
				delete(pc.perm, k)
			}
		}
		if len(pc.perm) >= defaultLimiterMaxKeys {
			clear(pc.perm)
		}
	}
	pc.perm[key] = permEntry{allowed: true, until: now.Add(pc.cfg.PermissionCacheTTL)}
	pc.permMu.Unlock()
	return true
}

func gitSourceBranch(gs store.GitSource) string {
	if gs.Branch == "" {
		return webhook.DefaultBranch
	}
	return gs.Branch
}

// prCommandBlock applies the same freeze, running-deploy, protected
// environment and (for deploys, not rollbacks) policy gates as the dashboard.
// It returns why the command must not run, or "".
func (rt *Router) prCommandBlock(ctx context.Context, svc store.DesiredService, withPolicy bool) string {
	if rt.deploySafety != nil {
		st, err := deploy.CheckFreeze(ctx, rt.deploySafety, svc.Name, time.Now())
		if err != nil {
			rt.logger.Error("api: pr comment command: freeze check failed", slog.String("error", err.Error()), slog.String("app_name", svc.Name))
			return "the freeze check failed, so nothing was run"
		}
		if st.Frozen {
			return "deploys are frozen" + reasonSuffix(st.Reason)
		}
	}
	if rt.hasRunningDeployAttempt(ctx, svc.Name) {
		return "a deploy for this app is already running"
	}
	if _, protected, err := rt.scheduledDeployEnvironment(ctx, svc); err != nil {
		return "the environment check failed, so nothing was run"
	} else if protected {
		return "this app's environment needs deploy approval, so request it from the dashboard"
	}
	if withPolicy {
		set, err := rt.effectivePolicy(ctx, svc.ProjectID, svc.EnvironmentID)
		if err != nil {
			return "the deploy policy could not be loaded, so nothing was run"
		}
		if blocking := policy.Blocking(policy.Evaluate(set, deployPolicySubject(svc))); len(blocking) > 0 {
			return "blocked by deploy policy" + reasonSuffix(blocking[0].Message)
		}
	}
	return ""
}

func reasonSuffix(reason string) string {
	if r := sanitizeCommentText(reason, prCommentFieldMax); r != "" {
		return ": " + codeSpan(r)
	}
	return ""
}

func (rt *Router) prCommandRedeploy(ctx context.Context, name string, gs store.GitSource, ev webhook.IssueCommentEvent, pr githubapp.PullRequest) string {
	branch := gitSourceBranch(gs)
	if pr.BaseRef != branch {
		return fmt.Sprintf("Not run: this pull request targets %s, not the deployed branch %s.", codeSpan(pr.BaseRef), codeSpan(branch))
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "Not run: the app could not be loaded."
	}
	if why := rt.prCommandBlock(ctx, *existing, true); why != "" {
		return "Not run: " + why + "."
	}
	ctx = context.WithValue(ctx, commitMetaKey{}, commitMeta{Branch: branch, Author: ev.Commenter, Message: fmt.Sprintf("Redeploy requested by PR comment on #%d", ev.Number)})
	if _, err := rt.triggerGitDeploy(ctx, name, branch, store.DeployAttemptSourcePRComment); err != nil {
		rt.logger.Error("api: pr comment redeploy failed", slog.String("error", err.Error()), slog.String("app_name", name))
		return "Redeploy failed. See the deploy history in the dashboard."
	}
	return "Redeploy of " + codeSpan(branch) + " finished."
}

func (rt *Router) prCommandRollback(ctx context.Context, name string, ev webhook.IssueCommentEvent, pr githubapp.PullRequest, gs store.GitSource) string {
	if pr.BaseRef != gitSourceBranch(gs) {
		return fmt.Sprintf("Not run: this pull request targets %s, not the deployed branch.", codeSpan(pr.BaseRef))
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "Not run: the app could not be loaded."
	}
	if why := rt.prCommandBlock(ctx, *existing, false); why != "" {
		return "Not run: " + why + "."
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return "Not run: the deploy history could not be loaded."
	}
	sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
	var target *store.DeployAttempt
	for i := range attempts {
		a := attempts[i]
		if a.Status == store.DeployAttemptStatusSucceeded && a.Image != "" && unpinnedRef(a.Image) != unpinnedRef(existing.Image) {
			target = &a
			break
		}
	}
	if target == nil {
		return "Not run: there is no earlier successful deploy to roll back to."
	}
	note := fmt.Sprintf("RollbackTo: %s. PR comment by %s on #%d", target.ID, ev.Commenter, ev.Number)
	if isStaticAttempt(*target) {
		if status, msg := rt.rollbackStatic(ctx, *target); status != 0 {
			return "Rollback failed: " + codeSpan(msg) + "."
		}
		return "Rolled back to deploy " + codeSpan(target.ID) + "."
	}
	pin := rt.rollbackPin(ctx, *target)
	ref, status, msg := rt.rollbackImageRef(ctx, *target, pin)
	if status != 0 {
		return "Rollback not possible: " + codeSpan(msg) + "."
	}
	restored := deploy.RestoreSnapshot(*existing, target.Snapshot)
	if deploy.LocalOnlyRef(pin, ref) {
		restored.PullPolicy = ""
	}
	if _, err := rt.executeConfirmedDeploy(ctx, restored, ref, confirmedDeployOptions{source: store.DeployAttemptSourcePRComment, reason: note}); err != nil {
		rt.logger.Error("api: pr comment rollback failed", slog.String("error", err.Error()), slog.String("app_name", name))
		return "Rollback failed. See the deploy history in the dashboard."
	}
	return "Rolled back to deploy " + codeSpan(target.ID) + "."
}

func (rt *Router) prCommandPreviewRedeploy(ctx context.Context, name string, gs store.GitSource, ev webhook.IssueCommentEvent, pr githubapp.PullRequest) string {
	if !gs.PreviewEnabled {
		return "Not run: preview environments are off for this app."
	}
	if rt.builder == nil {
		return "Not run: deploys are not configured on this control plane."
	}
	if pr.BaseRef != gitSourceBranch(gs) {
		return fmt.Sprintf("Not run: previews build pull requests into %s only.", codeSpan(gitSourceBranch(gs)))
	}
	if rt.deploySafety != nil {
		st, err := deploy.CheckFreeze(ctx, rt.deploySafety, name, time.Now())
		if err != nil {
			return "Not run: the freeze check failed."
		}
		if st.Frozen {
			return "Not run: deploys are frozen" + reasonSuffix(st.Reason) + "."
		}
	}
	prev := webhook.PullRequestEvent{
		Action: webhook.PullRequestSynchronize, Number: ev.Number, HeadRef: pr.HeadRef, HeadSHA: pr.HeadSHA, BaseRef: pr.BaseRef,
		HeadRepoFullName: pr.HeadRepoFullName, BaseRepoFullName: pr.BaseRepoFullName,
	}
	if status, _ := rt.deployPreviewEnvironment(ctx, name, gs, prev, false); status >= http.StatusBadRequest {
		return "Preview redeploy failed. See the preview in the dashboard."
	}
	if p, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, name, ev.Number); err == nil {
		return "Preview redeploy done, preview is " + codeSpan(string(p.Status)) + "."
	}
	return "Preview redeploy done."
}

func (rt *Router) prCommandStatus(ctx context.Context, name string, prNumber int) string {
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "Status unavailable: the app could not be loaded."
	}
	parts := []string{"Running image " + codeSpan(existing.Image)}
	if attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name); err == nil && len(attempts) > 0 {
		sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].StartedAt.After(attempts[j].StartedAt) })
		a := attempts[0]
		parts = append(parts, fmt.Sprintf("latest deploy %s via %s at %s", codeSpan(a.Status), codeSpan(a.Source), codeSpan(a.StartedAt.UTC().Format("2006-01-02 15.04 UTC"))))
	}
	if frozen, reason := rt.freezeStatus(ctx, name); frozen {
		parts = append(parts, "deploys are frozen"+reasonSuffix(reason))
	}
	if p, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, name, prNumber); err == nil {
		parts = append(parts, "preview "+codeSpan(string(p.Status)))
	}
	return strings.Join(parts, ", ") + "."
}

func (rt *Router) recordPRCommandEvent(ctx context.Context, name string, ev webhook.IssueCommentEvent, cmd prcommand.Command) {
	s := rt.appEvents()
	if s == nil {
		return
	}
	e := store.AppEvent{AppName: name, Kind: appEventPRCommand, Actor: prCommentActorTmpl + ev.Commenter,
		Title: "Pull request command: " + string(cmd), Detail: fmt.Sprintf("pull request #%d, comment %d", ev.Number, ev.CommentID)}
	if err := s.AddAppEvent(ctx, e); err != nil {
		rt.logger.Error("api: record pr comment command event failed", slog.String("error", err.Error()), slog.String("app_name", name))
	}
}
