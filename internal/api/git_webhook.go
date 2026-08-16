package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// gitSourceFetchFunc fetches repoURL at the exact commit sha to a local
// directory, authenticating with token when non-empty, returning the
// directory and a cleanup func that removes it. A distinct signature
// from both fetchFunc (builds.go, no credential: a manual build
// trigger's repo_url is hand-typed fresh every call, never stored) and
// internal/webhook's own private fetchFunc (also no credential: the
// original single-app path only ever documented a public repo): a
// connected git source is the first path in this codebase that can
// target a private repo, so it's the first one that needs to carry a
// token through to the actual clone.
type gitSourceFetchFunc func(ctx context.Context, repoURL, sha, token string) (dir string, cleanup func(), err error)

// gitCheckoutWithToken is the real gitSourceFetchFunc implementation,
// mirroring internal/webhook's own cloneAndCheckout (full clone, then
// checkout the exact commit hash, not ResolveRevision: see that
// function's own doc comment for why a full clone, not shallow, and
// why an exact hash, not an arbitrary revision, is correct for a
// webhook's pushed SHA specifically). The one real difference: when
// token is non-empty, the clone authenticates over HTTPS Basic auth,
// GitHub's (and GitLab's, and Bitbucket's) own documented scheme for a
// personal access token: any non-empty username, the token itself as
// the password.
func gitCheckoutWithToken(ctx context.Context, repoURL, sha, token string) (dir string, cleanup func(), err error) {
	dir, err = os.MkdirTemp("", "levelrail-git-source-*")
	if err != nil {
		return "", nil, fmt.Errorf("api: create temp checkout dir: %w", err)
	}
	cleanup = func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			slog.Default().Error("api: failed to remove temp checkout dir", slog.String("dir", dir), slog.String("error", rmErr.Error()))
		}
	}

	cloneOpts := &git.CloneOptions{URL: repoURL}
	if token != "" {
		cloneOpts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: token}
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, cloneOpts)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: clone %q: %w", repoURL, err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: get worktree for %q: %w", repoURL, err)
	}

	if err := wt.Checkout(&git.CheckoutOptions{Hash: plumbing.NewHash(sha)}); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("api: checkout %q at %q: %w", repoURL, sha, err)
	}

	return dir, cleanup, nil
}

// handleGitPushWebhook handles POST /api/v1/webhooks/github/{name}: the
// multi-app-aware evolution of internal/webhook.Handler (see that
// package's own doc comment). Registered directly in internal/api,
// rather than as a per-app Config resolved into internal/webhook.Handler
// itself, because reconstructing a full spec.Service from a stored app
// (specServiceFromDesired, builds.go) already lives in this package for
// the identical reason handleTriggerBuild needs it, and duplicating that
// reconstruction into internal/webhook would be exactly the kind of
// speculative parallel implementation the project's engineering
// standards warn against. Reuses internal/webhook's own verified
// signature/payload logic (VerifySignature, ParsePushEvent,
// MaxPayloadBytes, Config.TargetRef) rather than re-implementing any of
// it, and reuses this package's own beginBuildDeployAttempt (deploy_attempts.go,
// generalized to take a source parameter for exactly this second
// caller) so a webhook-triggered build gets the identical deploy-attempt
// history and log persistence a manual build trigger already gets.
//
// Unauthenticated by design (router.go's own route registration
// comment): GitHub cannot present a session or API token, so this
// route's HMAC signature check, against this app's own per-app secret
// generated at connect time and internal/secrets-encrypted at rest, is
// what stands in for auth.
//
// 404, not 501, when no git source is connected for name: unlike a
// control-plane-wide "not configured" gap (no master key set, checked
// first, below), a specific app simply not having a connected source is
// exactly the shape GET/PUT/DELETE .../git-source already treat as 404,
// and matches this route's own path shape (an app-scoped resource that
// doesn't exist for this name) better than a blanket 501 would.
func (rt *Router) handleGitPushWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.logger.Error("api: git push webhook: load git source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	secretsKey := store.GitSourceSecretsKey(name)
	secret, err := rt.gitSourceSecrets.Resolve(r.Context(), secretsKey, gitSourceSecretKey)
	if err != nil {
		rt.logger.Error("api: git push webhook: resolve webhook secret failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, webhook.MaxPayloadBytes+1))
	if err != nil {
		rt.logger.Warn("api: git push webhook: read request body failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}
	if len(body) > webhook.MaxPayloadBytes {
		rt.logger.Warn("api: git push webhook: request body exceeds max payload size", slog.String("name", name), slog.Int("max_bytes", webhook.MaxPayloadBytes))
		writeError(w, http.StatusBadRequest, "request body too large")
		return
	}

	if !webhook.VerifySignature([]byte(secret), body, r.Header.Get("X-Hub-Signature-256")) {
		rt.logger.Warn("api: git push webhook: signature verification failed", slog.String("name", name), slog.String("remote_addr", r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	ev, err := webhook.ParsePushEvent(body)
	if err != nil {
		rt.logger.Warn("api: git push webhook: malformed payload", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusBadRequest, "malformed payload")
		return
	}

	targetCfg := webhook.Config{Branch: gs.Branch}
	if ev.Ref != targetCfg.TargetRef() {
		rt.logger.Info("api: git push webhook: ignoring push to non-target branch", slog.String("name", name), slog.String("ref", ev.Ref), slog.String("want_ref", targetCfg.TargetRef()))
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, "ignored: push to %q, deploys trigger on %q only\n", ev.Ref, targetCfg.TargetRef()); err != nil {
			rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
		}
		return
	}

	if rt.builder == nil {
		writeError(w, http.StatusNotImplemented, "git push deploys are not configured on this control plane")
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: git push webhook: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	hasToken, err := rt.gitSourceSecrets.Exists(r.Context(), secretsKey, gitSourceTokenKey)
	if err != nil {
		rt.logger.Error("api: git push webhook: check deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	var token string
	if hasToken {
		token, err = rt.gitSourceSecrets.Resolve(r.Context(), secretsKey, gitSourceTokenKey)
		if err != nil {
			rt.logger.Error("api: git push webhook: resolve deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	svcSpec := specServiceFromDesired(*existing, spec.Build{Type: gs.BuildType, Path: gs.BuildPath})

	sourceDir, cleanup, err := rt.gitSourceFetch(r.Context(), gs.RepoURL, ev.After, token)
	if err != nil {
		rt.logger.Error("api: git push webhook: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("commit", ev.After))
		writeError(w, http.StatusInternalServerError, "deploy failed")
		return
	}
	defer cleanup()

	buildReq := deploy.Request{
		ServiceName: name,
		Service:     svcSpec,
		SourceDir:   sourceDir,
		CommitSHA:   ev.After,
		ImageRepo:   name,
	}

	_, progress, finishAttempt := rt.beginBuildDeployAttempt(r.Context(), buildReq, store.DeployAttemptSourceWebhook)
	tag, err := rt.builder.Deploy(r.Context(), buildReq, progress)
	finishAttempt(err)
	if err != nil {
		rt.logger.Error("api: git push webhook: deploy failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("commit", ev.After))
		writeError(w, http.StatusInternalServerError, "deploy failed")
		return
	}

	rt.logger.Info("api: git push webhook: deploy triggered", slog.String("name", name), slog.String("commit", ev.After), slog.String("tag", tag))
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, "deploy triggered: %s\n", tag); err != nil {
		rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
	}
}
