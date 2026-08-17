package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// GitSourceStore is the store surface the git source handlers need.
// *store.DB satisfies this structurally.
type GitSourceStore interface {
	SaveGitSource(ctx context.Context, g store.GitSource) error
	GetGitSource(ctx context.Context, serviceName string) (*store.GitSource, error)
	DeleteGitSource(ctx context.Context, serviceName string) error
}

// GitSourceSecrets is the surface a git source's connect flow and the
// git-push webhook route (git_webhook.go) both need from
// internal/secrets.Manager: unlike SecretSetter and BackupSecretsSetter
// (write-only, a secret's value never resolved back through internal/api
// at all), this package genuinely does call Resolve, but only for its
// own server-side use, HMAC-verifying an incoming webhook and
// authenticating a git clone, never to populate an HTTP response body.
// See handleSetGitSource's and handleGitPushWebhook's own doc comments
// for that boundary; *secrets.Manager satisfies this structurally.
type GitSourceSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// gitSourceTokenKey and gitSourceSecretKey are the internal/secrets
// envKeys a git source's two credentials are stored under, within the
// DEK store.GitSourceSecretsKey(name) namespaces (that function's own
// doc comment covers why this is a distinct namespace from the app's
// real runtime env secrets). Mirrors backup_targets.go's own literal
// "access_key_id"/"secret_access_key" convention: named constants here
// rather than inline strings only because both are referenced from two
// files (this one and git_webhook.go).
const (
	gitSourceTokenKey  = "deploy_token"
	gitSourceSecretKey = "webhook_secret"
)

