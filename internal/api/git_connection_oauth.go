package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const gitAccountLookupTimeout = 10 * time.Second

// handleStartGitConnect returns the handler of GET /api/v1/{provider}-app/connect:
// the redirect into the provider's OAuth2 authorization page for one
// connection. Without connection_id it authorizes the provider's newest
// connection that has no token yet.
func (rt *Router) handleStartGitConnect(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapter, _ := rt.gitAdapter(provider)
		if rt.gitConnSecrets == nil {
			writeError(w, http.StatusNotImplemented, errGitMasterKeyRequired)
			return
		}
		ctx := r.Context()
		conn, err := rt.connectionToAuthorize(ctx, provider, r.URL.Query().Get("connection_id"))
		if err != nil {
			switch {
			case errors.Is(err, store.ErrGitConnectionNotFound), errors.Is(err, errGitConnNone):
				writeError(w, http.StatusConflict, "register the "+adapter.label+" OAuth application first, then authorize it")
			default:
				rt.internalError(w, "api: pick git connection to authorize failed", err, slog.String("provider", provider))
			}
			return
		}
		baseURL, err := rt.controlPlaneBaseURL(ctx)
		if err != nil {
			if errors.Is(err, errNoPrimaryDomain) {
				writeError(w, http.StatusConflict, "set a primary domain in ingress settings before connecting "+provider+": it needs a real, reachable redirect url")
				return
			}
			rt.internalError(w, "api: get ingress settings for git connect failed", err, slog.String("provider", provider))
			return
		}
		state, err := rt.gitConnState.begin(conn.ID)
		if err != nil {
			rt.internalError(w, "api: begin git oauth state failed", err, slog.String("connection", conn.ID))
			return
		}
		http.Redirect(w, r, adapter.authorizeURL(conn, baseURL+adapter.callbackPath, state), http.StatusFound)
	}
}

func (rt *Router) connectionToAuthorize(ctx context.Context, provider, id string) (store.GitConnection, error) {
	if id != "" {
		return rt.resolveGitConn(ctx, provider, id)
	}
	conns, err := rt.gitConns.ListGitConnections(ctx, provider)
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("list %s connections: %w", provider, err)
	}
	for i := len(conns) - 1; i >= 0; i-- {
		ok, existsErr := rt.gitConnSecrets.Exists(ctx, conns[i].SecretsKey, gitConnAccessTokenKey)
		if existsErr == nil && !ok {
			return conns[i], nil
		}
	}
	return store.GitConnection{}, errGitConnNone
}

// handleGitConnectCallback returns the handler of GET /api/v1/{provider}-app/callback.
// The state value names the connection that started the attempt.
func (rt *Router) handleGitConnectCallback(provider string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapter, _ := rt.gitAdapter(provider)
		if rt.gitConnSecrets == nil {
			writeError(w, http.StatusNotImplemented, errGitMasterKeyRequired)
			return
		}
		q := r.URL.Query()
		code, state := strings.TrimSpace(q.Get("code")), strings.TrimSpace(q.Get("state"))
		if code == "" {
			writeError(w, http.StatusBadRequest, "missing code parameter")
			return
		}
		if state == "" {
			writeError(w, http.StatusBadRequest, "missing state parameter")
			return
		}
		connID, ok := rt.gitConnState.consume(state)
		if !ok {
			writeError(w, http.StatusBadRequest, "state parameter is invalid, expired, or already used")
			return
		}
		ctx := r.Context()
		conn, err := rt.resolveGitConn(ctx, provider, connID)
		if err != nil {
			writeError(w, http.StatusConflict, "the connection being authorized no longer exists")
			return
		}
		secret, err := rt.gitConnSecrets.Resolve(ctx, conn.SecretsKey, adapter.secretKey)
		if err != nil {
			rt.internalError(w, "api: resolve git connection client secret failed", err, slog.String("connection", conn.ID))
			return
		}
		baseURL, err := rt.controlPlaneBaseURL(ctx)
		if err != nil {
			rt.internalError(w, "api: get base url for git callback failed", err, slog.String("connection", conn.ID))
			return
		}
		tokens, err := adapter.exchange(ctx, conn, secret, baseURL+adapter.callbackPath, code)
		if err != nil {
			rt.logger.Error("api: exchange git oauth code failed", slog.String("connection", conn.ID), slog.String("error", err.Error()))
			writeError(w, http.StatusBadGateway, "failed to exchange the authorization code with "+adapter.label)
			return
		}
		target, err := rt.landGitTokens(ctx, adapter, conn, secret, tokens)
		if err != nil {
			rt.internalError(w, "api: save git oauth tokens failed", err, slog.String("connection", conn.ID))
			return
		}
		rt.auditGitConn(ctx, r, "git_connection.authorized", target.ID)
		rt.invalidateGitRepoCache(ctx, gitRepoCacheKey(provider, target.ID))
		rt.markGitConnOK(ctx, target.ID)
		dest := baseURL + "/settings/" + provider + "-app?git_connection=connected&id=" + url.QueryEscape(target.ID)
		http.Redirect(w, r, dest, http.StatusFound)
	}
}

