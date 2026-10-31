package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envGitPollEnabled     = "APP_GIT_POLL_ENABLED"
	envGitPollInterval    = "APP_GIT_POLL_INTERVAL"
	envGitPollJitter      = "APP_GIT_POLL_JITTER"
	envGitPollConcurrency = "APP_GIT_POLL_CONCURRENCY"
	envGitPollMaxBackoff  = "APP_GIT_POLL_MAX_BACKOFF"

	defaultGitPollInterval    = 5 * time.Minute
	defaultGitPollJitter      = 0.2
	defaultGitPollConcurrency = 4
	defaultGitPollMaxBackoff  = time.Hour
	gitPollHeadTimeout        = 30 * time.Second
	gitPollSourceTimeout      = 2 * time.Minute

	gitPollReason = "detected by poll, webhook not received"
)

type gitPollConfig struct {
	Enabled     bool
	Interval    time.Duration
	Jitter      float64
	Concurrency int
	MaxBackoff  time.Duration
}

// gitPollConfigFromEnv reads the poller settings: on by default, every five
// minutes with 20 percent jitter, four sources at a time.
func gitPollConfigFromEnv() gitPollConfig {
	c := gitPollConfig{Enabled: true, Interval: defaultGitPollInterval, Jitter: defaultGitPollJitter, Concurrency: defaultGitPollConcurrency, MaxBackoff: defaultGitPollMaxBackoff}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv(envGitPollEnabled))); v == "0" || v == "false" || v == "off" {
		c.Enabled = false
	}
	if d, err := time.ParseDuration(os.Getenv(envGitPollInterval)); err == nil && d > 0 {
		c.Interval = d
	}
	if f, err := strconv.ParseFloat(os.Getenv(envGitPollJitter), 64); err == nil && f >= 0 && f <= 1 {
		c.Jitter = f
	}
	if n, err := strconv.Atoi(os.Getenv(envGitPollConcurrency)); err == nil && n > 0 {
		c.Concurrency = n
	}
	if d, err := time.ParseDuration(os.Getenv(envGitPollMaxBackoff)); err == nil && d > 0 {
		c.MaxBackoff = d
	}
	return c
}

// gitPollState is the poller's per-source memory: when to look again and how
// many failures in a row, so a failing or rate limited host is backed off.
type gitPollState struct {
	mu       sync.Mutex
	next     map[string]time.Time
	failures map[string]int
}

func newGitPollState() *gitPollState {
	return &gitPollState{next: map[string]time.Time{}, failures: map[string]int{}}
}

func (s *gitPollState) due(name string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !now.Before(s.next[name])
}

func (s *gitPollState) schedule(name string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next[name] = at
}

func (s *gitPollState) fail(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures[name]++
	return s.failures[name]
}

func (s *gitPollState) ok(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failures, name)
}

func (c gitPollConfig) jittered(base time.Duration) time.Duration {
	if c.Jitter <= 0 {
		return base
	}
	spread := float64(base) * c.Jitter
	return base + time.Duration((rand.Float64()*2-1)*spread) //nolint:gosec,forbidigo // jitter only spreads polls in time
}

func (c gitPollConfig) backoff(failures int, retryAfter time.Duration) time.Duration {
	d := c.Interval
	for range min(failures, 10) {
		d *= 2
		if d >= c.MaxBackoff {
			d = c.MaxBackoff
			break
		}
	}
	return max(d, retryAfter)
}

