package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/cigate"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const (
	maxCIRequiredChecks = 50
	maxCICheckNameLen   = 200
	zeroGitSHA          = "0000000000000000000000000000000000000000"
)

// ciGateSetter is what a git source store needs to persist the CI gate;
// *store.DB satisfies it, test fakes need not.
type ciGateSetter interface {
	SetGitSourceCIGate(ctx context.Context, serviceName string, wait bool, checks []string) error
}

// gitSHAPattern accepts a full SHA-1 or SHA-256 object name.
var gitSHAPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func trimCIChecks(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, strings.TrimSpace(n))
	}
	return out
}

func validateCIChecks(names []string) error {
	if len(names) > maxCIRequiredChecks {
		return fmt.Errorf("at most %d required checks", maxCIRequiredChecks)
	}
	for _, n := range names {
		if strings.TrimSpace(n) == "" || len(n) > maxCICheckNameLen {
			return fmt.Errorf("check names must be 1 to %d characters", maxCICheckNameLen)
		}
	}
	return nil
}

func (rt *Router) saveCIGate(w http.ResponseWriter, r *http.Request, name string, next gitDeploySettings) bool {
	setter, ok := rt.gitSources.(ciGateSetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "CI gate is not available on this control plane")
		return false
	}
	if next.WaitForCI {
		if status, msg := rt.ciGateUnavailable(r.Context(), name); status != 0 {
			writeError(w, status, msg)
			return false
		}
	}
	if err := setter.SetGitSourceCIGate(r.Context(), name, next.WaitForCI, trimCIChecks(next.CIRequiredChecks)); err != nil {
		rt.internalError(w, "api: set git source ci gate failed", err, slog.String("name", name))
		return false
	}
	return true
}

// newGitLabCIClient is a variable so tests can swap the SSRF-guarded client.
var newGitLabCIClient = gitlabapp.NewClient

// ciReaderFor builds the status reader for a forge, or nil when the kind has
// none. The GitLab project path comes from the stored repo URL (f.repo), never
// from the webhook body.
func ciReaderFor(f *forge, ref string) cigate.Provider {
	switch f.kind {
	case forgeGitHub:
		owner, repo := f.ownerName()
		ghc := githubapp.NewClient()
		return &cigate.GitHub{HTTP: ghc.HTTP, BaseURL: ghc.APIBaseURL(f.instanceURL), Token: f.token, Owner: owner, Repo: repo}
	case forgeGitLab:
		return &cigate.GitLab{Client: newGitLabCIClient(), InstanceURL: f.instanceURL, Token: f.token, ProjectPath: f.repo, Ref: ref}
	}
	return nil
}

// gateOnCI holds a push deploy until the commit's CI checks conclude. It
// returns a non-empty skip message when the deploy must not start. Sources
// without the gate deploy as before; a gated source whose provider cannot be
// read does not deploy.
func (rt *Router) gateOnCI(ctx context.Context, name string, gs store.GitSource, sha, tag string) string {
	if !ciGateApplies(gs, sha) {
		return ""
	}
	if !gitSHAPattern.MatchString(sha) {
		return rt.skipForCIGate(ctx, name, sha, "commit id is not a full SHA")
	}
	// A resolve or token-mint failure is retried inside the wait budget.
	reader := &lazyCIReader{build: func(c context.Context) (cigate.Provider, error) {
		f, err := rt.resolveForge(c, gs.RepoURL, gs.ConnectionID)
		if err != nil {
			return nil, err
		}
		r := ciReaderFor(f, ciGateRef(c, gs, tag))
		if r == nil {
			return nil, &cigate.TerminalError{Reason: msgCIProviderUnreadable}
		}
		return r, nil
	}}
	cfg := cigate.ConfigFromEnv()
	pol := cigate.Policy{Required: trimCIChecks(gs.CIRequiredChecks)}
	if rt.brand != nil && rt.brand.Name != "" {
		pol.IgnorePrefixes = []string{rt.brand.Name + " deploy"}
	}
	res, err := cigate.Wait(ctx, reader, sha, pol, cfg)
	if err == nil && res.Outcome == cigate.OutcomePass {
		return ""
	}
	reason := "CI gate interrupted"
	if err == nil {
		reason = res.Reason()
		if reader.p == nil {
			rt.logger.Warn("api: ci gate: provider cannot be read, deploy held back", slog.String("name", name))
			reason = msgCIProviderUnreadable
		}
	} else if cause := context.Cause(ctx); errors.Is(cause, errCIGateSuperseded) || errors.Is(cause, errCIGateShutdown) {
		reason = cause.Error()
	}
	return rt.skipForCIGate(ctx, name, sha, reason)
}