// landGitTokens stores freshly exchanged tokens on the right connection:
// the one that started the flow, an existing connection for the same
// account (reconnect, never a duplicate), or a new connection when a
// different account signed in through an already authorized one.
func (rt *Router) landGitTokens(ctx context.Context, adapter gitProviderAdapter, conn store.GitConnection, secret string, tokens gitOAuthTokens) (store.GitConnection, error) {
	acct, acctErr := rt.lookupGitAccount(ctx, adapter, conn, tokens.Access)
	if acctErr != nil {
		rt.logger.Warn("api: look up git account after authorization failed", slog.String("connection", conn.ID), slog.String("error", acctErr.Error()))
	}
	wasAuthorized, err := rt.gitConnSecrets.Exists(ctx, conn.SecretsKey, gitConnAccessTokenKey)
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("check existing authorization: %w", err)
	}
	target := conn
	switch {
	case acctErr != nil || acct.ID == "":
	case conn.AccountID != "" && conn.AccountID != acct.ID:
		clone, err := rt.cloneGitConnection(ctx, adapter, conn, secret)
		if err != nil {
			return store.GitConnection{}, err
		}
		target, wasAuthorized = clone, false
	case !wasAuthorized:
		existing, found, err := rt.gitConns.FindGitConnectionByAccount(ctx, conn.Provider, conn.Host, acct.ID, conn.ClientID, conn.ID)
		if err != nil {
			return store.GitConnection{}, fmt.Errorf("find existing connection for account: %w", err)
		}
		if found {
			target, wasAuthorized = existing, true
			if err := rt.gitConnSecrets.SetValue(ctx, existing.SecretsKey, adapter.secretKey, secret); err != nil {
				return store.GitConnection{}, fmt.Errorf("update client secret of existing connection: %w", err)
			}
			rt.dropPendingGitConnection(ctx, conn)
		}
	}
	if err := rt.saveGitTokens(ctx, target, tokens); err != nil {
		return store.GitConnection{}, err
	}
	if acctErr == nil && acct.ID != "" {
		rt.recordGitAccount(ctx, adapter, &target, acct)
	}
	if !wasAuthorized {
		if n, err := rt.gitConns.RelinkOrphanedGitSources(ctx, target.Host, target.ID); err != nil {
			rt.logger.Warn("api: relink orphaned git sources failed", slog.String("connection", target.ID), slog.String("error", err.Error()))
		} else if n > 0 {
			rt.logger.Info("api: git sources reattached to connection", slog.String("connection", target.ID), slog.Int("sources", n))
		}
	}
	return target, nil
}

