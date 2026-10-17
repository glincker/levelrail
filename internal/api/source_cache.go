package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envGitReplayWindow     = "APP_GIT_REPLAY_WINDOW"
	defaultGitReplayWindow = 24 * time.Hour
	gitReplayLookback      = 5
)

// errGitCommitNotFound marks a checkout of a SHA the repository does not have.
var errGitCommitNotFound = sourcecache.ErrCommitNotFound

// WithSourceCache sets the mirror every git fetch goes through. Without it
// each fetch is a throwaway clone with the same timeouts and retries.
func WithSourceCache(c *sourcecache.Cache) Option {
	return func(rt *Router) { rt.sourceCache = c }
}

// SourceCache is the mirror every fetch of this router goes through.
func (rt *Router) SourceCache() *sourcecache.Cache { return rt.sourceCache }

// gitCheckout is the fetchFunc: it resolves ref (branch, tag or commit) and
// returns the commit it resolved to along with the checked out tree.
func (rt *Router) gitCheckout(ctx context.Context, repoURL, ref, token string) (dir, commit string, cleanup func(), err error) {
	co, err := rt.sourceCache.Checkout(ctx, repoURL, ref, token)
	if err != nil {
		return "", "", nil, err
	}
	return co.Dir, co.Commit, co.Cleanup, nil
}

// gitCheckoutWithToken is the gitSourceFetchFunc for push, release and
// preview deploys.
func (rt *Router) gitCheckoutWithToken(ctx context.Context, repoURL, checkoutRef, token string) (dir string, cleanup func(), err error) {
	co, err := rt.sourceCache.Checkout(ctx, repoURL, checkoutRef, token)
	if err != nil {
		return "", nil, err
	}
	return co.Dir, co.Cleanup, nil
}

// noteGitHostBlocked remembers that name could not deploy because its git
// host is down, so the push is retried when the host answers.
func (rt *Router) noteGitHostBlocked(repoURL, name string, err error) {
	if errors.Is(err, sourcecache.ErrHostUnreachable) {
		rt.sourceCache.Block(repoURL, name)
	}
}

// RunGitHostWatch re-probes failing git hosts until ctx ends and replays the
// newest push that was waiting on each app once its host answers.
func (rt *Router) RunGitHostWatch(ctx context.Context) {
	rt.sourceCache.Watch(ctx, sourcecache.ProbeInterval(), rt.replayBlockedPushes)
}

func (rt *Router) replayBlockedPushes(ctx context.Context, host string, apps []string) {
	window := envDuration(envGitReplayWindow, defaultGitReplayWindow)
	for _, name := range apps {
		if err := rt.replayNewestBlockedDelivery(ctx, name, window); err != nil {
			rt.logger.Warn("api: replay after git host recovery failed", slog.String("name", name), slog.String("host", host), slog.String("error", err.Error()))
		}
	}
}

// replayNewestBlockedDelivery replays name's newest webhook delivery when it
// is the one that failed on an unreachable host and is still recent. An older
// blocked push is superseded by any newer delivery, so only the newest counts.
func (rt *Router) replayNewestBlockedDelivery(ctx context.Context, name string, window time.Duration) error {
	deliveries, err := rt.webhookDeliveries.ListWebhookDeliveries(ctx, name, gitReplayLookback, nil)
	if err != nil {
		return fmt.Errorf("list deliveries: %w", err)
	}
	if len(deliveries) == 0 {
		return nil
	}
	newest := deliveries[0]
	if !strings.Contains(newest.Error, sourcecache.ErrHostUnreachable.Error()) || time.Since(newest.ReceivedAt) > window {
		return nil
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return fmt.Errorf("load git source: %w", err)
	}
	status, msg := rt.replayDelivery(ctx, name, *gs, &newest)
	rt.logger.Info("api: replayed webhook delivery after git host recovery", slog.String("name", name), slog.String("delivery_id", newest.ID), slog.Int("status", status), slog.String("message", strings.TrimSpace(msg)))
	return nil
}

// replayDelivery re-runs a stored delivery through the processing branch it
// originally took; the operator replay endpoint and the recovery loop share it.
func (rt *Router) replayDelivery(ctx context.Context, name string, gs store.GitSource, d *store.WebhookDelivery) (status int, message string) {
	header := http.Header{}
	for k, v := range d.HeaderFields {
		if v != "" {
			header.Set(k, v)
		}
	}
	pctx, outcome := withWebhookOutcome(withWebhookReplay(ctx))
	status, message = rt.processGitPushWebhookPayload(pctx, name, gs, d.Payload, header)
	if outcome.AttemptID != "" {
		rt.kickDeployQueue(ctx)
	}
	return status, message
}

// gitHostAttentionItems raises one item per git host that is currently
// failing, naming the apps whose pushes are waiting on it.
func (rt *Router) gitHostAttentionItems(abilities []string) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	var items []attention.Item
	for _, h := range rt.sourceCache.Unreachable() {
		detail := fmt.Sprintf("failing since %s: %s. Known commits and rollbacks still deploy.", h.Since.UTC().Format("15:04 MST"), h.Reason)
		if len(h.Blocked) > 0 {
			detail += " Waiting to retry: " + strings.Join(h.Blocked, ", ") + "."
		}
		items = append(items, feedItem(attention.Warning, attention.KindGitHostUnreachable, h.Host, detail, map[string]string{"host": h.Host}))
	}
	return items
}
