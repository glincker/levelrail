package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"

	"github.com/GLINCKER/levelrail/internal/build"
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
// comment): neither GitHub nor GitLab can present a session or API
// token, so this route's own request verification (GitHub's HMAC
// signature or GitLab's plain secret token, see
// verifyGitPushWebhookAuth), against this app's own per-app secret
// generated at connect time and internal/secrets-encrypted at rest, is
// what stands in for auth. The URL still says "github" for backward
// compatibility with already-configured GitHub webhooks; GitLab's own
// project webhook (internal/api/gitlab_app_projects.go) points at this
// exact same path.
//
// 404, not 501, when no git source is connected for name: unlike a
// control-plane-wide "not configured" gap (no master key set, checked
// first, below), a specific app simply not having a connected source is
// exactly the shape GET/PUT/DELETE .../git-source already treat as 404,
// and matches this route's own path shape (an app-scoped resource that
// doesn't exist for this name) better than a blanket 501 would.
//
// A connected source with a persisted services: map (gs.Services) fans
// out through deployServicesSpecFanout, the same deploy.Pipeline.DeploySpec
// path POST .../deploy-spec uses; one with none falls through to the
// single-service deploy plus the older deployAdditionalServices walk
// below, unchanged.
func (rt *Router) handleGitPushWebhook(w http.ResponseWriter, r *http.Request) {
	// Every success/partial-failure response below is a plain status
	// line that can embed webhook-payload fields a pusher or PR author
	// controls (a branch name, a PR's base ref): an explicit text/plain
	// content type stops any client from ever MIME-sniffing one of
	// those bodies as HTML. writeError's own writeJSON call overrides
	// this to application/json for the error paths below, which is
	// correct: this only needs to cover the plain-text success paths.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

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

	if !verifyGitPushWebhookAuth(secret, body, r.Header) {
		rt.logger.Warn("api: git push webhook: signature verification failed", slog.String("name", name), slog.String("remote_addr", r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "invalid signature")
		return
	}

	if webhook.IsPullRequestEvent(r.Header) {
		prEv, err := webhook.ParsePullRequestEventForProvider(body, r.Header)
		if err != nil {
			rt.logger.Warn("api: git push webhook: malformed pull request payload", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusBadRequest, "malformed payload")
			return
		}
		status, message := rt.handlePullRequestWebhookEvent(r.Context(), name, *gs, prEv)
		w.WriteHeader(status)
		if _, err := fmt.Fprint(w, message); err != nil { //nolint:gosec // gosec's taint check can't see the text/plain Content-Type set above this function's entry, which is the actual mitigation for a client ever interpreting message's webhook-payload-derived fields as markup
			rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
		}
		return
	}

	ev, err := webhook.ParsePushEventForProvider(body, r.Header.Get("X-Event-Key"))
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

	token, err := rt.resolveGitSourceDeployToken(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: git push webhook: resolve deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	sourceDir, cleanup, err := rt.gitSourceFetch(r.Context(), gs.RepoURL, ev.After, token)
	if err != nil {
		rt.logger.Error("api: git push webhook: fetch source failed", slog.String("error", err.Error()), slog.String("name", name), slog.String("commit", ev.After))
		writeError(w, http.StatusInternalServerError, "deploy failed")
		return
	}
	defer cleanup()

	// A persisted services: map (store.GitSource.Services's own doc
	// comment) routes this push through the same deploy.Pipeline.DeploySpec
	// fan-out POST .../deploy-spec already uses, instead of the
	// single-service Deploy plus AdditionalServices walk below: the two
	// are mutually exclusive by construction (validateGitSourceServices),
	// so this is the one place that decision is actually made.
	if len(gs.Services) > 0 {
		appName := existing.AppID
		if appName == "" {
			appName = name
		}
		failed := rt.deployServicesSpecFanout(r.Context(), appName, *gs, sourceDir, ev.After)
		status := http.StatusOK
		if failed > 0 {
			status = http.StatusMultiStatus
		}
		w.WriteHeader(status)
		if _, err := fmt.Fprintf(w, "services deploy triggered for app %q (%d service(s) failed)\n", appName, failed); err != nil {
			rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
		}
		return
	}

	svcSpec := specServiceFromDesired(*existing, spec.Build{Type: gs.BuildType, Path: gs.BuildPath})

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

	additionalFailed := rt.deployAdditionalServices(r.Context(), gs.AdditionalServices, sourceDir, ev.After)

	status := http.StatusOK
	if additionalFailed > 0 {
		status = http.StatusMultiStatus
	}
	w.WriteHeader(status)
	if _, err := fmt.Fprintf(w, "deploy triggered: %s (%d additional service(s) failed)\n", tag, additionalFailed); err != nil {
		rt.logger.Warn("api: git push webhook: failed to write response body", slog.String("error", err.Error()), slog.String("name", name))
	}
}

// resolveGitSourceDeployToken returns name's connected git source's
// deploy token, or "" if none was ever set (the ordinary public-repo
// case): shared by handleGitPushWebhook's own push path and
// deployPreviewEnvironment (preview_environments.go), so both resolve a
// private-repo credential exactly the same way.
func (rt *Router) resolveGitSourceDeployToken(ctx context.Context, name string) (string, error) {
	secretsKey := store.GitSourceSecretsKey(name)
	hasToken, err := rt.gitSourceSecrets.Exists(ctx, secretsKey, gitSourceTokenKey)
	if err != nil {
		return "", fmt.Errorf("check deploy token: %w", err)
	}
	if !hasToken {
		return "", nil
	}
	token, err := rt.gitSourceSecrets.Resolve(ctx, secretsKey, gitSourceTokenKey)
	if err != nil {
		return "", fmt.Errorf("resolve deploy token: %w", err)
	}
	return token, nil
}

// deployAdditionalServices rebuilds and redeploys each sibling service in
// additional from the same checkout and commit already fetched for the
// pushed app, returning how many failed. One sibling's build failure
// (e.g. it has since been deleted, or its Dockerfile path is stale) does
// not block any other sibling, matching DeploySpec's own "one broken
// resource must not block the others" principle (internal/deploy/multi.go).
func (rt *Router) deployAdditionalServices(ctx context.Context, additional map[string]store.GitSourceBuild, sourceDir, commitSHA string) int {
	failed := 0
	for svcName, buildCfg := range additional {
		svc, err := rt.apps.GetDesiredService(ctx, svcName)
		if err != nil {
			rt.logger.Error("api: git push webhook: additional service: load failed", slog.String("service", svcName), slog.String("error", err.Error()))
			failed++
			continue
		}

		svcSpec := specServiceFromDesired(*svc, spec.Build{Type: buildCfg.BuildType, Path: buildCfg.BuildPath})
		req := deploy.Request{
			ServiceName: svcName,
			Service:     svcSpec,
			SourceDir:   sourceDir,
			CommitSHA:   commitSHA,
			ImageRepo:   svcName,
		}

		_, progress, finishAttempt := rt.beginBuildDeployAttempt(ctx, req, store.DeployAttemptSourceWebhook)
		tag, err := rt.builder.Deploy(ctx, req, progress)
		finishAttempt(err)
		if err != nil {
			rt.logger.Error("api: git push webhook: additional service: deploy failed", slog.String("service", svcName), slog.String("error", err.Error()))
			failed++
			continue
		}
		rt.logger.Info("api: git push webhook: additional service: deploy triggered", slog.String("service", svcName), slog.String("tag", tag))
	}
	return failed
}

// deployServicesSpecFanout routes a webhook-triggered push through
// deploy.Pipeline.DeploySpec, the same fan-out handleDeploySpec
// (apps_multi.go) already uses for a manual/API-triggered multi-service
// deploy, so gs.Services and a hand-submitted deploy-spec request never
// diverge again (docs/roadmap.md's own previously-named gap). appName is
// resolved by the caller (existing.AppID, falling back to the git
// source's own service name), matching DeploySpec's own ensureApp
// default so a push against an already-fanned-out app reuses the same
// store.App and the same "<appName>-<key>" service names, rather than
// minting a second, parallel app group. Returns how many service keys
// failed, the same shape deployAdditionalServices returns, so the caller
// can pick 200 vs 207 the identical way.
func (rt *Router) deployServicesSpecFanout(ctx context.Context, appName string, gs store.GitSource, sourceDir, commitSHA string) int {
	progress := func(serviceKey string, ev build.ProgressEvent) {
		rt.logger.Info("api: git push webhook: services fan-out build progress", slog.String("app_name", appName), slog.String("service_key", serviceKey), slog.String("step", ev.Step), slog.Bool("completed", ev.Completed), slog.String("error", ev.Error))
	}

	outcomes, err := rt.builder.DeploySpec(ctx, deploy.MultiRequest{
		AppName:       appName,
		Services:      gs.Services,
		SourceDir:     sourceDir,
		CommitSHA:     commitSHA,
		ImageRepoBase: appName,
	}, progress)
	if err != nil {
		rt.logger.Error("api: git push webhook: services fan-out failed", slog.String("app_name", appName), slog.String("error", err.Error()))
		return len(gs.Services)
	}

	failed := 0
	for _, o := range outcomes {
		if o.Err != nil {
			failed++
			rt.logger.Error("api: git push webhook: services fan-out: service failed", slog.String("app_name", appName), slog.String("service_key", o.ServiceKey), slog.String("error", o.Err.Error()))
			continue
		}
		rt.logger.Info("api: git push webhook: services fan-out: service deployed", slog.String("app_name", appName), slog.String("service_key", o.ServiceKey), slog.String("image", o.Image))
	}
	return failed
}

// verifyGitPushWebhookAuth checks an incoming push webhook request
// against secret, whichever of GitHub's, GitLab's, or Bitbucket's own
// scheme the request's headers indicate. GitHub signs the body
// (X-Hub-Signature-256, an HMAC-SHA256 hex digest,
// webhook.VerifySignature); GitLab instead sends the configured secret
// back verbatim (X-Gitlab-Token, checked with a constant-time comparison
// since it's a direct secret comparison, not a signature); Bitbucket
// signs the body the same HMAC-SHA256 way GitHub does, over the
// identical "sha256=<hex>" wire format, just under a header name with
// no "-256" suffix (X-Hub-Signature), so webhook.VerifySignature's own
// parsing is reused unchanged for it. No known-header case present, or
// a mismatch, fails closed.
func verifyGitPushWebhookAuth(secret string, body []byte, header http.Header) bool {
	if token := header.Get("X-Gitlab-Token"); token != "" {
		return secret != "" && subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
	}
	if sig := header.Get("X-Hub-Signature-256"); sig != "" {
		return webhook.VerifySignature([]byte(secret), body, sig)
	}
	return webhook.VerifySignature([]byte(secret), body, header.Get("X-Hub-Signature"))
}