func (rt *Router) skipForCIGate(ctx context.Context, name, sha, reason string) string {
	// The wait may end because its context was cancelled; the audit row must still land.
	ctx = context.WithoutCancel(ctx)
	rt.logger.Info("api: ci gate: deploy skipped", slog.String("name", name), slog.String("commit", sha), slog.String("reason", reason))
	rt.recordCIGateSkip(ctx, name, sha, reason)
	return "not deployed: " + reason + "\n"
}

// ciGateRef is the ref the pipeline ran on. A tag push names its tag; release
// mode without one reads every ref and relies on the worst-state merge.
func ciGateRef(ctx context.Context, gs store.GitSource, tag string) string {
	if tag != "" {
		return tag
	}
	if b := commitMetaFrom(ctx).Branch; b != "" {
		return b
	}
	if effectiveGitSourceTriggerMode(gs.TriggerMode) == spec.TriggerModeRelease {
		return ""
	}
	return gs.Branch
}

// recordCIGateSkip adds a failed attempt to the deploy history, which also
// fires the app's deploy notification channels.
func (rt *Router) recordCIGateSkip(ctx context.Context, name, sha, reason string) {
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.logger.Warn("api: ci gate: record skip failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	_, _, finish, _ := rt.beginBuildDeployAttempt(ctx, deploy.Request{
		ServiceName: name,
		CommitSHA:   sha,
		ImageRepo:   defaultImageRepo(name, existing.Image),
	}, *existing, store.DeployAttemptSourceWebhook, "")
	finish(errors.New("skipped by CI gate: " + reason))
}

const msgCIProviderUnreadable = "cannot read provider status"

// lazyCIReader builds the provider reader on first use and keeps retrying the
// build while it fails, so a flaky token mint reads as pending, not as a skip.
type lazyCIReader struct {
	build func(ctx context.Context) (cigate.Provider, error)
	p     cigate.Provider
}

func (l *lazyCIReader) Checks(ctx context.Context, sha string) ([]cigate.Check, error) {
	if l.p == nil {
		p, err := l.build(ctx)
		if err != nil {
			return nil, err
		}
		l.p = p
	}
	return l.p.Checks(ctx, sha)
}

// ciGateUnavailable says why wait_for_ci cannot be enabled: 422 when no reader
// is possible for the source, 503 when the check itself failed and may pass later.
func (rt *Router) ciGateUnavailable(ctx context.Context, name string) (int, string) {
	const never = "wait_for_ci needs a GitHub App or GitLab connection that can read this repository's CI status"
	const transient = "could not confirm CI access right now (provider connection or token unavailable); try again"
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return http.StatusServiceUnavailable, transient
	}
	if !strings.HasPrefix(gs.RepoURL, "https://") && !strings.HasPrefix(gs.RepoURL, "http://") {
		return http.StatusUnprocessableEntity, never
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if err != nil {
		return http.StatusServiceUnavailable, transient
	}
	if ciReaderFor(f, "") == nil {
		return http.StatusUnprocessableEntity, never
	}
	return 0, ""
}

func ciGateApplies(gs store.GitSource, sha string) bool {
	return gs.WaitForCI && sha != "" && sha != zeroGitSHA
}

// gateCommitSHA is the commit whose statuses the gate reads. GitLab's tag_push
// `after` is the tag object for annotated tags, while checkout_sha is the commit.
func gateCommitSHA(body []byte, ev webhook.PushEvent) string {
	if !strings.HasPrefix(ev.Ref, "refs/tags/") {
		return ev.After
	}
	var p struct {
		CheckoutSHA string `json:"checkout_sha"`
	}
	if json.Unmarshal(body, &p) == nil && gitSHAPattern.MatchString(p.CheckoutSHA) {
		return p.CheckoutSHA
	}
	return ev.After
}

