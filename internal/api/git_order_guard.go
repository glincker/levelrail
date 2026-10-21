package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// staleGitDelivery says why a push for sha must not deploy because a newer
// commit was already deployed or queued for name, or "" when it may. Order is
// proven from the push's own parent link or the host's commit timestamps; when
// neither shows the delivery is older it proceeds (a force push, for one), and
// the deploy-time ordering guard still applies.
func (rt *Router) staleGitDelivery(ctx context.Context, name, sha, before string, commitAt time.Time) string {
	if sha == "" || rt.deploySafety == nil {
		return ""
	}
	for _, known := range rt.knownGitHeads(ctx, name) {
		if known.sha == "" || known.sha == sha {
			continue
		}
		o := store.DeployOrder{Automatic: true, CommitSHA: sha, Before: before, CommitAt: commitAt}
		c := store.DeployCursor{CommitSHA: known.sha, Before: known.before, CommitAt: known.at}
		if err := deploy.CheckOrder(c, o); err != nil {
			return err.Error()
		}
	}
	return ""
}

type knownGitHead struct {
	sha    string
	before string
	at     time.Time
}

// knownGitHeads lists the commits already applied, running or queued for name:
// the ones a late delivery must not overwrite.
func (rt *Router) knownGitHeads(ctx context.Context, name string) []knownGitHead {
	var out []knownGitHead
	if rt.deployAttempts == nil {
		return out
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, name)
	if err != nil {
		rt.logger.Warn("api: git order guard: list attempts failed", slog.String("name", name), slog.String("error", err.Error()))
		return out
	}
	appliedSeen := false
	if cur, ok := rt.deploySafety.(interface {
		GetDeployCursor(ctx context.Context, serviceName string) (store.DeployCursor, error)
	}); ok {
		if c, err := cur.GetDeployCursor(ctx, name); err == nil && c.CommitSHA != "" {
			out = append(out, knownGitHead{sha: c.CommitSHA, before: c.Before, at: c.CommitAt})
			appliedSeen = true
		}
	}
	for _, a := range attempts {
		switch a.Status {
		case store.DeployAttemptStatusQueued, store.DeployAttemptStatusRunning:
			h := knownGitHead{sha: a.CommitSHA}
			if hr, ok := queuedRequest(a); ok {
				h.before, h.at = hr.Before, hr.CommitAt
			}
			out = append(out, h)
		case store.DeployAttemptStatusSucceeded:
			if appliedSeen {
				continue
			}
			appliedSeen = true
			out = append(out, knownGitHead{sha: a.CommitSHA})
		}
	}
	return out
}

func staleDeliveryMessage(sha, reason string) string {
	return fmt.Sprintf("ignored: commit %s is older than what is already deployed or queued (%s)\n", webhookShortSHA(sha), reason)
}

// noteGitHead remembers the newest head a webhook delivered, so the poller
// knows a webhook arrived and does not queue the same commit again.
func (rt *Router) noteGitHead(ctx context.Context, name, head string) {
	if rt.gitRel == nil || head == "" || isWebhookReplay(ctx) {
		return
	}
	if err := rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), true); err != nil {
		rt.logger.Warn("api: git push webhook: remember head failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// admitGitPush decides what a trigger-matching push becomes. handled is true
// when it was ignored (already deployed, in flight or older than what is
// deployed), held for a freeze, or queued; false means the caller falls back
// to the inline path of a control plane without deploy safety. The check and
// the enqueue run under one lock so a webhook and the poller never both queue a commit.
func (rt *Router) admitGitPush(ctx context.Context, name string, ev webhook.PushEvent) (int, string, bool) {
	rt.gitEnqueueMu.Lock()
	defer rt.gitEnqueueMu.Unlock()
	if !isWebhookReplay(ctx) {
		switch rt.commitDeployState(ctx, name, ev.After) {
		case "deployed":
			return http.StatusOK, fmt.Sprintf("ignored: commit %s is already deployed\n", webhookShortSHA(ev.After)), true
		case "in flight":
			return http.StatusOK, fmt.Sprintf("ignored: commit %s is already in flight\n", webhookShortSHA(ev.After)), true
		}
		if reason := rt.staleGitDelivery(ctx, name, ev.After, ev.Before, ev.HeadCommitAt); reason != "" {
			rt.logger.Info("api: git push webhook: delivery older than deployed commit", slog.String("name", name), slog.String("commit", webhookShortSHA(ev.After)), slog.String("reason", reason))
			return http.StatusOK, staleDeliveryMessage(ev.After, reason), true
		}
	}
	hr := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: ev.After, CommitLabel: ev.After, CommitSHA: ev.After, Before: ev.Before, CommitAt: ev.HeadCommitAt}
	if held, msg := rt.holdIfFrozen(ctx, name, hr, name+":"+ev.After); held {
		return http.StatusAccepted, msg, true
	}
	if rt.builder != nil && rt.deploySafety != nil {
		hr.Branch = strings.TrimPrefix(ev.Ref, "refs/heads/")
		if status, msg, ok := rt.enqueueGitPushDeploy(ctx, name, hr); ok {
			return status, msg, true
		}
	}
	return 0, "", false
}
