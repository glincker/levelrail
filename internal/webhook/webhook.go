// Package webhook is TASKS.md 1.5's git integration: a GitHub push-webhook
// receiver that verifies the request signature, extracts the pushed
// commit SHA, fetches the repository at that SHA to a local checkout, and
// hands the result to internal/deploy's Pipeline (TASKS.md 1.4), the same
// pipeline a manual deploy trigger will use once the HTTP API (1.9)
// exists.
//
// Handler is an http.Handler, not a standalone server: TASKS.md 1.9's
// HTTP API package mounts it at whatever path it chooses, so this package
// must never call http.ListenAndServe itself.
//
// Scope boundary: Config is deliberately static, single-app configuration
// (one secret, one repo, one branch, one spec.Service). This phase's
// exit criterion is a single app, thesvg.org, deploying from a git
// push; this package does not build a multi-tenant "which repo maps to
// which app.yaml" registry, since designing that now would be exactly the
// speculative-ahead-of-a-real-second-app work the project's engineering
// standards warn against.
// A second app arriving is the trigger to revisit this, not a hypothesis
// about one arriving.
package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultBranch is used when Config.Branch is empty.
const DefaultBranch = "main"

// maxPayloadBytes bounds how much of a request body ServeHTTP will read.
// GitHub documents 25 MB as its own webhook payload ceiling
// (docs.github.com/webhooks: "payloads... are limited to 25 MB"), so this
// mirrors GitHub's own protocol limit rather than being a tunable
// business threshold: nothing this package does gets more correct by
// raising or lowering it, it only bounds how much an unauthenticated
// request can force this handler to buffer before signature
// verification even runs.
const maxPayloadBytes = 25 << 20

// Deployer is the narrow surface this package needs from
// internal/deploy.Pipeline, so tests can fake it without a real BuildKit
// connection or Docker daemon, the same narrow-interface pattern
// internal/deploy and internal/reconcile/application already use.
// *deploy.Pipeline satisfies this.
type Deployer interface {
	Deploy(ctx context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error)
}

// AttemptStore is the narrow store surface Handler needs to record one
// deploy_attempts row per triggering push: this package is the third
// (and, per docs-local/research/deploy-attempt-id-and-log-persistence.md's
// own framing, the most important) of the three real deploy-trigger
// paths this history exists for. An unattended push-to-deploy that fails
// with no replayable record defeats the point of having a log viewer at
// all. *store.DB satisfies this structurally.
type AttemptStore interface {
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error
}

// DeployNotifier is the narrow surface Handler needs to fire a
// deploy-outcome notification (Slack/Discord/Telegram/generic-webhook/
// email, wave-2 roadmap item #5) once a push-triggered deploy reaches a
// terminal state: distinct from this package's own git-integration
// concern, this is deliberately just "hand the finished outcome to
// whatever internal/alerting.DeployDispatcher does with it," the same
// narrow-consumer-interface shape AttemptStore above already uses.
// *alerting.DeployDispatcher satisfies this structurally. nil is valid:
// Handler.notifier's own doc comment covers what a nil value does.
type DeployNotifier interface {
	Dispatch(ctx context.Context, resourceID string, ev alerting.DeployOutcome)
}

// Config is everything needed to map a validated GitHub push into a
// deploy.Request. See the package doc comment for why this is
// single-app, not a multi-tenant registry.
type Config struct {
	// Secret is the shared HMAC secret configured on the GitHub webhook,
	// used to verify the X-Hub-Signature-256 header. Required: a Handler
	// built with an empty Secret rejects every request, since an empty
	// secret would make the signature check meaningless.
	Secret []byte
	// RepoURL is the git remote this handler clones on a triggering
	// push, e.g. "https://github.com/org/thesvg.org.git". Deliberately
	// server-side configuration, never taken from the webhook payload:
	// only the commit SHA (a hash, not a URL) comes from the untrusted
	// request, so a forged payload cannot redirect a clone at an
	// attacker-controlled remote.
	RepoURL string
	// Branch triggers a deploy on push. Pushes to any other branch are
	// accepted (200 OK, so GitHub does not retry) but do not deploy.
	// Defaults to DefaultBranch when empty.
	Branch string
	// ServiceName, Service, and ImageRepo are passed straight through to
	// deploy.Request; see internal/deploy for what each means.
	ServiceName string
	Service     spec.Service
	ImageRepo   string
}

