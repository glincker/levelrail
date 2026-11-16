package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/failure"
	"github.com/GLINCKER/levelrail/internal/registryvendor"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/dockerhub"
)

// Fallbacks for a Router built without runtime settings (unit tests only);
// the registered defaults in runtimesettings win in a running server.
const (
	dockerHubDefaultWarnPercent = 20
	dockerHubDefaultCacheSecs   = 300
	dockerHubDefaultPageSize    = 25
)

// permissionNote is shown with every status: Docker offers no way to read
// a token's permission level and this integration never attempts a write.
const dockerHubPermissionNote = "Docker Hub does not report a token's permission level and no write is ever attempted, so read-only cannot be proven. Use a personal token with Read-only permission or an organization token with the read scopes only."

// DockerHubAccountClient is the credentialed surface of *dockerhub.Client.
// A test double that does not implement it makes these routes answer 501.
type DockerHubAccountClient interface {
	CachedValidate(ctx context.Context, cred dockerhub.Credentials, ttl time.Duration, refresh bool) (dockerhub.CachedValidation, error)
	CachedPullLimits(ctx context.Context, cred dockerhub.Credentials, ttl time.Duration, refresh bool) (dockerhub.PullLimits, time.Time, bool, error)
	ListRepositories(ctx context.Context, cred dockerhub.Credentials, namespace, nameFilter string, page, pageSize int) (dockerhub.RepositoryPage, error)
	ListTagsPage(ctx context.Context, cred dockerhub.Credentials, namespace, repository string, page, pageSize int) (dockerhub.TagPage, error)
	ResolveDigest(ctx context.Context, cred dockerhub.Credentials, namespace, repository, tag string) (dockerhub.Manifest, error)
}

func (rt *Router) dockerHubAccount() DockerHubAccountClient {
	c, _ := rt.dockerHubClient.(DockerHubAccountClient)
	return c
}

func (rt *Router) dockerHubInt(ctx context.Context, key string, fallback int) int {
	if rt.runtimeSettings == nil {
		return fallback
	}
	return rt.runtimeSettings.Limit(ctx, key)
}

func (rt *Router) dockerHubCacheTTL(ctx context.Context) time.Duration {
	return time.Duration(rt.dockerHubInt(ctx, runtimesettings.KeyDockerHubLimitsCacheSecond, dockerHubDefaultCacheSecs)) * time.Second
}

func (rt *Router) dockerHubPageSize(ctx context.Context) int {
	return rt.dockerHubInt(ctx, runtimesettings.KeyDockerHubTagPageSize, dockerHubDefaultPageSize)
}

func (rt *Router) dockerHubUseCredential(ctx context.Context) bool {
	if rt.runtimeSettings == nil {
		return true
	}
	return rt.runtimeSettings.Bool(ctx, runtimesettings.KeyDockerHubUseCredential)
}

// dockerHubCredentialFor resolves a stored Docker Hub credential and its
// token. Callers must write the response themselves when ok is false.
func (rt *Router) dockerHubCredentialFor(w http.ResponseWriter, r *http.Request, id string) (store.RegistryCredential, dockerhub.Credentials, bool) {
	if rt.dockerHubAccount() == nil || rt.registryCredentialSecrets == nil {
		writeError(w, http.StatusNotImplemented, "Docker Hub integration is not configured on this control plane (no master key set)")
		return store.RegistryCredential{}, dockerhub.Credentials{}, false
	}
	cred, err := rt.registryCredentials.GetRegistryCredential(r.Context(), id)
	if errors.Is(err, store.ErrRegistryCredentialNotFound) {
		writeError(w, http.StatusNotFound, "registry credential not found")
		return store.RegistryCredential{}, dockerhub.Credentials{}, false
	}
	if err != nil {
		rt.internalError(w, "api: docker hub: load registry credential failed", err, slog.String("id", id))
		return store.RegistryCredential{}, dockerhub.Credentials{}, false
	}
	if !registryvendor.IsDockerHubHost(cred.RegistryHost) {
		writeError(w, http.StatusBadRequest, "this registry credential is not for Docker Hub")
		return store.RegistryCredential{}, dockerhub.Credentials{}, false
	}
	token, err := rt.registryCredentialSecrets.Resolve(r.Context(), store.RegistryCredentialSecretsKey(id), "password")
	if err != nil {
		rt.internalError(w, "api: docker hub: resolve token failed", err, slog.String("id", id))
		return store.RegistryCredential{}, dockerhub.Credentials{}, false
	}
	return cred, dockerhub.Credentials{Username: cred.Username, Token: token}, true
}