var (
	errCIGateSuperseded = errors.New("superseded by a newer push")
	errCIGateShutdown   = errors.New("interrupted by shutdown")
)

// ciGateTracker keeps at most one waiting gate per app. A router has no field
// for it, so trackers hang off the router pointer.
type ciGateTracker struct {
	mu    sync.Mutex
	slots map[string]*ciGateSlot
	root  context.Context
	stop  context.CancelFunc
}

// ciGateSlot remembers the waiting push so a later delivery can be ordered against it.
type ciGateSlot struct {
	cancel   context.CancelCauseFunc
	sha      string
	before   string
	commitAt time.Time
}

var ciGateTrackers sync.Map

func (rt *Router) ciGates() *ciGateTracker {
	if v, ok := ciGateTrackers.Load(rt); ok {
		t, _ := v.(*ciGateTracker)
		return t
	}
	root, stop := context.WithCancel(context.Background())
	v, loaded := ciGateTrackers.LoadOrStore(rt, &ciGateTracker{slots: map[string]*ciGateSlot{}, root: root, stop: stop})
	if loaded {
		stop()
	}
	t, _ := v.(*ciGateTracker)
	return t
}

// CancelCIGates ends every waiting CI gate; each records an "interrupted by shutdown" skip.
func (rt *Router) CancelCIGates() { rt.ciGates().stop() }