func (rt *Router) lookupGitAccount(ctx context.Context, adapter gitProviderAdapter, conn store.GitConnection, token string) (gitAccountInfo, error) {
	actx, cancel := context.WithTimeout(ctx, gitAccountLookupTimeout)
	defer cancel()
	return adapter.account(actx, conn, token)
}

func (rt *Router) recordGitAccount(ctx context.Context, adapter gitProviderAdapter, c *store.GitConnection, acct gitAccountInfo) {
	scopes := c.Scopes
	if scopes == "" {
		scopes = adapter.scopes
	}
	if err := rt.gitConns.SetGitConnectionAccount(ctx, c.ID, acct.ID, acct.Login, scopes); err != nil {
		rt.logger.Warn("api: record git connection account failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
		return
	}
	if c.Label == gitConnDefaultLabel(c.Provider, c.Host, "") {
		if err := rt.gitConns.RenameGitConnection(ctx, c.ID, gitConnDefaultLabel(c.Provider, c.Host, acct.Login)); err != nil {
			rt.logger.Warn("api: label git connection after authorization failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
		}
	}
	c.AccountID, c.AccountName, c.Scopes = acct.ID, acct.Login, scopes
}

func (rt *Router) cloneGitConnection(ctx context.Context, adapter gitProviderAdapter, from store.GitConnection, secret string) (store.GitConnection, error) {
	id, err := store.NewGitConnectionID()
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("generate id for additional connection: %w", err)
	}
	c := store.GitConnection{
		ID: id, Provider: from.Provider, Label: gitConnDefaultLabel(from.Provider, from.Host, ""), Host: from.Host,
		InstanceURL: from.InstanceURL, ClientID: from.ClientID, SecretsKey: store.GitConnectionSecretsKey(id),
		Scopes: from.Scopes, CreatedBy: from.CreatedBy, CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, adapter.secretKey, secret); err != nil {
		return store.GitConnection{}, fmt.Errorf("store client secret of additional connection: %w", err)
	}
	if err := rt.gitConns.CreateGitConnection(ctx, c); err != nil {
		_ = rt.gitConnSecrets.DeleteAll(ctx, c.SecretsKey)
		return store.GitConnection{}, fmt.Errorf("create additional connection: %w", err)
	}
	return c, nil
}

func (rt *Router) dropPendingGitConnection(ctx context.Context, c store.GitConnection) {
	if err := rt.gitConns.DeleteGitConnection(ctx, c.ID); err != nil {
		rt.logger.Warn("api: drop pending git connection failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
	}
	if err := rt.gitConnSecrets.DeleteAll(ctx, c.SecretsKey); err != nil {
		rt.logger.Warn("api: delete pending git connection secrets failed", slog.String("connection", c.ID), slog.String("error", err.Error()))
	}
}

// saveGitTokens stores the expiry as a Unix timestamp string so the token
// path can compare it with time.Now() without a format to get wrong.
func (rt *Router) saveGitTokens(ctx context.Context, c store.GitConnection, t gitOAuthTokens) error {
	if err := rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, gitConnAccessTokenKey, t.Access); err != nil {
		return fmt.Errorf("save access_token: %w", err)
	}
	if t.Refresh != "" {
		if err := rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, gitConnRefreshTokenKey, t.Refresh); err != nil {
			return fmt.Errorf("save refresh_token: %w", err)
		}
	}
	if err := rt.gitConnSecrets.SetValue(ctx, c.SecretsKey, gitConnTokenExpiresKey, strconv.FormatInt(t.Expires.Unix(), 10)); err != nil {
		return fmt.Errorf("save token_expires_at: %w", err)
	}
	return nil
}