// writeDockerHubError maps a client error to a typed response. The token
// never appears in these messages.
func (rt *Router) writeDockerHubError(w http.ResponseWriter, err error, op string, attrs ...slog.Attr) {
	var rl *dockerhub.RateLimitedError
	fail := func(status int, code, msg string) {
		writeJSON(w, status, &typedError{Error: msg, Code: code, Message: msg, RequestID: w.Header().Get(requestIDHeader), status: status})
	}
	switch {
	case errors.As(err, &rl):
		if secs := int(rl.RetryAfter / time.Second); secs > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(secs))
		}
		fail(http.StatusTooManyRequests, failure.CodeRegistryRateLimited, "Docker Hub rate limit reached. Add or check a Docker Hub token under Integrations, then retry.")
	case errors.Is(err, dockerhub.ErrUnauthorized):
		fail(http.StatusBadGateway, failure.CodeRegistryAuthFailed, "Docker Hub rejected the token. Reconnect it under Integrations.")
	case errors.Is(err, dockerhub.ErrForbidden):
		fail(http.StatusBadGateway, failure.CodeRegistryAuthFailed, "The Docker Hub token lacks the scope this needs (repository list, tag read or image pull).")
	case errors.Is(err, dockerhub.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found on Docker Hub")
	default:
		rt.logger.LogAttrs(context.Background(), slog.LevelError, "api: docker hub request failed", append([]slog.Attr{slog.String("op", op), slog.String("error", err.Error())}, attrs...)...)
		writeError(w, http.StatusBadGateway, "could not reach Docker Hub")
	}
}

type dockerHubLimitsResponse struct {
	CredentialID  string    `json:"credential_id,omitempty"`
	Authenticated bool      `json:"authenticated"`
	Known         bool      `json:"known"`
	Limit         int       `json:"limit"`
	Remaining     int       `json:"remaining"`
	WindowSeconds int       `json:"window_seconds"`
	Source        string    `json:"source"`
	Low           bool      `json:"low"`
	WarnPercent   int       `json:"warn_percent"`
	UseForPulls   bool      `json:"use_for_pulls"`
	CheckedAt     time.Time `json:"checked_at"`
	Cached        bool      `json:"cached"`
}

func (rt *Router) toDockerHubLimits(ctx context.Context, id string, lim dockerhub.PullLimits, at time.Time, cached bool) dockerHubLimitsResponse {
	warn := rt.dockerHubInt(ctx, runtimesettings.KeyDockerHubLimitWarnPercent, dockerHubDefaultWarnPercent)
	return dockerHubLimitsResponse{
		CredentialID: id, Authenticated: lim.Authenticated, Known: lim.Known, Limit: lim.Limit, Remaining: lim.Remaining,
		WindowSeconds: lim.WindowSeconds, Source: lim.Source, WarnPercent: warn, UseForPulls: rt.dockerHubUseCredential(ctx),
		Low:       lim.Known && lim.Limit > 0 && lim.Remaining*100 < warn*lim.Limit,
		CheckedAt: at, Cached: cached,
	}
}

// handleDockerHubLimits handles GET /api/v1/registry-credentials/{id}/limits:
// the pull quota left for this token, read with a HEAD that costs no pull.
// AbilityReadSensitive because the stored token makes an outbound call.
func (rt *Router) handleDockerHubLimits(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	_, cred, ok := rt.dockerHubCredentialFor(w, r, id)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), registryCredentialTestTimeout)
	defer cancel()
	lim, at, cached, err := rt.dockerHubAccount().CachedPullLimits(ctx, cred, rt.dockerHubCacheTTL(ctx), r.URL.Query().Get("refresh") == "true")
	if err != nil {
		rt.writeDockerHubError(w, err, "limits", slog.String("id", id))
		return
	}
	writeJSON(w, http.StatusOK, rt.toDockerHubLimits(ctx, id, lim, at, cached))
}

type dockerHubCapabilityResource struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type dockerHubStatusResponse struct {
	CredentialID string `json:"credential_id,omitempty"`
	Valid        bool   `json:"valid"`
	// TokenKind is "personal" or "organization", inferred from the token prefix.
	TokenKind           string                        `json:"kind"`
	Namespace           string                        `json:"namespace"`
	Message             string                        `json:"message,omitempty"`
	Scopes              []string                      `json:"scopes"`
	Capabilities        []dockerHubCapabilityResource `json:"capabilities"`
	PrivateReposVisible bool                          `json:"private_repos_visible"`
	// ExpiryKnown is false unless the operator recorded an expiry: Docker
	// does not expose a token's expiry.
	ExpiryKnown    bool                     `json:"expiry_known"`
	ExpiresAt      *time.Time               `json:"expires_at"`
	PermissionNote string                   `json:"permission_note"`
	Limits         *dockerHubLimitsResponse `json:"limits"`
	CheckedAt      time.Time                `json:"checked_at"`
	Cached         bool                     `json:"cached"`
}