// ShutdownCIGates cancels waiting gates and waits for their skip rows, so a
// restart never leaves a delivery stuck at 202. It reports whether all finished by ctx.
func (rt *Router) ShutdownCIGates(ctx context.Context) bool {
	rt.CancelCIGates()
	done := make(chan struct{})
	go func() {
		rt.previewDeploys.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// begin makes job the app's only waiting gate. It refuses, returning why, when
// the waiting push is the same commit or provably newer, so a late or replayed
// older delivery can never cancel a newer wait.
func (t *ciGateTracker) begin(job ciGateJob, cancel context.CancelCauseFunc) (*ciGateSlot, string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if old := t.slots[job.name]; old != nil {
		if old.sha == job.sha {
			return nil, "commit " + webhookShortSHA(job.sha) + " is already waiting for CI"
		}
		cur := store.DeployCursor{CommitSHA: old.sha, Before: old.before, CommitAt: old.commitAt}
		ord := store.DeployOrder{Automatic: true, CommitSHA: job.sha, Before: job.before, CommitAt: job.commitAt}
		if deploy.CheckOrder(cur, ord) != nil {
			return nil, "older than the push already waiting"
		}
		old.cancel(errCIGateSuperseded)
	}
	s := &ciGateSlot{cancel: cancel, sha: job.sha, before: job.before, commitAt: job.commitAt}
	t.slots[job.name] = s
	return s, ""
}

// claim reports whether s is still the newest gate; the winner leaves the table
// so a later push starts a fresh wait instead of cancelling a running deploy.
func (t *ciGateTracker) claim(name string, s *ciGateSlot) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.slots[name] != s {
		return false
	}
	delete(t.slots, name)
	return true
}

type ciGateMarkKey struct{}

// withCIGateMark lets the webhook handler see that a push moved to a background
// wait, so it keeps the replay claim for it.
func withCIGateMark(ctx context.Context) (context.Context, *atomic.Bool) {
	b := &atomic.Bool{}
	return context.WithValue(ctx, ciGateMarkKey{}, b), b
}

func markCIGateAccepted(ctx context.Context) {
	if b, _ := ctx.Value(ciGateMarkKey{}).(*atomic.Bool); b != nil {
		b.Store(true)
	}
}

// ciGateJob is one gated trigger: wait for gateSHA's CI, then run on the detached context.
type ciGateJob struct {
	name     string
	gs       store.GitSource
	gateSHA  string
	tag      string
	sha      string
	before   string
	commitAt time.Time
	label    string
	run      func(ctx context.Context)
}

// startGatedJob acknowledges now and waits for CI in the background, so the
// forge's short delivery timeout cannot disable the hook.
func (rt *Router) startGatedJob(ctx context.Context, job ciGateJob) (int, string) {
	bg, _ := withWebhookOutcome(context.WithoutCancel(ctx))
	gctx, cancel := context.WithCancelCause(bg)
	tracker := rt.ciGates()
	slot, refused := tracker.begin(job, cancel)
	if slot == nil {
		cancel(nil)
		return http.StatusOK, "ignored: " + refused + "\n"
	}
	stopRoot := context.AfterFunc(tracker.root, func() { cancel(errCIGateShutdown) })
	markCIGateAccepted(ctx)
	rt.previewDeploys.Add(1)
	go func() {
		defer rt.previewDeploys.Done()
		defer stopRoot()
		defer cancel(nil)
		defer tracker.claim(job.name, slot)
		defer func() {
			if r := recover(); r != nil {
				rt.logger.Error("api: ci gate: background job panicked", slog.String("name", job.name), slog.Any("panic", r))
			}
		}()
		rt.runGatedJob(gctx, bg, tracker, slot, job)
	}()
	return http.StatusAccepted, "accepted: waiting for CI before deploying " + job.label + "\n"
}

func (rt *Router) runGatedJob(gctx, bg context.Context, tracker *ciGateTracker, slot *ciGateSlot, job ciGateJob) {
	if rt.gateOnCI(gctx, job.name, job.gs, job.gateSHA, job.tag) != "" {
		return
	}
	// Passing is not enough: a newer push that arrived meanwhile owns the app now.
	if !tracker.claim(job.name, slot) {
		rt.skipForCIGate(bg, job.name, job.gateSHA, errCIGateSuperseded.Error())
		return
	}
	job.run(bg)
	if o := webhookOutcomeFrom(bg); o != nil && o.AttemptID != "" {
		rt.kickDeployQueue(bg)
	}
}

// gatedPushEvent makes a tag push deploy the very commit that was gated, never
// a different object the payload names.
func gatedPushEvent(ev webhook.PushEvent, gateSHA string) webhook.PushEvent {
	if strings.HasPrefix(ev.Ref, "refs/tags/") {
		ev.After = gateSHA
	}
	return ev
}

// startGatedPush gates a push already adjusted by gatedPushEvent.
func (rt *Router) startGatedPush(ctx context.Context, name string, gs store.GitSource, ev webhook.PushEvent, gateSHA string) (int, string) {
	tag, isTag := strings.CutPrefix(ev.Ref, "refs/tags/")
	if !isTag {
		tag = ""
	}
	return rt.startGatedJob(ctx, ciGateJob{
		name: name, gs: gs, gateSHA: gateSHA, tag: tag,
		sha: ev.After, before: ev.Before, commitAt: ev.HeadCommitAt, label: ev.After,
		run: func(c context.Context) { rt.deployGatedPush(c, name, gs, ev) },
	})
}

// deployGatedPush runs the hold-and-enqueue half of admission after the gate
// passed, so a freeze or approval requirement that began during the wait applies.
func (rt *Router) deployGatedPush(ctx context.Context, name string, gs store.GitSource, ev webhook.PushEvent) {
	if status, msg, handled := rt.admitGitPush(ctx, name, ev); handled {
		if status >= http.StatusBadRequest {
			rt.logger.Warn("api: ci gate: gated push not admitted", slog.String("name", name), slog.Int("status", status), slog.String("message", msg))
		}
		return
	}
	queued, _, release := rt.queueIfBusy(ctx, name, gs, deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: ev.After, CommitLabel: ev.After, CommitSHA: ev.After, Before: ev.Before, CommitAt: ev.HeadCommitAt, Branch: strings.TrimPrefix(ev.Ref, "refs/heads/")}, name+":"+ev.After)
	defer release()
	if queued {
		return
	}
	status, msg := rt.deployFromGitSource(withStartRelease(ctx, release), name, gs, ev.After, ev.After, rt.nextOrder(ctx, name, ev.After, ev.Before, ev.HeadCommitAt))
	if status >= http.StatusBadRequest {
		rt.logger.Warn("api: ci gate: gated deploy failed", slog.String("name", name), slog.Int("status", status), slog.String("message", msg))
	}
}