func (c Config) branch() string {
	if c.Branch == "" {
		return DefaultBranch
	}
	return c.Branch
}

func (c Config) targetRef() string {
	return "refs/heads/" + c.branch()
}

// fetchFunc fetches repoURL at sha to a local directory, returning the
// directory and a cleanup func that removes it. Overridable in tests so
// dispatch-logic tests don't need real network or git I/O; see
// cloneAndCheckout for the real implementation.
type fetchFunc func(ctx context.Context, repoURL, sha string) (dir string, cleanup func(), err error)

// Handler is an http.Handler that receives GitHub push event payloads,
// verifies them, and triggers a deploy through Deployer when the push
// targets Config.Branch.
type Handler struct {
	cfg      Config
	deployer Deployer
	// attempts and recorder are both nil-valid, together: see
	// deployAttemptEnabled's own doc comment for why they're gated as a
	// pair rather than independently. cmd/levelrail/main.go always wires
	// both (a real *store.DB and a real *deploylog.Recorder shared with
	// internal/api.Router, see that package's own doc comment for why
	// the Recorder specifically must be the same instance); nil/nil here
	// exists mainly so this package's own dispatch-logic tests, which
	// have nothing to do with attempt tracking, don't need to fake it.
	attempts AttemptStore
	recorder *deploylog.Recorder
	// notifier fires a deploy-outcome notification once beginDeployAttempt's
	// finish closure has recorded a terminal status. nil (the default,
	// e.g. this package's own dispatch-logic tests) simply means no
	// deploy-outcome notification is sent for a push-triggered deploy,
	// the same "optional signal, absence is not an error" shape every
	// other optional collaborator in this codebase has, never a panic or
	// a swallowed error.
	notifier DeployNotifier
	log      *slog.Logger
	fetch    fetchFunc
}

// New builds a Handler. attempts and recorder may both be nil (see
// Handler.attempts' own doc comment); notifier may be nil (see
// Handler.notifier's own doc comment); log defaults to slog.Default() if
// nil.
func New(cfg Config, deployer Deployer, attempts AttemptStore, recorder *deploylog.Recorder, notifier DeployNotifier, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{cfg: cfg, deployer: deployer, attempts: attempts, recorder: recorder, notifier: notifier, log: log, fetch: cloneAndCheckout}
}

// deployAttemptEnabled reports whether this Handler should record
// deploy_attempts history for the push it's about to deploy. attempts
// and recorder are checked together, not independently: a row with no
// recorder to feed it would have a permanently-empty log with no way to
// ever change that, and a recorder running with nothing to persist to
// would fan out live events nobody could reconnect to see a replay of
// after the fact, either partial state is worse than plainly falling
// back to the pre-existing build.SlogProgress behavior for this push.
func (h *Handler) deployAttemptEnabled() bool {
	return h.attempts != nil && h.recorder != nil
}