func (rt *Router) buildDockerHubStatus(ctx context.Context, id string, expires *time.Time, cred dockerhub.Credentials, refresh bool) (dockerHubStatusResponse, error) {
	client := rt.dockerHubAccount()
	ttl := rt.dockerHubCacheTTL(ctx)
	res := dockerHubStatusResponse{
		CredentialID: id, TokenKind: cred.Kind(), Namespace: cred.Username, Scopes: []string{}, Capabilities: []dockerHubCapabilityResource{},
		ExpiryKnown: expires != nil, ExpiresAt: expires, PermissionNote: dockerHubPermissionNote,
	}
	cv, err := client.CachedValidate(ctx, cred, ttl, refresh)
	if errors.Is(err, dockerhub.ErrUnauthorized) {
		res.Message, res.CheckedAt = registryvendor.DockerHubRejected, time.Now().UTC()
		return res, nil
	}
	if err != nil {
		return res, err
	}
	res.Valid, res.CheckedAt, res.Cached = true, cv.CheckedAt.UTC(), cv.Cached
	res.PrivateReposVisible = cv.Validation.PrivateReposVisible
	res.Scopes = append(res.Scopes, cv.Validation.Scopes...)
	for _, c := range cv.Validation.Capabilities {
		res.Capabilities = append(res.Capabilities, dockerHubCapabilityResource{Name: c.Name, Status: c.Status})
	}
	// A limits failure must not hide a valid token, so it only leaves limits null.
	if lim, at, cached, lerr := client.CachedPullLimits(ctx, cred, ttl, refresh); lerr == nil {
		l := rt.toDockerHubLimits(ctx, id, lim, at, cached)
		res.Limits = &l
	}
	return res, nil
}

// handleDockerHubStatus handles GET /api/v1/registry-credentials/{id}/dockerhub:
// whether Docker Hub accepts the stored token, what it can do, and the pull
// limit. The outcome is also recorded as the connection health.
func (rt *Router) handleDockerHubStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stored, cred, ok := rt.dockerHubCredentialFor(w, r, id)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*registryCredentialTestTimeout)
	defer cancel()
	res, err := rt.buildDockerHubStatus(ctx, id, stored.ExpiresAt, cred, r.URL.Query().Get("refresh") == "true")
	if err != nil {
		rt.writeDockerHubError(w, err, "status", slog.String("id", id))
		return
	}
	if !res.Cached {
		msg := ""
		if !res.Valid {
			msg = registryvendor.DockerHubRejected
		}
		rt.recordDockerHubHealth(ctx, stored, msg)
	}
	writeJSON(w, http.StatusOK, res)
}

type dockerHubProbeRequest struct {
	Username string `json:"username"`
	Token    string `json:"token"`
}

// handleProbeDockerHub handles POST /api/v1/registry-credentials/dockerhub/test:
// validates a candidate token before it is saved. Nothing is stored or cached.
func (rt *Router) handleProbeDockerHub(w http.ResponseWriter, r *http.Request) {
	if rt.dockerHubAccount() == nil {
		writeError(w, http.StatusNotImplemented, "Docker Hub integration is not configured on this control plane")
		return
	}
	var req dockerHubProbeRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Username == "" || req.Token == "" {
		writeError(w, http.StatusBadRequest, "username and token are required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*registryCredentialTestTimeout)
	defer cancel()
	res, err := rt.buildDockerHubStatus(ctx, "", nil, dockerhub.Credentials{Username: req.Username, Token: req.Token}, true)
	if err != nil {
		rt.writeDockerHubError(w, err, "probe")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// registryHealthRecorder is the optional store method that keeps the last
// validation outcome on the credential row.
type registryHealthRecorder interface {
	RecordRegistryCredentialRefresh(ctx context.Context, id, registryHost, username string, expiresAt, refreshedAt, nextAt *time.Time, errMsg string) error
}

func (rt *Router) recordDockerHubHealth(ctx context.Context, c store.RegistryCredential, errMsg string) {
	rec, ok := rt.registryCredentials.(registryHealthRecorder)
	if !ok {
		return
	}
	now := time.Now().UTC()
	var at *time.Time
	if errMsg == "" {
		at = &now
	}
	if err := rec.RecordRegistryCredentialRefresh(ctx, c.ID, "", "", nil, at, nil, errMsg); err != nil {
		rt.logger.Warn("api: docker hub: record health failed", slog.String("id", c.ID), slog.String("error", err.Error()))
	}
}

// verifyDockerHubOnSave checks a Docker Hub token before it is stored.
// rejected is true only when Docker refused it; an unreachable Docker Hub
// does not block saving.
func (rt *Router) verifyDockerHubOnSave(ctx context.Context, host, username, token string) (checked, rejected bool) {
	client := rt.dockerHubAccount()
	if client == nil || !registryvendor.IsDockerHubHost(host) || token == "" {
		return false, false
	}
	vctx, cancel := context.WithTimeout(ctx, registryCredentialTestTimeout)
	defer cancel()
	_, err := client.CachedValidate(vctx, dockerhub.Credentials{Username: username, Token: token}, 0, true)
	if errors.Is(err, dockerhub.ErrUnauthorized) {
		return true, true
	}
	return err == nil, false
}
