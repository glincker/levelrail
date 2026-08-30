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
	// SetGitSourcePreviewEnabled backs PUT
	// /api/v1/apps/{name}/preview-settings (preview_environments_handlers.go):
	// the opt-in toggle for preview environments per pull request.
	SetGitSourcePreviewEnabled(ctx context.Context, serviceName string, enabled bool) error
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
	ServiceName        string                          `json:"service_name"`
	RepoURL            string                          `json:"repo_url"`
	Branch             string                          `json:"branch"`
	BuildType          string                          `json:"build_type"`
	BuildPath          string                          `json:"build_path,omitempty"`
	AdditionalServices map[string]store.GitSourceBuild `json:"additional_services,omitempty"`
	// Services is an app.yaml-style services: map (store.GitSource.Services's
	// own doc comment): when set, a push fans out through the same
	// deploy.Pipeline.DeploySpec logic POST .../deploy-spec uses, instead
	// of the AdditionalServices walk. Mutually exclusive with
	// AdditionalServices, see validateGitSourceServices.
	Services      map[string]spec.Service `json:"services,omitempty"`
	HasToken      bool                    `json:"has_token"`
	WebhookURL    string                  `json:"webhook_url"`
	WebhookSecret string                  `json:"webhook_secret,omitempty"`
	// PreviewEnabled mirrors store.GitSource.PreviewEnabled: read-only
	// here, set via PUT /api/v1/apps/{name}/preview-settings
	// (preview_environments_handlers.go), not this resource's own PUT.
	PreviewEnabled bool      `json:"preview_enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
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
		ServiceName:        g.ServiceName,
		RepoURL:            g.RepoURL,
		Branch:             g.Branch,
		BuildType:          g.BuildType,
		BuildPath:          g.BuildPath,
		AdditionalServices: g.AdditionalServices,
		Services:           g.Services,
		HasToken:           hasToken,
		WebhookURL:         gitSourceWebhookPath(g.ServiceName),
		PreviewEnabled:     g.PreviewEnabled,
		CreatedAt:          g.CreatedAt,
		UpdatedAt:          g.UpdatedAt,
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
	// AdditionalServices lets a push also rebuild sibling services under
	// the same app group (apps_group.go), keyed by the sibling
	// DesiredService's own name. Each sibling must already exist and
	// must not be name itself; see validateAdditionalServices. Rejected
	// together with Services (validateGitSourceServices): once a real
	// services: map is configured, a push fans out through that, and a
	// separate AdditionalServices list alongside it would just be an
	// ambiguous second fan-out for the same push.
	AdditionalServices map[string]store.GitSourceBuild `json:"additional_services,omitempty"`
	// Services is an app.yaml-style services: map (store.GitSource.Services's
	// own doc comment): once set, a push routes through the same
	// deploy.Pipeline.DeploySpec fan-out POST .../deploy-spec already
	// uses (handleGitPushWebhook), instead of the single-service Deploy
	// plus AdditionalServices walk. Validated the same way
	// handleDeploySpec validates its own services map
	// (validateDeploySpecServiceTypes).
	Services map[string]spec.Service `json:"services,omitempty"`
}

// validateAdditionalServices rejects a self-reference (name fanning out
// to itself, which would double-deploy it) and any build_type this git
// source can't handle, then normalizes each entry's build_type the same
// way the primary service's own already does. It does not require each
// sibling to already exist as a store.App-linked service: an operator
// may reasonably connect the fan-out before the sibling's own first
// deploy, and handleGitPushWebhook already treats a missing sibling as a
// per-service failure at push time, not a connect-time error.
func validateAdditionalServices(name string, additional map[string]store.GitSourceBuild) (map[string]store.GitSourceBuild, error) {
	if len(additional) == 0 {
		return nil, nil
	}
	out := make(map[string]store.GitSourceBuild, len(additional))
	for svcName, build := range additional {
		if svcName == name {
			return nil, fmt.Errorf("additional_services: %q cannot list itself", svcName)
		}
		buildType, err := normalizeGitSourceBuildType(build.BuildType)
		if err != nil {
			return nil, fmt.Errorf("additional_services[%q]: %w", svcName, err)
		}
		out[svcName] = store.GitSourceBuild{BuildType: buildType, BuildPath: build.BuildPath}
	}
	return out, nil
}

// validateGitSourceServices rejects services alongside a non-empty
// additional, since routing a push through both DeploySpec's fan-out and
// the AdditionalServices walk for the same push would double-deploy
// whatever service key happens to be named in both, and validates each
// service's own build.type the same way handleDeploySpec does
// (validateDeploySpecServiceTypes, apps_multi.go), so a persisted
// services: map can never contain a build.type internal/deploy.Pipeline
// can't handle either.
func validateGitSourceServices(services map[string]spec.Service, additional map[string]store.GitSourceBuild) error {
	if len(services) == 0 {
		return nil
	}
	if len(additional) > 0 {
		return errors.New("services and additional_services cannot both be set: a services map already fans out every service on push")
	}
	return validateDeploySpecServiceTypes(services)
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
		return "", fmt.Errorf("build_type %q is not supported for a git source: a compose file always declares its own set of services, which can only be expanded into a real multi-service deploy (POST /api/v1/apps/{name}/deploy-spec), never a single one this source's own webhook rebuilds", buildType)
	case spec.BuildImage:
		return "", fmt.Errorf("build_type %q is not supported for a git source: a pinned image has nothing to rebuild on push, use POST /api/v1/apps/{name}/deploys to redeploy it directly", buildType)
	default:
		return "", fmt.Errorf("build_type %q is not recognized", buildType)
	}
}

// useRepoAsSourceRequest is the request body every provider's own
// use-as-source handler decodes (GitHub, GitLab, Bitbucket): which app
// the picked repo becomes the connected git source for, plus the same
// optional build configuration handleSetGitSource's own request
// accepts. One shared type rather than a separate, structurally
// identical one per provider: the URL's own path parameters
// (owner/repo, project id, workspace/repoSlug) identify which repo,
// never this body, so the body shape has never actually differed.
type useRepoAsSourceRequest struct {
	AppName   string `json:"app_name"`
	Branch    string `json:"branch,omitempty"`
	BuildType string `json:"build_type,omitempty"`
	BuildPath string `json:"build_path,omitempty"`
}

// decodeUseAsSourceRequest is the common preamble every provider's own
// use-as-source handler needs before its own provider-specific repo
// lookup: decode the body, validate app_name and build_type/build_path,
// and confirm the target app exists. ok is false once this has already
// written an HTTP error response, in which case the caller must return
// immediately without writing anything else.
func (rt *Router) decodeUseAsSourceRequest(w http.ResponseWriter, r *http.Request, logPrefix string) (req useRepoAsSourceRequest, buildType string, ok bool) {
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, "", false
	}
	if req.AppName == "" {
		writeError(w, http.StatusBadRequest, "app_name is required")
		return req, "", false
	}
	buildType, err := normalizeGitSourceBuildType(req.BuildType)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return req, "", false
	}
	if buildType == "railpack" && req.BuildPath != "" {
		writeError(w, http.StatusBadRequest, "build_path is not meaningful for build_type \"railpack\"")
		return req, "", false
	}

	if _, err := rt.apps.GetDesiredService(r.Context(), req.AppName); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return req, "", false
	} else if err != nil {
		rt.logger.Error(logPrefix+": load app failed", slog.String("error", err.Error()), slog.String("app_name", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return req, "", false
	}

	return req, buildType, true
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
	additionalServices, err := validateAdditionalServices(name, req.AdditionalServices)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateGitSourceServices(req.Services, additionalServices); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	result, err := rt.connectGitSource(r.Context(), name, connectGitSourceParams{
		RepoURL: req.RepoURL, Branch: branch, BuildType: buildType, BuildPath: req.BuildPath, Token: req.Token,
		AdditionalServices: additionalServices, Services: req.Services,
	})
	if err != nil {
		rt.logger.Error("api: set git source failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	status := http.StatusOK
	if result.Creating {
		status = http.StatusCreated
	}
	rt.logger.Info("api: git source connected", slog.String("name", name), slog.String("repo_url", req.RepoURL), slog.Bool("creating", result.Creating))
	writeJSON(w, status, result.Resource)
}

// connectGitSourceParams is connectGitSource's already-validated input:
// every field a caller derives its own way (a hand-typed form for
// handleSetGitSource, a picked GitLab project for
// handleUseGitLabProjectAsSource) before reaching this shared step.
type connectGitSourceParams struct {
	RepoURL   string
	Branch    string
	BuildType string
	BuildPath string
	// Token is optional; empty means "leave whatever is currently
	// stored unchanged" on an update, matching setGitSourceRequest's own
	// Token field.
	Token              string
	AdditionalServices map[string]store.GitSourceBuild
	Services           map[string]spec.Service
}

// connectGitSourceResult is connectGitSource's return: the resource to
// send back (WebhookSecret populated only when Creating is true) plus
// whether this call created a new git source or updated an existing one,
// so each caller can pick its own HTTP status for that distinction.
type connectGitSourceResult struct {
	Resource gitSourceResource
	Creating bool
}

// connectGitSource is the one place a repository actually becomes an
// app's connected git source: generates a webhook secret on first
// connect, stores the optional deploy token, and persists the
// store.GitSource row. handleSetGitSource (the manual connect form) and
// handleUseGitLabProjectAsSource (GitLab's "use project as source"
// action) both call this rather than each reimplementing the connect
// logic, so a GitLab-originated source ends up in exactly the same
// store.GitSource shape, behind the exact same webhook receiver, as a
// hand-typed one.
func (rt *Router) connectGitSource(ctx context.Context, name string, p connectGitSourceParams) (connectGitSourceResult, error) {
	key := store.GitSourceSecretsKey(name)

	_, err := rt.gitSources.GetGitSource(ctx, name)
	creating := errors.Is(err, store.ErrGitSourceNotFound)
	if err != nil && !creating {
		return connectGitSourceResult{}, fmt.Errorf("load existing git source: %w", err)
	}

	var webhookSecretPlain string
	if creating {
		webhookSecretPlain, err = randomWebhookSecret()
		if err != nil {
			return connectGitSourceResult{}, fmt.Errorf("generate webhook secret: %w", err)
		}
		if err := rt.gitSourceSecrets.SetValue(ctx, key, gitSourceSecretKey, webhookSecretPlain); err != nil {
			return connectGitSourceResult{}, fmt.Errorf("save webhook secret: %w", err)
		}
	}
	if p.Token != "" {
		if err := rt.gitSourceSecrets.SetValue(ctx, key, gitSourceTokenKey, p.Token); err != nil {
			return connectGitSourceResult{}, fmt.Errorf("save deploy token: %w", err)
		}
	}

	if err := rt.gitSources.SaveGitSource(ctx, store.GitSource{
		ServiceName: name, RepoURL: p.RepoURL, Branch: p.Branch, BuildType: p.BuildType, BuildPath: p.BuildPath,
		AdditionalServices: p.AdditionalServices, Services: p.Services,
	}); err != nil {
		return connectGitSourceResult{}, fmt.Errorf("save git source: %w", err)
	}

	saved, err := rt.gitSources.GetGitSource(ctx, name)
	if err != nil {
		return connectGitSourceResult{}, fmt.Errorf("reload git source: %w", err)
	}
	hasToken, err := rt.gitSourceSecrets.Exists(ctx, key, gitSourceTokenKey)
	if err != nil {
		return connectGitSourceResult{}, fmt.Errorf("check deploy token: %w", err)
	}

	resource := toGitSourceResource(*saved, hasToken)
	if creating {
		resource.WebhookSecret = webhookSecretPlain
	}
	return connectGitSourceResult{Resource: resource, Creating: creating}, nil
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
