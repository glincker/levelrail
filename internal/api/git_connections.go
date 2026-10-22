package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const gitConnLabelMax = 80

var (
	errGitConnNone          = errors.New("api: no git connection is configured for this provider")
	errGitConnNotAuthorized = errors.New("api: git connection is configured but not authorized")
)

// gitRepoCacheKey is the repo list cache key of one connection.
func gitRepoCacheKey(provider, connectionID string) string { return provider + ":" + connectionID }

// gitConnectionResource is the wire shape of one provider connection. It
// carries no credential: the client secret and tokens stay in secrets.
type gitConnectionResource struct {
	ID          string          `json:"id"`
	Provider    string          `json:"provider"`
	Label       string          `json:"label"`
	Host        string          `json:"host"`
	InstanceURL string          `json:"instance_url"`
	ClientID    string          `json:"client_id"`
	AccountID   string          `json:"account_id,omitempty"`
	AccountName string          `json:"account_name,omitempty"`
	Scopes      string          `json:"scopes,omitempty"`
	IsDefault   bool            `json:"is_default"`
	Authorized  bool            `json:"authorized"`
	CreatedBy   string          `json:"created_by,omitempty"`
	CreatedAt   string          `json:"created_at"`
	SourceCount int             `json:"source_count"`
	Connection  gitConnResource `json:"connection"`
	// AuthorizePath is the browser navigation that authorizes or reauthorizes this connection.
	AuthorizePath string `json:"authorize_path"`
	// AuthMethod is "token" for a pasted access token and "oauth" for an OAuth application.
	AuthMethod string `json:"auth_method"`
}

const (
	gitAuthToken = "token"
	gitAuthOAuth = "oauth"
)

