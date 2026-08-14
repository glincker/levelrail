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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
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
	log      *slog.Logger
	fetch    fetchFunc
}

// New builds a Handler. log defaults to slog.Default() if nil.
func New(cfg Config, deployer Deployer, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{cfg: cfg, deployer: deployer, log: log, fetch: cloneAndCheckout}
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

	tag, err := h.deployer.Deploy(r.Context(), req, build.SlogProgress(h.log))
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