// beginDeployAttempt mints and saves a deploy_attempts row for req (see
// AttemptStore's own doc comment for why this package needs one at all),
// and returns the progress func to pass to h.deployer.Deploy plus a
// finish func the caller must invoke exactly once with Deploy's result,
// success or failure. When deployAttemptEnabled is false, or minting/
// saving the row itself fails (logged, not fatal to the deploy: a
// history-tracking failure must never block the actual deploy this push
// triggered), this falls back to build.SlogProgress and a no-op finish,
// the identical behavior this package had before attempt tracking
// existed.
//
// image mirrors internal/deploy's own deployDockerfile tag construction
// (ImageRepo + ":" + CommitSHA) rather than waiting for Deploy to return
// it: the tag is fully determined by req before any build I/O happens,
// so a failed build's history row still shows what tag it was trying to
// produce, not a blank field.
func (h *Handler) beginDeployAttempt(ctx context.Context, req deploy.Request) (progress func(build.ProgressEvent), finish func(tag string, deployErr error)) {
	noop := func(string, error) {}
	if !h.deployAttemptEnabled() {
		return build.SlogProgress(h.log), noop
	}

	id, err := store.NewDeployAttemptID()
	if err != nil {
		h.log.Error("webhook: mint deploy attempt id failed", "error", err)
		return build.SlogProgress(h.log), noop
	}

	image := req.ImageRepo + ":" + req.CommitSHA
	if err := h.attempts.SaveDeployAttempt(ctx, store.DeployAttempt{
		ID: id, ServiceName: req.ServiceName, Image: image,
		Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		h.log.Error("webhook: save deploy attempt failed", "attempt_id", id, "error", err)
		return build.SlogProgress(h.log), noop
	}

	h.recorder.Start(id)
	progress = h.recorder.Progress(id)
	finish = func(_ string, deployErr error) {
		// context.Background(), not ctx/r.Context(): this runs after
		// Deploy has already returned, and must still complete (flush
		// the log, mark the terminal status) even if the triggering
		// HTTP request's own context is on its way out, the same
		// reasoning deploylog.Recorder.flush's own doc comment applies
		// one layer down.
		finishCtx := context.Background()
		h.recorder.Finish(finishCtx, id)

		status := store.DeployAttemptStatusSucceeded
		errMsg := ""
		if deployErr != nil {
			status = store.DeployAttemptStatusFailed
			errMsg = deployErr.Error()
		}
		if err := h.attempts.FinishDeployAttempt(finishCtx, id, status, time.Now(), errMsg); err != nil {
			h.log.Error("webhook: finish deploy attempt failed", "attempt_id", id, "error", err)
			return
		}
		// Deploy-outcome notification (wave-2 roadmap item #5): fires
		// only now, once FinishDeployAttempt has actually persisted the
		// attempt's terminal status, never for the in-progress "running"
		// status this closure's caller (ServeHTTP's call to h.deployer.Deploy)
		// hasn't returned from yet. h.notifier is nil-valid, see its own
		// doc comment.
		if h.notifier != nil {
			h.notifier.Dispatch(finishCtx, resourceIDForService(req.ServiceName), alerting.DeployOutcome{
				AppName: req.ServiceName, Image: image, Succeeded: deployErr == nil, Error: errMsg,
			})
		}
	}
	return progress, finish
}

// resourceIDForService mirrors internal/api's own resourceIDForApp
// (telemetry.metrics.go): "service:<name>", the same stable identifier
// convention used to scope both an app's metrics/logs and its alert
// rules/deploy-notify-targets to one resource ID. Duplicated as a plain
// string format here rather than imported, the same
// applicationControllerName-style choice internal/api/deploys.go's own
// doc comment already makes for that package's identical duplication of
// internal/reconcile's controller-name convention: importing
// internal/api here to reuse one string format would pull that whole
// package's HTTP-handler surface into this one just for a formatting
// rule, and if the convention ever changes, both call sites need
// updating together regardless of which one "owns" it.
func resourceIDForService(name string) string {
	return "service:" + name
}

// pushEvent is the subset of GitHub's push event payload this package
// needs. See
// https://docs.github.com/en/webhooks/webhook-events-and-payloads#push.
type pushEvent struct {
	Ref   string `json:"ref"`
	After string `json:"after"`
}

// ServeHTTP implements http.Handler. Response codes: 200 for a
// successfully triggered or correctly-ignored (wrong branch) push, 400
// for a malformed payload, 401 for a missing or invalid signature, 500
// with a non-leaky message if the deploy itself fails. Deploy progress is
// logged via slog, not streamed in the response: GitHub expects a fast
// response and does not wait for one, actual progress streaming to a UI
// is TASKS.md 1.9's job.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxPayloadBytes+1))
	if err != nil {
		h.log.Warn("webhook: failed to read request body", "error", err)
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}
	if len(body) > maxPayloadBytes {
		h.log.Warn("webhook: request body exceeds max payload size", "max_bytes", maxPayloadBytes)
		http.Error(w, "request body too large", http.StatusBadRequest)
		return
	}

	if !h.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		h.log.Warn("webhook: signature verification failed", "remote_addr", r.RemoteAddr)
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}

	var ev pushEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		h.log.Warn("webhook: malformed payload", "error", err)
		http.Error(w, "malformed payload", http.StatusBadRequest)
		return
	}
	if ev.After == "" || ev.Ref == "" {
		h.log.Warn("webhook: payload missing ref or after", "ref", ev.Ref, "after", ev.After)
		http.Error(w, "malformed payload: missing ref or after", http.StatusBadRequest)
		return
	}

	wantRef := h.cfg.targetRef()
	if ev.Ref != wantRef {
		h.log.Info("webhook: ignoring push to non-target branch", "ref", ev.Ref, "want_ref", wantRef)
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, "ignored: push to %q, deploys trigger on %q only\n", ev.Ref, wantRef); err != nil {
			h.log.Warn("webhook: failed to write response body", "error", err)
		}
		return
	}

	h.log.Info("webhook: push matched target branch, fetching source", "commit", ev.After, "ref", ev.Ref)

	sourceDir, cleanup, err := h.fetch(r.Context(), h.cfg.RepoURL, ev.After)
	if err != nil {
		h.log.Error("webhook: fetching source failed", "commit", ev.After, "repo_url", h.cfg.RepoURL, "error", err)
		http.Error(w, "deploy failed", http.StatusInternalServerError)
		return
	}
	defer cleanup()

	req := deploy.Request{
		ServiceName: h.cfg.ServiceName,
		Service:     h.cfg.Service,
		SourceDir:   sourceDir,
		CommitSHA:   ev.After,
		ImageRepo:   h.cfg.ImageRepo,
	}

	progress, finishAttempt := h.beginDeployAttempt(r.Context(), req)
	tag, err := h.deployer.Deploy(r.Context(), req, progress)
	finishAttempt(tag, err)
	if err != nil {
		h.log.Error("webhook: deploy failed", "commit", ev.After, "service", h.cfg.ServiceName, "error", err)
		http.Error(w, "deploy failed", http.StatusInternalServerError)
		return
	}

	h.log.Info("webhook: deploy triggered", "commit", ev.After, "service", h.cfg.ServiceName, "tag", tag)
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "deploy triggered: %s\n", tag); err != nil {
		h.log.Warn("webhook: failed to write response body", "error", err)
	}
}