// gitSourceResource is the wire shape for a connected git source.
// WebhookSecret is the one field that's ever populated on the way out: a
// generated-at-connect-time value, present only in handleSetGitSource's
// own create-time response (never on GET, never on an update PUT), the
// same "shown once, at mint time" shape handleCreateNodeJoinToken's own
// plaintext token response already establishes for a different
// generated-once credential in this codebase. Unlike a node join token,
// this value is also kept (envelope-encrypted) for later use, since this
// package must go on verifying it against every subsequent push; a join
// token by contrast only ever needs a hash, because it's checked once,
// at exchange time, not on an ongoing basis.
type gitSourceResource struct {
	ServiceName   string    `json:"service_name"`
	RepoURL       string    `json:"repo_url"`
	Branch        string    `json:"branch"`
	BuildType     string    `json:"build_type"`
	BuildPath     string    `json:"build_path,omitempty"`
	HasToken      bool      `json:"has_token"`
	WebhookURL    string    `json:"webhook_url"`
	WebhookSecret string    `json:"webhook_secret,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// gitSourceWebhookPath is the relative API path GitHub's own webhook
// config points at for name: POST /api/v1/webhooks/github/{name}
// (router.go). Relative, not absolute: this server has no reliable way
// to know its own externally-reachable hostname (see CLAUDE.md section
// 3's brand indirection: even the product's own name isn't hardcoded
// here), so the frontend prepends window.location.origin for display,
// the same "server stays dumb about its own external address" choice
// DatabasePublicAccessCard.tsx's connectionString makes with
// window.location.hostname.
func gitSourceWebhookPath(name string) string {
	return "/api/v1/webhooks/github/" + name
}

func toGitSourceResource(g store.GitSource, hasToken bool) gitSourceResource {
	return gitSourceResource{
		ServiceName: g.ServiceName,
		RepoURL:     g.RepoURL,
		Branch:      g.Branch,
		BuildType:   g.BuildType,
		BuildPath:   g.BuildPath,
		HasToken:    hasToken,
		WebhookURL:  gitSourceWebhookPath(g.ServiceName),
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// setGitSourceRequest is PUT /api/v1/apps/{name}/git-source's body.
type setGitSourceRequest struct {
	RepoURL string `json:"repo_url"`
	// Branch defaults to webhook.DefaultBranch when empty.
	Branch string `json:"branch,omitempty"`
	// BuildType defaults to spec.BuildDockerfile when empty; must be one
	// of dockerfile, railpack, or static. build.type: image is rejected
	// (normalizeGitSourceBuildType): a connected git source redeploys on
	// every push, but an image-type service has no build step to redeploy
	// with, only a fixed, already-pushed reference that a new commit
	// can't change, so pinning one here would silently do nothing useful
	// on the next push.
	BuildType string `json:"build_type,omitempty"`
	BuildPath string `json:"build_path,omitempty"`
	// Token is an optional git hosting personal access token for a
	// private repo, sent over HTTPS Basic auth as the password
	// (gitCheckoutWithToken, git_webhook.go). Empty on an update means
	// "leave the currently stored token, if any, unchanged"; there is no
	// way to explicitly clear a previously-set token short of
	// disconnecting and reconnecting the source (DELETE then PUT), the
	// same "known gap, left honest" shape handleDeleteBackupTarget's own
	// doc comment already accepts for secret deletion in this codebase:
	// internal/secrets.Manager has no delete operation today.
	Token string `json:"token,omitempty"`
}

// normalizeGitSourceBuildType mirrors handleTriggerBuild's own build.type
// validation (builds.go) for the types a git source can meaningfully
// redeploy on every push: empty defaults to dockerfile, compose is
// rejected as not-yet-supported (internal/deploy.Pipeline.Deploy has no
// real case for it), image is rejected because a persistent git source
// has nothing to redeploy for it (see setGitSourceRequest.BuildType's own
// doc comment), anything else unrecognized is a 400. A separate copy
// rather than a shared helper: builds.go's version is embedded directly
// in that handler's larger validation flow, and the two request shapes
// differ (BuildType/BuildPath as top-level fields here, a nested Build
// sub-object there), the same "two fetchFuncs, one per caller"
// duplication builds.go's own gitCheckout doc comment already accepts
// for an analogous pair.
func normalizeGitSourceBuildType(buildType string) (string, error) {
	if buildType == "" {
		return spec.BuildDockerfile, nil
	}
	switch buildType {
	case spec.BuildDockerfile, spec.BuildRailpack, spec.BuildStatic:
		return buildType, nil
	case spec.BuildCompose:
		return "", fmt.Errorf("build_type %q is not yet supported for a git source", buildType)
	case spec.BuildImage:
		return "", fmt.Errorf("build_type %q is not supported for a git source: a pinned image has nothing to rebuild on push, use POST /api/v1/apps/{name}/deploys to redeploy it directly", buildType)
	default:
		return "", fmt.Errorf("build_type %q is not recognized", buildType)
	}
}

// requireHTTPOrHTTPSScheme rejects any repoURL scheme go-git's client
// registry would otherwise happily dial (notably "file", per
// errRepoURLSchemeNotAllowed's own doc comment in git_branches.go): a
// connected git source is control-plane-initiated I/O (both the clone
// this package performs on every matching push, and go-git's registry
// dependency this scheme check exists to guard against) the same way
// listRemoteBranches's own repoURL already is, so it gets the identical
// restriction, reusing that function's own sentinel error.
func requireHTTPOrHTTPSScheme(repoURL string) error {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return fmt.Errorf("api: parse repo_url %q: %w", repoURL, err)
	}
	switch parsed.Scheme {
	case "http", "https":
		return nil
	default:
		return errRepoURLSchemeNotAllowed
	}
}

// randomWebhookSecret generates a 256-bit HMAC key, URL-safe base64
// encoded (44 characters): sized for HMAC-SHA256, the same algorithm
// internal/webhook.VerifySignature (and this package's own
// handleGitPushWebhook) verifies incoming pushes against.
func randomWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate webhook secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// handleGetGitSource handles GET /api/v1/apps/{name}/git-source.
func (rt *Router) handleGetGitSource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.logger.Error("api: get git source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var hasToken bool
	if rt.gitSourceSecrets != nil {
		hasToken, err = rt.gitSourceSecrets.Exists(r.Context(), store.GitSourceSecretsKey(name), gitSourceTokenKey)
		if err != nil {
			rt.logger.Error("api: get git source: check token failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, toGitSourceResource(*gs, hasToken))
}

// handleSetGitSource handles PUT /api/v1/apps/{name}/git-source: connect
// a repo (creating a new webhook secret) the first time it's called for
// an app, or edit the connection (repo_url/branch/build config, and
// optionally rotate the deploy token) on every call after. The webhook
// secret itself is never regenerated by an update: GitHub's own
// already-configured webhook still needs to keep matching the value this
// handler returned the one time it was generated, so silently rotating
// it on an unrelated field edit would break a working webhook out from
// under the operator. Response body: WebhookSecret is only ever
// populated on the create path (see gitSourceResource's own doc comment
// for why that's a deliberate, one-time exception to this package's
// general "never decrypt a secret back into a response" rule, the same
// rule handleSetSecret's own doc comment establishes for app env
// secrets).
func (rt *Router) handleSetGitSource(w http.ResponseWriter, r *http.Request) {
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "git sources are not configured on this control plane (no master key set)")
		return
	}

	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set git source: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setGitSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RepoURL == "" {
		writeError(w, http.StatusBadRequest, "repo_url is required")
		return
	}
	if err := requireHTTPOrHTTPSScheme(req.RepoURL); err != nil {
		writeError(w, http.StatusBadRequest, "repo_url must use http or https")
		return
	}
	buildType, err := normalizeGitSourceBuildType(req.BuildType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if buildType == spec.BuildRailpack && req.BuildPath != "" {
		writeError(w, http.StatusBadRequest, "build_path is not meaningful for build_type \"railpack\"")
		return
	}
	branch := req.Branch
	if branch == "" {
		branch = webhook.DefaultBranch
	}

	key := store.GitSourceSecretsKey(name)

	_, err = rt.gitSources.GetGitSource(r.Context(), name)
	creating := errors.Is(err, store.ErrGitSourceNotFound)
	if err != nil && !creating {
		rt.logger.Error("api: set git source: load existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var webhookSecretPlain string
	if creating {
		webhookSecretPlain, err = randomWebhookSecret()
		if err != nil {
			rt.logger.Error("api: set git source: generate webhook secret failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if err := rt.gitSourceSecrets.SetValue(r.Context(), key, gitSourceSecretKey, webhookSecretPlain); err != nil {
			rt.logger.Error("api: set git source: save webhook secret failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if req.Token != "" {
		if err := rt.gitSourceSecrets.SetValue(r.Context(), key, gitSourceTokenKey, req.Token); err != nil {
			rt.logger.Error("api: set git source: save deploy token failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.gitSources.SaveGitSource(r.Context(), store.GitSource{
		ServiceName: name, RepoURL: req.RepoURL, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath,
	}); err != nil {
		rt.logger.Error("api: set git source: save failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	saved, err := rt.gitSources.GetGitSource(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set git source: reload failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	hasToken, err := rt.gitSourceSecrets.Exists(r.Context(), key, gitSourceTokenKey)
	if err != nil {
		rt.logger.Error("api: set git source: check token failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resource := toGitSourceResource(*saved, hasToken)
	status := http.StatusOK
	if creating {
		status = http.StatusCreated
		resource.WebhookSecret = webhookSecretPlain
	}
	rt.logger.Info("api: git source connected", slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.Bool("creating", creating))
	writeJSON(w, status, resource)
}

// handleDeleteGitSource handles DELETE /api/v1/apps/{name}/git-source.
// Known gap, matching handleDeleteBackupTarget's own documented one: see
// store.DeleteGitSource's doc comment, the deploy token and webhook
// secret this source's connect flow wrote remain in the secrets store,
// unreferenced but not erased at rest.
func (rt *Router) handleDeleteGitSource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	err := rt.gitSources.DeleteGitSource(r.Context(), name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete git source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
