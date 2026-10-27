package api

import (
	"context"
	"encoding/json"
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
	refused *refusalAuditWindow
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
			refused: newRefusalAuditWindow(envDurationOr(envPRCommentRefusalAuditWindow, defaultPRCommentRefusalAuditWindow),
				envIntDefault(envPRCommentRefusalAuditMaxKeys, defaultPRCommentRefusalAuditMaxKeys)),
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
	off := pc == nil || pc.cfg.Disabled || !gs.PRCommentCommands
	ignore := func(msg string) (int, string) {
		if off {
			return http.StatusOK, "ignored: pull request comment commands are off\n"
		}
		return http.StatusOK, msg
	}
	ev, err := webhook.ParseIssueCommentEvent(body)
	if err != nil {
		if off {
			return ignore("")
		}
		return http.StatusBadRequest, "malformed payload"
	}
	if ev.Action != "created" || !ev.IsPullRequest {
		return ignore("ignored: not a new pull request comment\n")
	}
	var claimed, first bool
	var claimErr error
	claim := func(c prcommand.Command) (bool, error) {
		if !claimed {
			claimed = true
			first, claimErr = rt.prCommentCommands.ClaimPRCommentCommand(ctx, name, ev.CommentID, ev.Commenter, string(c), ev.Number, pc.now(), pc.cfg.LogRetention)
		}
		return first, claimErr
	}
	// Refusals are audited only for a configured feature, once per comment id,
	// and through a per-login window so a comment loop cannot flood the log.
	refuse := func(c prcommand.Command, reason string) {
		if pc == nil {
			return
		}
		if ok, err := claim(c); err != nil || !ok {
			return
		}
		rt.auditPRRefusalWindowed(ctx, pc, name, ev, c, reason)
	}
	if len(ev.Body) > envIntDefault(envPRCommentMaxBodyBytes, defaultPRCommentMaxBodyBytes) {
		if strings.Contains(strings.ToLower(ev.Body), prcommand.Prefix) {
			refuse("", prRefusedBodyTooLarge)
		}
		return ignore("ignored: comment is too long\n")
	}
	cmd, parseErr := prcommand.Parse(ev.Body)
	if cmd == "" && parseErr == nil {
		return ignore("ignored: no command\n")
	}
	if off {
		refuse(cmd, prRefusedCommandsOff)
		return ignore("")
	}
	if strings.Contains(ev.Body, prCommentMarker) {
		return http.StatusOK, "ignored: comment is from a bot\n"
	}
	if ev.CommenterType == "Bot" || ev.SenderType == "Bot" || ev.PerformedViaApp || strings.HasSuffix(ev.Commenter, "[bot]") {
		refuse(cmd, prRefusedBot)
		return http.StatusOK, "ignored: comment is from a bot\n"
	}
	if !githubLoginPattern.MatchString(ev.Commenter) {
		refuse(cmd, prRefusedInvalidLogin)
		return http.StatusOK, "ignored: unusable commenter login\n"
	}
	inst, token, owner, repo, ok := rt.githubTargetFor(ctx, name, gs)
	if !ok || !strings.EqualFold(ev.RepoFullName, owner+"/"+repo) {
		rt.logger.Info("api: pr comment command ignored: repository does not match the connected one", slog.String("app_name", name))
		refuse(cmd, prRefusedRepoMismatch)
		return http.StatusOK, "ignored: repository does not match\n"
	}
	isFirst, err := claim(cmd)
	if err != nil {
		rt.logger.Error("api: pr comment command claim failed", slog.String("error", err.Error()), slog.String("app_name", name))
		return http.StatusInternalServerError, "internal error"
	}
	if !isFirst {
		return http.StatusOK, "ignored: comment already handled\n"
	}
	if ok, _ := pc.perUser.allow(strings.ToLower(ev.Commenter)); !ok {
		rt.logger.Warn("api: pr comment command rate limited", slog.String("app_name", name), slog.String("actor", ev.Commenter), slog.String("scope", "user"))
		rt.auditPRRefusalWindowed(ctx, pc, name, ev, cmd, prRefusedRateLimitedUser)
		return http.StatusOK, "ignored: rate limited\n"
	}
	if ok, _ := pc.perPR.allow(fmt.Sprintf("%s|%d", name, ev.Number)); !ok {
		rt.logger.Warn("api: pr comment command rate limited", slog.String("app_name", name), slog.Int("pr_number", ev.Number), slog.String("scope", "pr"))
		rt.auditPRRefusalWindowed(ctx, pc, name, ev, cmd, prRefusedRateLimitedPR)
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
	if ok, why := rt.commenterCanWrite(ctx, inst, token, owner, repo, ev.Commenter); !ok {
		log.Info("api: pr comment command refused: commenter lacks write access or the check failed", slog.String("reason", why))
		rt.auditPRRefusalWindowed(ctx, rt.prCommands, name, ev, cmd, why)
		return
	}
	refuse := func(reason, note string) { rt.auditPRCommandRefused(ctx, name, ev, cmd, reason, note, 0) }
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
		refuse(prRefusedMultiple, "")
		reply("Send one `/levelrail` command per comment. Nothing was run.")
		return
	}
	// Help is answered only here, after the write check, so outsiders learn nothing.
	if cmd == prcommand.Help {
		rt.recordPRCommandEvent(ctx, name, ev, cmd)
		reply(prCommandHelpText())
		return
	}
	pr, err := rt.githubAppClient.GetPullRequest(ctx, inst, token, owner, repo, ev.Number)
	if err != nil {
		log.Warn("api: pr comment command: load pull request failed", slog.String("error", err.Error()))
		refuse(prRefusedPRLookup, "")
		return
	}
	if pr.State != "open" || pr.Merged {
		refuse(prRefusedPRNotOpen, "")
		reply("This pull request is not open. Nothing was run.")
		return
	}
	if !strings.EqualFold(pr.BaseRepoFullName, owner+"/"+repo) {
		log.Warn("api: pr comment command: pull request base is not the connected repository")
		refuse(prRefusedRepoMismatch, "")
		return
	}
	if !strings.EqualFold(pr.HeadRepoFullName, pr.BaseRepoFullName) && !gs.PRCommentCommandsForks {
		refuse(prRefusedFork, "")
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
	if strings.HasPrefix(text, prNotRun) {
		refuse(prRefusedBlocked, strings.TrimPrefix(text, prNotRun))
	}
	rt.recordPRCommandEvent(ctx, name, ev, cmd)
	reply(fmt.Sprintf("%s (requested by %s)", text, codeSpan(ev.Commenter)))
}

// commenterCanWrite asks GitHub for the commenter's permission and fails
// closed on any error. Only a positive answer is cached, and only briefly.
func (rt *Router) commenterCanWrite(ctx context.Context, inst, token, owner, repo, login string) (bool, string) {
	pc := rt.prCommands
	key := strings.ToLower(owner + "/" + repo + "/" + login)
	now := pc.now()
	pc.permMu.Lock()
	if e, ok := pc.perm[key]; ok && now.Before(e.until) {
		pc.permMu.Unlock()
		return e.allowed, ""
	}
	pc.permMu.Unlock()

	perm, err := rt.githubAppClient.GetCollaboratorPermission(ctx, inst, token, owner, repo, login)
	if err != nil {
		rt.logger.Warn("api: pr comment command: permission lookup failed, refusing", slog.String("error", err.Error()))
		return false, prRefusedPermissionLookup
	}
	allowed := perm == "admin" || perm == "write"
	if !allowed {
		return false, prRefusedNoWriteAccess
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
	return true, ""
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
		return fmt.Sprintf(prNotRun+"this pull request targets %s, not the deployed branch %s.", codeSpan(pr.BaseRef), codeSpan(branch))
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return prNotRun + "the app could not be loaded."
	}
	if why := rt.prCommandBlock(ctx, *existing, true); why != "" {
		return prNotRun + why + "."
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
		return fmt.Sprintf(prNotRun+"this pull request targets %s, not the deployed branch.", codeSpan(pr.BaseRef))
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return prNotRun + "the app could not be loaded."
	}
	if why := rt.prCommandBlock(ctx, *existing, false); why != "" {
		return prNotRun + why + "."
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		return prNotRun + "the deploy history could not be loaded."
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
		return prNotRun + "there is no earlier successful deploy to roll back to."
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
		return prNotRun + "preview environments are off for this app."
	}
	if rt.builder == nil {
		return prNotRun + "deploys are not configured on this control plane."
	}
	if pr.BaseRef != gitSourceBranch(gs) {
		return fmt.Sprintf(prNotRun+"previews build pull requests into %s only.", codeSpan(gitSourceBranch(gs)))
	}
	if rt.deploySafety != nil {
		st, err := deploy.CheckFreeze(ctx, rt.deploySafety, name, time.Now())
		if err != nil {
			return prNotRun + "the freeze check failed."
		}
		if st.Frozen {
			return prNotRun + "deploys are frozen" + reasonSuffix(st.Reason) + "."
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

func prCommandHelpText() string {
	parts := make([]string, 0, len(prcommand.Supported()))
	for _, c := range prcommand.Supported() {
		parts = append(parts, codeSpan(prcommand.Prefix+" "+string(c)))
	}
	return "Supported commands, one per comment: " + strings.Join(parts, ", ") + "."
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

// A command is one short line, so a long body is never worth scanning.
const (
	envPRCommentMaxBodyBytes     = "APP_PR_COMMENT_MAX_BODY_BYTES"
	defaultPRCommentMaxBodyBytes = 4096
)

// Refusal audits are windowed per app and login; the key cap bounds memory.
const (
	envPRCommentRefusalAuditWindow      = "APP_PR_COMMENT_REFUSAL_AUDIT_WINDOW"
	defaultPRCommentRefusalAuditWindow  = 10 * time.Minute
	envPRCommentRefusalAuditMaxKeys     = "APP_PR_COMMENT_REFUSAL_AUDIT_MAX_KEYS"
	defaultPRCommentRefusalAuditMaxKeys = 10000
)

// Refusal reasons are stable values that audit filters and runbooks rely on.
const (
	auditActionPRCommandRefused = "pr_comment_command.refused"
	prNotRun                    = "Not run: "

	prRefusedCommandsOff      = "commands_off"
	prRefusedBodyTooLarge     = "body_too_large"
	prRefusedBot              = "bot"
	prRefusedInvalidLogin     = "invalid_login"
	prRefusedRepoMismatch     = "repository_mismatch"
	prRefusedRateLimitedUser  = "rate_limited_user"
	prRefusedRateLimitedPR    = "rate_limited_pull_request"
	prRefusedNoWriteAccess    = "no_write_access"
	prRefusedPermissionLookup = "permission_lookup_failed"
	prRefusedMultiple         = "multiple_commands"
	prRefusedPRLookup         = "pull_request_lookup_failed"
	prRefusedPRNotOpen        = "pull_request_not_open"
	prRefusedFork             = "fork_commands_off"
	prRefusedBlocked          = "blocked"
)

type prCommandRefusalDetail struct {
	App     string `json:"app"`
	Login   string `json:"login"`
	Repo    string `json:"repo"`
	PR      int    `json:"pr_number"`
	Comment int64  `json:"comment_id"`
	Command string `json:"command,omitempty"`
	Reason  string `json:"reason"`
	Note    string `json:"note,omitempty"`
	// Suppressed counts refusals for the same app and login dropped since the previous row.
	Suppressed int `json:"suppressed,omitempty"`
}

// auditPRCommandRefused writes one audit row per refused command. The comment
// body is never stored, and every echoed field is reduced to plain characters.
func (rt *Router) auditPRCommandRefused(ctx context.Context, name string, ev webhook.IssueCommentEvent, cmd prcommand.Command, reason, note string, suppressed int) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	login := sanitizeCommentText(ev.Commenter, prCommentFieldMax)
	raw, err := json.Marshal(prCommandRefusalDetail{
		App: name, Login: login, Repo: sanitizeCommentText(ev.RepoFullName, prCommentFieldMax),
		PR: ev.Number, Comment: ev.CommentID, Command: string(cmd), Reason: reason,
		Note: sanitizeCommentText(note, prCommentFieldMax), Suppressed: suppressed,
	})
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSystem, ActorID: "pr-comment-command", ActorName: prCommentActorTmpl + login,
		Ability: AbilityDeploy, Method: http.MethodPost, StatusCode: http.StatusForbidden,
		Path:      "/api/v1/webhooks/github/" + name + "#pr-" + fmt.Sprint(ev.Number),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: auditClientKindSystem,
		Action: auditActionPRCommandRefused, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Error("api: pr comment command: audit refusal failed", slog.String("error", err.Error()), slog.String("app_name", name))
	}
}