// gitConnToken returns a valid access token for one connection, refreshing
// it first when it is at or past its recorded expiry. The refresh is
// serialized per connection and records that connection's health.
func (rt *Router) gitConnToken(ctx context.Context, id string) (store.GitConnection, string, error) {
	conn, err := rt.gitConns.GetGitConnection(ctx, id)
	if err != nil {
		return store.GitConnection{}, "", fmt.Errorf("get git connection %q: %w", id, err)
	}
	adapter, ok := rt.gitAdapter(conn.Provider)
	if !ok || rt.gitConnSecrets == nil {
		return store.GitConnection{}, "", fmt.Errorf("git connection %q cannot be used: %w", id, errGitConnNone)
	}
	authorized, err := rt.gitConnSecrets.Exists(ctx, conn.SecretsKey, gitConnAccessTokenKey)
	if err != nil {
		return store.GitConnection{}, "", fmt.Errorf("check access token of %s: %w", id, err)
	}
	if !authorized {
		return store.GitConnection{}, "", errGitConnNotAuthorized
	}
	rawExpiry, err := rt.gitConnSecrets.Resolve(ctx, conn.SecretsKey, gitConnTokenExpiresKey)
	if err != nil {
		return store.GitConnection{}, "", fmt.Errorf("resolve token expiry of %s: %w", id, err)
	}
	expiresUnix, err := strconv.ParseInt(rawExpiry, 10, 64)
	if err != nil {
		return store.GitConnection{}, "", fmt.Errorf("parse token expiry of %s: %w", id, err)
	}
	var token string
	if time.Now().Add(gitOAuthRefreshMargin).Before(time.Unix(expiresUnix, 0)) {
		token, err = rt.gitConnSecrets.Resolve(ctx, conn.SecretsKey, gitConnAccessTokenKey)
		if err != nil {
			return store.GitConnection{}, "", fmt.Errorf("resolve access token of %s: %w", id, err)
		}
	} else {
		token, err = rt.refreshGitOAuth(ctx, gitOAuthSpec{
			provider: conn.Provider, key: conn.ID,
			read: func(ctx context.Context) (gitOAuthState, error) {
				return rt.readGitOAuthState(ctx, rt.gitConnSecrets, conn.SecretsKey, gitConnAccessTokenKey, gitConnRefreshTokenKey, gitConnTokenExpiresKey)
			},
			exchange: func(ctx context.Context, refresh string) (gitOAuthTokens, error) {
				secret, err := rt.gitConnSecrets.Resolve(ctx, conn.SecretsKey, adapter.secretKey)
				if err != nil {
					return gitOAuthTokens{}, fmt.Errorf("resolve client secret: %w", err)
				}
				return adapter.refresh(ctx, conn, secret, refresh)
			},
			save: func(ctx context.Context, t gitOAuthTokens) error { return rt.saveGitTokens(ctx, conn, t) },
		})
		if err != nil {
			return store.GitConnection{}, "", err
		}
	}
	if conn.AccountID == "" {
		rt.backfillGitAccount(ctx, adapter, &conn, token)
	}
	return conn, token, nil
}

// backfillGitAccount fills in the account of a connection migrated from the
// single-connection model, once per process and best effort.
func (rt *Router) backfillGitAccount(ctx context.Context, adapter gitProviderAdapter, conn *store.GitConnection, token string) {
	if _, tried := rt.gitAccountTried.LoadOrStore(conn.ID, true); tried {
		return
	}
	acct, err := rt.lookupGitAccount(ctx, adapter, *conn, token)
	if err != nil || acct.ID == "" {
		rt.logger.Info("api: git connection account not available yet", slog.String("connection", conn.ID))
		return
	}
	rt.recordGitAccount(ctx, adapter, conn, acct)
}

// auditGitConn records a connection lifecycle event the request audit row cannot carry.
func (rt *Router) auditGitConn(ctx context.Context, r *http.Request, action, connectionID string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	entry := store.AuditEntry{
		ID: id, Ability: action, Method: auditMethodEvent, Path: "/api/v1/git-connections/" + connectionID, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action, RemoteAddr: clientIP(r),
	}
	if t, aid, name, ok := rt.currentActor(r); ok {
		entry.ActorType, entry.ActorID, entry.ActorName = t, aid, name
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save git connection audit entry failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
