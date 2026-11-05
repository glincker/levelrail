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
		if msg := rt.ciGateUnavailable(r.Context(), name); msg != "" {
			writeError(w, http.StatusUnprocessableEntity, msg)
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
	} else if errors.Is(context.Cause(ctx), errCIGateSuperseded) {
		reason = errCIGateSuperseded.Error()
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

// ciGateUnavailable says why wait_for_ci cannot work for the app's source, or "" when it can.
func (rt *Router) ciGateUnavailable(ctx context.Context, name string) string {
	const msg = "wait_for_ci needs a GitHub App or GitLab connection that can read this repository's CI status"
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return msg
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if err != nil || ciReaderFor(f, "") == nil {
		return msg
	}
	return ""
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

var errCIGateSuperseded = errors.New("superseded by a newer push")

// ciGateTracker keeps at most one waiting gate per app. A router has no field
// for it, so trackers hang off the router pointer.
type ciGateTracker struct {
	mu    sync.Mutex
	slots map[string]*ciGateSlot
}

type ciGateSlot struct{ cancel context.CancelCauseFunc }

var ciGateTrackers sync.Map

func (rt *Router) ciGates() *ciGateTracker {
	v, _ := ciGateTrackers.LoadOrStore(rt, &ciGateTracker{slots: map[string]*ciGateSlot{}})
	t, _ := v.(*ciGateTracker)
	return t
}

// begin makes cancel the app's only waiting gate and cancels the one it replaces.
func (t *ciGateTracker) begin(name string, cancel context.CancelCauseFunc) *ciGateSlot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if old := t.slots[name]; old != nil {
		old.cancel(errCIGateSuperseded)
	}
	s := &ciGateSlot{cancel: cancel}
	t.slots[name] = s
	return s
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

// startGatedPush acknowledges the webhook now and waits for CI, then deploys,
// in the background so the forge's short delivery timeout cannot disable the hook.
func (rt *Router) startGatedPush(ctx context.Context, name string, gs store.GitSource, ev webhook.PushEvent, gateSHA string) (int, string) {
	bg, _ := withWebhookOutcome(context.WithoutCancel(ctx))
	gctx, cancel := context.WithCancelCause(bg)
	tracker := rt.ciGates()
	slot := tracker.begin(name, cancel)
	tag, isTag := strings.CutPrefix(ev.Ref, "refs/tags/")
	if !isTag {
		tag = ""
	}
	rt.previewDeploys.Add(1)
	go func() {
		defer rt.previewDeploys.Done()
		defer cancel(nil)
		defer tracker.claim(name, slot)
		defer func() {
			if r := recover(); r != nil {
				rt.logger.Error("api: ci gate: background push panicked", slog.String("name", name), slog.Any("panic", r))
			}
		}()
		rt.runGatedPush(gctx, bg, tracker, slot, name, gs, ev, gateSHA, tag)
	}()
	return http.StatusAccepted, "accepted: waiting for CI before deploying " + ev.After + "\n"
}

func (rt *Router) runGatedPush(gctx, bg context.Context, tracker *ciGateTracker, slot *ciGateSlot, name string, gs store.GitSource, ev webhook.PushEvent, gateSHA, tag string) {
	if rt.gateOnCI(gctx, name, gs, gateSHA, tag) != "" {
		return
	}
	// Passing is not enough: a newer push that arrived meanwhile owns the app now.
	if !tracker.claim(name, slot) {
		rt.skipForCIGate(bg, name, gateSHA, errCIGateSuperseded.Error())
		return
	}
	queued, _, release := rt.queueIfBusy(bg, name, gs, deploy.HeldRequest{Kind: deploy.HeldKindGit, CheckoutRef: ev.After, CommitLabel: ev.After, CommitSHA: ev.After, Before: ev.Before, CommitAt: ev.HeadCommitAt, Branch: strings.TrimPrefix(ev.Ref, "refs/heads/")}, name+":"+ev.After)
	defer release()
	if !queued {
		status, msg := rt.deployFromGitSource(withStartRelease(bg, release), name, gs, ev.After, ev.After, rt.nextOrder(bg, name, ev.After, ev.Before, ev.HeadCommitAt))
		if status >= http.StatusBadRequest {
			rt.logger.Warn("api: ci gate: gated deploy failed", slog.String("name", name), slog.Int("status", status), slog.String("message", msg))
		}
	}
	if o := webhookOutcomeFrom(bg); o != nil && o.AttemptID != "" {
		rt.kickDeployQueue(bg)
	}
}