// verifySignature checks header against an HMAC-SHA256 of body keyed by
// h.cfg.Secret, GitHub's X-Hub-Signature-256 scheme
// (docs.github.com/webhooks: "validating-webhook-deliveries"). Comparison
// uses hmac.Equal, constant-time by construction, rather than a plain
// byte-slice or string comparison, since a timing-observable comparison
// here would let an attacker recover a valid signature byte by byte.
func (h *Handler) verifySignature(body []byte, header string) bool {
	if len(h.cfg.Secret) == 0 {
		// An empty configured secret can never produce a valid
		// signature: refuse rather than degrade to "any signature
		// passes."
		return false
	}
	const prefix = "sha256="
	sigHex, ok := strings.CutPrefix(header, prefix)
	if !ok {
		return false
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.cfg.Secret)
	mac.Write(body) // hash.Hash.Write never returns an error.
	expected := mac.Sum(nil)
	return hmac.Equal(sig, expected)
}

// cloneAndCheckout fetches repoURL to a new temp directory and checks out
// sha specifically, returning the directory and a cleanup func that
// removes it. Callers must call cleanup once done with the directory,
// deploy attempt succeeded or failed, so repeated webhook calls don't
// leak disk.
//
// This does a full clone rather than a shallow (depth 1) one. A depth-1
// clone only fetches the tip of history for whatever ref it targets, and
// the pushed SHA is not guaranteed to still be that tip by the time this
// runs (a later push, or a fetch that lands mid-force-push, can move it),
// so a shallow clone can fail to find the commit at all. A full clone
// costs more bandwidth per webhook call, but guarantees the target commit
// is reachable without needing ref-guessing or retry logic. Fetching just
// the one commit (a targeted `git fetch <sha>`, when the remote and
// protocol support it) would avoid that cost, but adds real complexity
// (not every remote allows fetching arbitrary SHAs) for a tradeoff that
// doesn't matter yet at Phase 1's single-app, single-repo scale. Revisit
// if a real repo's clone time becomes a measured problem.
func cloneAndCheckout(ctx context.Context, repoURL, sha string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "levelrail-webhook-*")
	if err != nil {
		return "", nil, fmt.Errorf("webhook: create temp checkout dir: %w", err)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			slog.Default().Error("webhook: failed to remove temp checkout dir", "dir", dir, "error", rmErr)
		}
	}

	repo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL: repoURL,
	})
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("webhook: clone %q: %w", repoURL, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("webhook: get worktree for %q: %w", repoURL, err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(sha)}); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("webhook: checkout %q at %q: %w", repoURL, sha, err)
	}

	return dir, cleanup, nil
}