// RunGitPoller periodically compares each git source's remote branch head
// with what it last saw and queues a deploy when a webhook was missed. It
// returns when ctx is cancelled. Builds never run here: a missed head is
// queued like a webhook push and the deploy queue runs it.
func (rt *Router) RunGitPoller(ctx context.Context) {
	cfg := gitPollConfigFromEnv()
	if !cfg.Enabled {
		rt.logger.Info("api: git poller disabled", slog.String("env", envGitPollEnabled))
		return
	}
	state := newGitPollState()
	tick := time.NewTicker(max(cfg.Interval/4, time.Second))
	defer tick.Stop()
	for {
		rt.pollGitSourcesOnce(ctx, cfg, state, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// pollGitSourcesOnce polls every due auto-deploy git source with bounded concurrency.
func (rt *Router) pollGitSourcesOnce(ctx context.Context, cfg gitPollConfig, state *gitPollState, now time.Time) {
	if rt.gitSources == nil || ctx.Err() != nil {
		return
	}
	sources, err := rt.gitSources.ListGitSources(ctx)
	if err != nil {
		rt.logger.Warn("api: git poller: list git sources failed", slog.String("error", err.Error()))
		return
	}
	sem := make(chan struct{}, cfg.Concurrency)
	var wg sync.WaitGroup
	for _, s := range sources {
		if !state.due(s.ServiceName, now) {
			continue
		}
		state.schedule(s.ServiceName, now.Add(cfg.jittered(cfg.Interval)))
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			sctx, cancel := context.WithTimeout(ctx, gitPollSourceTimeout)
			defer cancel()
			rt.pollGitSource(sctx, cfg, state, s.ServiceName)
		}()
	}
	wg.Wait()
}

// gitPollProviderRateLimited reports a provider connection that is rate limited right now.
func (rt *Router) gitPollProviderRateLimited(ctx context.Context, gs store.GitSource) (time.Duration, bool) {
	host, _ := repoHostAndPath(gs.RepoURL)
	var keys []string
	if rt.githubInstanceHost(ctx) == host {
		keys = append(keys, gitProviderGitHub)
	}
	for _, p := range connectionGitProviders {
		if c, err := rt.gitConnForRepo(ctx, p, gs.RepoURL, gs.ConnectionID); err == nil {
			keys = append(keys, c.ID)
		}
	}
	for _, k := range keys {
		if h := rt.gitConnHealth(ctx, k); h.Status == store.GitConnRateLimited && h.NextCheckAt != nil {
			return time.Until(*h.NextCheckAt), true
		}
	}
	return 0, false
}

func (rt *Router) githubInstanceHost(ctx context.Context) string {
	conn, err := rt.githubApp.GetGitHubAppConnection(ctx)
	if err != nil {
		return ""
	}
	if conn.InstanceURL == "" {
		return "github.com"
	}
	return repoURLHost(conn.InstanceURL)
}

// pollGitSource looks at one source's remote head. It never polls a source
// whose auto-deploy is off (release mode), and records the poll either way.
func (rt *Router) pollGitSource(ctx context.Context, cfg gitPollConfig, state *gitPollState, name string) {
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil || gs == nil {
		return
	}
	if effectiveGitSourceTriggerMode(gs.TriggerMode) == spec.TriggerModeRelease {
		return
	}
	now := time.Now()
	if wait, limited := rt.gitPollProviderRateLimited(ctx, *gs); limited {
		state.schedule(name, now.Add(cfg.backoff(0, wait)))
		return
	}

	head, err := rt.gitPollHead(ctx, name, *gs)
	if err != nil {
		retry := time.Duration(0)
		if rl, ok := gitprovider.AsRateLimit(err); ok {
			retry = time.Until(rl.NextCheck)
		}
		state.schedule(name, now.Add(cfg.backoff(state.fail(name), retry)))
		rt.recordPoll(ctx, name, now, clipError(err))
		rt.logger.Warn("api: git poller: resolve branch head failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	state.ok(name)
	rt.recordPoll(ctx, name, now, "")
	if err := rt.handlePolledHead(ctx, name, *gs, head); err != nil {
		rt.logger.Warn("api: git poller: queue missed deploy failed", slog.String("name", name), slog.String("head", webhookShortSHA(head)), slog.String("error", err.Error()))
	}
}

func (rt *Router) gitPollHead(ctx context.Context, name string, gs store.GitSource) (string, error) {
	token, err := rt.resolveGitSourceDeployToken(ctx, name)
	if err != nil {
		return "", fmt.Errorf("resolve deploy token: %w", err)
	}
	hctx, cancel := context.WithTimeout(ctx, gitPollHeadTimeout)
	defer cancel()
	head, err := rt.resolveBranchSHA(hctx, gs.RepoURL, gs.Branch, token)
	if err != nil {
		return "", fmt.Errorf("resolve head: %w", err)
	}
	return head, nil
}

func (rt *Router) recordPoll(ctx context.Context, name string, at time.Time, pollErr string) {
	if rt.gitRel == nil {
		return
	}
	if err := rt.gitRel.SetGitSourcePollResult(ctx, name, at, pollErr); err != nil {
		rt.logger.Warn("api: git poller: record poll failed", slog.String("name", name), slog.String("error", err.Error()))
	}
}

// handlePolledHead queues a deploy for head when nothing has seen it yet. The
// first head seen for a source is only remembered: it cannot be told apart
// from a deliberate rollback, so it is never deployed. Later heads that no
// webhook delivered are deployed once, deduped by commit SHA against webhooks.
func (rt *Router) handlePolledHead(ctx context.Context, name string, gs store.GitSource, head string) error {
	if rt.gitRel == nil || head == "" {
		return nil
	}
	rel, err := rt.gitRel.GetGitSourceReliability(ctx, name)
	if err != nil {
		return fmt.Errorf("load poll state: %w", err)
	}
	if rel.LastSeenHead == head {
		return nil
	}
	if rel.LastSeenHead == "" {
		return rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false)
	}
	if rt.builder == nil || rt.deploySafety == nil {
		return nil
	}

	rt.gitEnqueueMu.Lock()
	defer rt.gitEnqueueMu.Unlock()
	if state := rt.commitDeployState(ctx, name, head); state != "" {
		return rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false)
	}
	if reason := rt.staleGitDelivery(ctx, name, head, rel.LastSeenHead, time.Time{}); reason != "" {
		rt.logger.Info("api: git poller: head is older than what is deployed or queued", slog.String("name", name), slog.String("reason", reason))
		return rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false)
	}
	hr := deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: head, CommitLabel: head, CommitSHA: head, Before: rel.LastSeenHead, Branch: gs.Branch}
	if held, _ := rt.holdIfFrozen(ctx, name, hr, name+":"+head); held {
		return rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false)
	}
	if handled, _, _ := rt.holdIfProtected(ctx, name, hr, name+":"+head, store.DeployAttemptSourcePoll, SystemApprovalActor); handled {
		return rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false)
	}
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return fmt.Errorf("load app: %w", err)
	}
	rt.buildStartMu.Lock()
	id, err := rt.enqueueDeploy(ctx, *existing, store.DeployAttemptSourcePoll, name+":"+head, hr)
	rt.buildStartMu.Unlock()
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("enqueue deploy: %w", err)
	}
	if err := rt.gitRel.NoteGitSourceHead(ctx, name, head, time.Now(), false); err != nil {
		return fmt.Errorf("remember head: %w", err)
	}
	rt.logger.Info("api: git poller: missed webhook, deploy queued", slog.String("name", name), slog.String("attempt_id", id), slog.String("head", webhookShortSHA(head)), slog.String("reason", gitPollReason))
	rt.kickDeployQueue(ctx)
	return nil
}