func (rt *Router) gitConnectionResourceFor(ctx context.Context, c store.GitConnection) gitConnectionResource {
	res := gitConnectionResource{
		ID: c.ID, Provider: c.Provider, Label: c.Label, Host: c.Host, InstanceURL: c.InstanceURL, ClientID: c.ClientID,
		AccountID: c.AccountID, AccountName: c.AccountName, Scopes: c.Scopes, IsDefault: c.IsDefault,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt, Connection: rt.gitConnHealth(ctx, c.ID),
		AuthorizePath: "/api/v1/" + c.Provider + "-app/connect?connection_id=" + url.QueryEscape(c.ID), AuthMethod: gitAuthOAuth,
	}
	if c.ClientID == "" {
		res.AuthorizePath, res.AuthMethod = "", gitAuthToken
	}
	if rt.gitConnSecrets != nil {
		ok, err := rt.gitConnSecrets.Exists(ctx, c.SecretsKey, gitConnAccessTokenKey)
		if err != nil {
			rt.logger.Warn("api: check git connection authorization failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
		}
		res.Authorized = ok
	}
	if names, err := rt.gitConns.ListGitSourcesByConnection(ctx, c.ID); err == nil {
		res.SourceCount = len(names)
	}
	return res
}

func validGitProvider(p string) bool {
	return slices.Contains(connectionGitProviders, p)
}

// handleListGitConnections handles GET /api/v1/git-connections?provider=:
// every connection with its account, host and health. AbilityReadSensitive,
// the tier of the repository listings that connections feed.
func (rt *Router) handleListGitConnections(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider != "" && !validGitProvider(provider) {
		writeError(w, http.StatusBadRequest, errGitProviderOneOf)
		return
	}
	conns, err := rt.gitConns.ListGitConnections(r.Context(), provider)
	if err != nil {
		rt.internalError(w, "api: list git connections failed", err)
		return
	}
	out := make([]gitConnectionResource, 0, len(conns))
	for _, c := range conns {
		out = append(out, rt.gitConnectionResourceFor(r.Context(), c))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) loadGitConnection(w http.ResponseWriter, r *http.Request) (store.GitConnection, bool) {
	c, err := rt.gitConns.GetGitConnection(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrGitConnectionNotFound) {
		writeError(w, http.StatusNotFound, "no such git connection")
		return store.GitConnection{}, false
	}
	if err != nil {
		rt.internalError(w, "api: get git connection failed", err, slog.String("connection", r.PathValue("id")))
		return store.GitConnection{}, false
	}
	return c, true
}

// handleGetGitConnection handles GET /api/v1/git-connections/{id}.
func (rt *Router) handleGetGitConnection(w http.ResponseWriter, r *http.Request) {
	if c, ok := rt.loadGitConnection(w, r); ok {
		writeJSON(w, http.StatusOK, rt.gitConnectionResourceFor(r.Context(), c))
	}
}

// createGitConnectionRequest registers an OAuth application. ReuseConnectionID
// copies the application of an existing connection so another account on the
// same instance needs no second application.
type createGitConnectionRequest struct {
	Provider          string `json:"provider"`
	InstanceURL       string `json:"instance_url"`
	ClientID          string `json:"client_id"`
	ClientSecret      string `json:"client_secret"`
	Label             string `json:"label"`
	ReuseConnectionID string `json:"reuse_connection_id"`
	// Token is the personal access token of a token-authenticated provider (Bitbucket Server); it replaces the OAuth client fields.
	Token string `json:"token"`
}

// handleCreateGitConnection handles POST /api/v1/git-connections: saves a new
// connection's OAuth application. It never replaces an existing connection;
// authorizing it is the separate browser step in AuthorizePath.
func (rt *Router) handleCreateGitConnection(w http.ResponseWriter, r *http.Request) {
	if rt.gitConnSecrets == nil {
		writeError(w, http.StatusNotImplemented, errGitMasterKeyRequired)
		return
	}
	var req createGitConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	adapter, ok := rt.gitAdapter(strings.TrimSpace(req.Provider))
	if !ok {
		writeError(w, http.StatusBadRequest, errGitProviderOneOf)
		return
	}
	if adapter.patAuth || strings.TrimSpace(req.Token) != "" {
		rt.createPATGitConnection(w, r, adapter, req)
		return
	}
	ctx := r.Context()
	req.Label, req.ClientID, req.ClientSecret = strings.TrimSpace(req.Label), strings.TrimSpace(req.ClientID), strings.TrimSpace(req.ClientSecret)
	req.InstanceURL = strings.TrimRight(strings.TrimSpace(req.InstanceURL), "/")

	if req.ReuseConnectionID != "" {
		src, err := rt.gitConns.GetGitConnection(ctx, req.ReuseConnectionID)
		if err != nil || src.Provider != adapter.provider {
			writeError(w, http.StatusBadRequest, "reuse_connection_id does not name a "+adapter.label+" connection")
			return
		}
		secret, err := rt.gitConnSecrets.Resolve(ctx, src.SecretsKey, adapter.secretKey)
		if err != nil {
			rt.internalError(w, "api: resolve reused git connection client secret failed", err, slog.String("connection", src.ID))
			return
		}
		req.InstanceURL, req.ClientID, req.ClientSecret = src.InstanceURL, src.ClientID, secret
	}
	if adapter.provider == gitProviderBitbucket {
		req.InstanceURL = "https://bitbucket.org"
	}
	if msg := validateGitConnectionInput(adapter.provider, req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	host := store.RepoHost(req.InstanceURL)
	if host == "" {
		writeError(w, http.StatusBadRequest, "instance_url must be a valid http or https address")
		return
	}
	if req.Label == "" {
		req.Label = gitConnDefaultLabel(adapter.provider, host, "")
	}
	if len(req.Label) > gitConnLabelMax {
		writeError(w, http.StatusBadRequest, "label is too long")
		return
	}

	id, err := store.NewGitConnectionID()
	if err != nil {
		rt.internalError(w, "api: generate git connection id failed", err)
		return
	}
	conn := store.GitConnection{
		ID: id, Provider: adapter.provider, Label: req.Label, Host: host, InstanceURL: req.InstanceURL, ClientID: req.ClientID,
		SecretsKey: store.GitConnectionSecretsKey(id), Scopes: adapter.scopes, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if _, _, name, ok := rt.currentActor(r); ok {
		conn.CreatedBy = name
	}
	if err := rt.gitConnSecrets.SetValue(ctx, conn.SecretsKey, adapter.secretKey, req.ClientSecret); err != nil {
		rt.internalError(w, "api: store git connection client secret failed", err, slog.String("connection", id))
		return
	}
	if err := rt.gitConns.CreateGitConnection(ctx, conn); err != nil {
		_ = rt.gitConnSecrets.DeleteAll(ctx, conn.SecretsKey)
		rt.internalError(w, "api: save git connection failed", err, slog.String("connection", id), slog.String("instance_url", redactURLCredentials(req.InstanceURL)))
		return
	}
	created, err := rt.gitConns.GetGitConnection(ctx, id)
	if err != nil {
		rt.internalError(w, "api: reload git connection failed", err, slog.String("connection", id))
		return
	}
	rt.logger.Info("api: git connection created", slog.String("connection", id), slog.String("provider", adapter.provider), slog.String("host", host))
	writeJSON(w, http.StatusCreated, rt.gitConnectionResourceFor(ctx, created))
}

func validateGitConnectionInput(provider string, req createGitConnectionRequest) string {
	if provider != gitProviderBitbucket {
		if req.InstanceURL == "" {
			return "instance_url is required"
		}
		if err := requireHTTPOrHTTPSScheme(req.InstanceURL); err != nil {
			return "instance_url must use http or https"
		}
	}
	if req.ClientID == "" {
		return "client_id is required"
	}
	if req.ClientSecret == "" {
		return "client_secret is required"
	}
	return ""
}

// handleRenameGitConnection handles PATCH /api/v1/git-connections/{id}.
func (rt *Router) handleRenameGitConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadGitConnection(w, r)
	if !ok {
		return
	}
	var req struct {
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" || len(label) > gitConnLabelMax {
		writeError(w, http.StatusBadRequest, "label is required and must be at most 80 characters")
		return
	}
	if err := rt.gitConns.RenameGitConnection(r.Context(), c.ID, label); err != nil {
		rt.internalError(w, "api: rename git connection failed", err, slog.String("connection", c.ID))
		return
	}
	c.Label = label
	writeJSON(w, http.StatusOK, rt.gitConnectionResourceFor(r.Context(), c))
}

// handleSetDefaultGitConnection handles POST /api/v1/git-connections/{id}/default.
func (rt *Router) handleSetDefaultGitConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadGitConnection(w, r)
	if !ok {
		return
	}
	if err := rt.gitConns.SetDefaultGitConnection(r.Context(), c.ID); err != nil {
		rt.internalError(w, "api: set default git connection failed", err, slog.String("connection", c.ID))
		return
	}
	c.IsDefault = true
	writeJSON(w, http.StatusOK, rt.gitConnectionResourceFor(r.Context(), c))
}

// handleDisconnectGitConnection handles DELETE /api/v1/git-connections/{id}.
// Other connections are untouched. Git sources that used this one stay and
// report needs_reconnect until an account on the same host is connected again.
func (rt *Router) handleDisconnectGitConnection(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadGitConnection(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	sources, err := rt.gitConns.ListGitSourcesByConnection(ctx, c.ID)
	if err != nil {
		rt.internalError(w, "api: list sources of git connection failed", err, slog.String("connection", c.ID))
		return
	}
	revoked, note := rt.revokeGitConnGrant(ctx, c)
	if err := rt.gitConns.DeleteGitConnection(ctx, c.ID); err != nil {
		if errors.Is(err, store.ErrGitConnectionNotFound) {
			writeError(w, http.StatusNotFound, "no such git connection")
			return
		}
		rt.internalError(w, "api: delete git connection failed", err, slog.String("connection", c.ID))
		return
	}
	if rt.gitConnSecrets != nil {
		if err := rt.gitConnSecrets.DeleteAll(ctx, c.SecretsKey); err != nil {
			rt.logger.Error("api: delete git connection secrets failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
		}
	}
	rt.invalidateGitRepoCache(ctx, gitRepoCacheKey(c.Provider, c.ID))
	rt.logger.Info("api: git connection disconnected", slog.String("connection", c.ID), slog.String("provider", c.Provider), slog.Int("sources", len(sources)))
	if len(sources) > 0 {
		note += fmt.Sprintf(" %d git source(s) used this connection; they stay and show needs reconnect until you connect an account on %s again.", len(sources), c.Host)
	}
	rt.finishGitDisconnect(ctx, w, c.ID, revoked, strings.TrimSpace(note), sources)
}

// revokeGitConnGrant ends the OAuth grant at the provider when it supports
// that. A failure never blocks the disconnect; the note says what to do.
func (rt *Router) revokeGitConnGrant(ctx context.Context, c store.GitConnection) (bool, string) {
	adapter, ok := rt.gitAdapter(c.Provider)
	if !ok || rt.gitConnSecrets == nil {
		return false, ""
	}
	if c.ClientID == "" && adapter.revoke != nil {
		return false, gitTokenRevokeNote
	}
	if adapter.revoke == nil {
		return false, adapter.revokeNote
	}
	token, err := rt.gitConnSecrets.Resolve(ctx, c.SecretsKey, gitConnRefreshTokenKey)
	if err != nil || token == "" {
		token, err = rt.gitConnSecrets.Resolve(ctx, c.SecretsKey, gitConnAccessTokenKey)
	}
	secret, secretErr := rt.gitConnSecrets.Resolve(ctx, c.SecretsKey, adapter.secretKey)
	if err != nil || secretErr != nil {
		return false, "There was no stored token to revoke. " + adapter.revokeNote
	}
	rctx, cancel := context.WithTimeout(ctx, gitRevokeTimeout)
	defer cancel()
	if err := adapter.revoke(rctx, c, secret, token); err != nil {
		rt.logger.Warn("api: revoke git connection token failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
		return false, adapter.label + " did not accept the revocation. " + adapter.revokeNote
	}
	return true, "The token was revoked at " + adapter.label + "."
}

// resolveGitConn picks the connection a request means: the named one, else
// the provider's default.
func (rt *Router) resolveGitConn(ctx context.Context, provider, id string) (store.GitConnection, error) {
	if id != "" {
		c, err := rt.gitConns.GetGitConnection(ctx, id)
		if err != nil {
			return store.GitConnection{}, fmt.Errorf("get git connection %q: %w", id, err)
		}
		if c.Provider != provider {
			return store.GitConnection{}, fmt.Errorf("git connection %q is not a %s connection: %w", id, provider, store.ErrGitConnectionNotFound)
		}
		return c, nil
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("list %s connections: %w", provider, err)
	}
	for _, c := range conns {
		if c.IsDefault {
			return c, nil
		}
	}
	return store.GitConnection{}, errGitConnNone
}

// gitConnForRepo picks the connection that serves a repository: the one the
// source recorded, else a connection on the repository's host (default first).
func (rt *Router) gitConnForRepo(ctx context.Context, provider, repoURL, connectionID string) (store.GitConnection, error) {
	if connectionID != "" {
		return rt.resolveGitConn(ctx, provider, connectionID)
	}
	host := store.RepoHost(repoURL)
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("list %s connections: %w", provider, err)
	}
	var match *store.GitConnection
	for i := range conns {
		if conns[i].Host != host {
			continue
		}
		if conns[i].IsDefault {
			return conns[i], nil
		}
		if match == nil {
			match = &conns[i]
		}
	}
	if match == nil {
		return store.GitConnection{}, errGitConnNone
	}
	return *match, nil
}

// gitTokenForRepo is gitConnForRepo plus a usable access token.
func (rt *Router) gitTokenForRepo(ctx context.Context, provider, repoURL, connectionID string) (store.GitConnection, string, error) {
	c, err := rt.gitConnForRepo(ctx, provider, repoURL, connectionID)
	if err != nil {
		return store.GitConnection{}, "", err
	}
	return rt.gitConnToken(ctx, c.ID)
}

// writeGitConnTokenError maps token-path failures to 404, 409, 429 or a logged 500.
func (rt *Router) writeGitConnTokenError(w http.ResponseWriter, logMsg string, err error) {
	if rt.writeGitConnError(w, err) {
		return
	}
	switch {
	case errors.Is(err, errGitConnNone):
		writeError(w, http.StatusConflict, "no account is connected for this provider; connect one first")
	case errors.Is(err, store.ErrGitConnectionNotFound):
		writeError(w, http.StatusNotFound, "no such git connection")
	case errors.Is(err, errGitConnNotAuthorized):
		writeError(w, http.StatusConflict, "this connection is not authorized yet; authorize it first")
	default:
		rt.logger.Error(logMsg, slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, errInternal)
	}
}

// gitProviderStatusFor summarizes a provider's connections for the git-providers list.
func (rt *Router) gitProviderStatusFor(ctx context.Context, provider string) gitProviderResource {
	res := gitProviderResource{Provider: provider}
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	if err != nil {
		rt.logger.Error("api: list git connections for git-providers status failed", slog.String("provider", provider), slog.String("error", err.Error()))
		return res
	}
	res.Connections = len(conns)
	if rt.gitConnSecrets == nil {
		return res
	}
	for _, c := range conns {
		ok, err := rt.gitConnSecrets.Exists(ctx, c.SecretsKey, gitConnAccessTokenKey)
		if err != nil || !ok {
			continue
		}
		res.Connected = true
		res.CanListBranches = true
		res.CanRegisterWebhook = true
		res.Authorized++
	}
	return res
}

const gitSourceNeedsReconnect = "needs_reconnect"

// gitSourceConnectionResource is the state of the connection a git source was picked from.
type gitSourceConnectionResource struct {
	ID          string `json:"id"`
	Provider    string `json:"provider,omitempty"`
	Label       string `json:"label,omitempty"`
	AccountName string `json:"account_name,omitempty"`
	Host        string `json:"host,omitempty"`
	// State is ok, needs_reauthorize, revoked, rate_limited, or needs_reconnect once the connection was removed.
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// gitSourceConnectionState reports the connection behind a source, or nil for
// a source that was not picked from one.
func (rt *Router) gitSourceConnectionState(ctx context.Context, connectionID string) *gitSourceConnectionResource {
	if connectionID == "" {
		return nil
	}
	c, err := rt.gitConns.GetGitConnection(ctx, connectionID)
	if errors.Is(err, store.ErrGitConnectionNotFound) {
		return &gitSourceConnectionResource{ID: connectionID, State: gitSourceNeedsReconnect, Detail: "the account this repository was picked from was disconnected"}
	}
	if err != nil {
		rt.logger.Warn("api: load git source connection failed", slog.String("connection", connectionID), slog.String("error", err.Error()))
		return nil
	}
	res := &gitSourceConnectionResource{ID: c.ID, Provider: c.Provider, Label: c.Label, AccountName: c.AccountName, Host: c.Host, State: store.GitConnOK}
	if rt.gitConnSecrets != nil {
		if ok, _ := rt.gitConnSecrets.Exists(ctx, c.SecretsKey, gitConnAccessTokenKey); !ok {
			res.State, res.Detail = store.GitConnNeedsReauthorize, "this connection has no valid authorization"
			return res
		}
	}
	if h := rt.gitConnHealth(ctx, c.ID); h.Status != store.GitConnOK {
		res.State, res.Detail = h.Status, h.Detail
	}
	return res
}
