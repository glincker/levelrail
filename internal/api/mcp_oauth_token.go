package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

type mcpTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// mintMCPOAuthToken mints through the same engine mint and legacy mirror as an
// ordinary API token, so the token shows in the token list and can be revoked there.
func (rt *Router) mintMCPOAuthToken(ctx context.Context, userID, clientName string, abilities []string, expires time.Time) (raw, tokenID string, err error) {
	tokenID, err = randomTokenID()
	if err != nil {
		return "", "", fmt.Errorf("api: mcp oauth token id: %w", err)
	}
	agent := mcpOAuthAgentName(clientName)
	raw, rec, err := rt.authLib.tokens.MintToken(ctx, authengine.MintInput{
		OwnerLegacyID: userID, Name: agent, Abilities: abilities, ExpiresAt: &expires, AgentName: agent,
	})
	if err != nil {
		return "", "", fmt.Errorf("api: mcp oauth token mint: %w", err)
	}
	legacy := store.APIToken{
		ID: tokenID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: abilities, CreatedAt: rec.CreatedAt,
		ExpiresAt: &expires, AgentName: agent, OwnerUserID: userID,
	}
	if err := rt.tokens.SaveAPIToken(ctx, legacy); err != nil {
		_ = rt.authLib.tokens.RevokeToken(ctx, rec.EngineID)
		return "", "", fmt.Errorf("api: mcp oauth token mirror: %w", err)
	}
	if err := rt.authLib.tokens.LinkToken(ctx, tokenID, rec.EngineID); err != nil {
		return "", "", fmt.Errorf("api: mcp oauth token link: %w", err)
	}
	return raw, tokenID, nil
}

// revokeMCPGrant revokes the grant and every access token it issued.
func (rt *Router) revokeMCPGrant(ctx context.Context, st mcpOAuthStore, grantID string) {
	ids, err := st.RevokeMCPOAuthGrant(ctx, grantID, nowUnix())
	if err != nil {
		rt.logger.Error("api: mcp oauth grant revoke failed", "grant_id", grantID)
		return
	}
	for _, id := range ids {
		if err := rt.libraryRevokeToken(ctx, id); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			rt.logger.Warn("api: mcp oauth token revoke failed", "token_id", id)
		}
	}
}

// grantTokensRevoked reports whether an operator revoked any token of the
// grant, which must end the whole grant so a refresh cannot undo it.
func (rt *Router) grantTokensRevoked(ctx context.Context, st mcpOAuthStore, grantID string) bool {
	ids, err := st.MCPOAuthGrantTokenIDs(ctx, grantID)
	if err != nil {
		return true
	}
	for _, id := range ids {
		if t, err := rt.tokens.GetAPITokenByID(ctx, id); err == nil && t.RevokedAt != nil {
			return true
		}
	}
	return false
}

// handleMCPToken serves the token endpoint for the authorization_code and
// refresh_token grants. Clients are public: PKCE is the proof of possession.
func (m *mcpHTTP) handleMCPToken(w http.ResponseWriter, r *http.Request) {
	rt := m.rt
	st, _, ok := rt.mcpOAuthReady(w, r)
	if !ok {
		return
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/x-www-form-urlencoded" {
		oauthErrorBody(w, http.StatusBadRequest, "invalid_request", "Content-Type must be application/x-www-form-urlencoded")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		oauthErrorBody(w, http.StatusBadRequest, "invalid_request", "unreadable form body")
		return
	}
	clientID := r.PostFormValue("client_id")
	if allowed, retry := m.oauthLimit.allow("client:" + clientID + "|" + clientIP(r)); !allowed {
		writeRateLimited(w, retry)
		return
	}
	client, err := st.GetMCPOAuthClient(r.Context(), clientID)
	if errors.Is(err, store.ErrMCPOAuthNotFound) {
		oauthErrorBody(w, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		return
	}
	if err != nil {
		rt.internalError(w, "api: mcp oauth token: load client failed", err)
		return
	}
	switch r.PostFormValue("grant_type") {
	case "authorization_code":
		rt.mcpCodeGrant(w, r, st, client)
	case "refresh_token":
		rt.mcpRefreshGrant(w, r, st, client)
	default:
		oauthErrorBody(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
	}
}

func (rt *Router) mcpCodeGrant(w http.ResponseWriter, r *http.Request, st mcpOAuthStore, client *store.MCPOAuthClient) {
	invalid := func(desc string) { oauthErrorBody(w, http.StatusBadRequest, "invalid_grant", desc) }
	code := r.PostFormValue("code")
	if code == "" {
		invalid("code is required")
		return
	}
	// Only the client the code was issued to may burn or replay it.
	if pre, perr := st.GetMCPOAuthRequestByCode(r.Context(), sha256Hex(code)); perr == nil && subtle.ConstantTimeCompare([]byte(pre.ClientID), []byte(client.ID)) != 1 {
		invalid("the authorization code was issued to another client")
		return
	}
	req, reused, err := st.ConsumeMCPOAuthCode(r.Context(), sha256Hex(code), nowUnix())
	if err != nil {
		if reused && req != nil {
			// A replayed code means it may have leaked, so the grant it created ends.
			if g, gerr := st.GetMCPOAuthGrantByRequest(r.Context(), req.ID); gerr == nil {
				rt.revokeMCPGrant(r.Context(), st, g.ID)
				rt.mcpOAuthAudit(r.Context(), r, AuditActionMCPOAuthReuse, "token", g.ID, map[string]any{"client_id": client.ID, "kind": "code"})
			}
		}
		if !errors.Is(err, store.ErrMCPOAuthNotFound) {
			rt.internalError(w, "api: mcp oauth token: consume code failed", err)
			return
		}
		invalid("the authorization code is invalid, expired or already used")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.ClientID), []byte(client.ID)) != 1 || r.PostFormValue("redirect_uri") != req.RedirectURI {
		invalid("client_id or redirect_uri does not match the authorization request")
		return
	}
	if res := r.PostFormValue("resource"); res != "" && res != req.Resource {
		oauthErrorBody(w, http.StatusBadRequest, "invalid_target", "resource does not match the authorization request")
		return
	}
	if !pkceS256(r.PostFormValue("code_verifier"), req.CodeChallenge) {
		invalid("code_verifier does not match the code_challenge")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), req.UserID)
	if err != nil {
		invalid("the approving user no longer exists")
		return
	}
	if gone, gerr := rt.tokenOwnerGone(r.Context(), &store.APIToken{OwnerUserID: user.ID}); gerr != nil || gone {
		invalid("the approving user no longer exists or is disabled")
		return
	}
	scopes := mcpGrantable(req.Scopes, user.Abilities)
	if len(scopes) == 0 {
		invalid("the approving user no longer holds the granted scopes")
		return
	}
	grantID, err := randomURLToken(18)
	if err != nil {
		rt.internalError(w, "api: mcp oauth token: grant id failed", err)
		return
	}
	now := time.Now()
	grant := store.MCPOAuthGrant{
		ID: grantID, ClientID: client.ID, UserID: user.ID, Scopes: scopes, Resource: req.Resource, RequestID: req.ID,
		ExpiresAt: now.Add(envDuration(envMCPOAuthGrantMax, mcpOAuthDefaultGrantMax)).Unix(),
	}
	access, tokenID, refresh, accessExp, err := rt.mcpMintPair(r.Context(), client, grant, now)
	if err != nil {
		rt.internalError(w, "api: mcp oauth token: mint failed", err)
		return
	}
	refreshExp := mcpBoundedExpiry(now, envMCPOAuthRefreshTTL, mcpOAuthDefaultRefreshTTL, grant.ExpiresAt)
	if err := st.CreateMCPOAuthGrant(r.Context(), grant, tokenID, sha256Hex(refresh), refreshExp, now.Unix()); err != nil {
		_ = rt.libraryRevokeToken(r.Context(), tokenID)
		rt.internalError(w, "api: mcp oauth token: save grant failed", err)
		return
	}
	rt.mcpOAuthAudit(r.Context(), r, AuditActionMCPOAuthGrant, "token", tokenID, map[string]any{"client_id": client.ID, "grant_id": grantID, "scopes": scopes, "stage": "token_issued"})
	writeMCPTokens(w, access, refresh, accessExp.Sub(now), scopes)
}

func mcpBoundedExpiry(now time.Time, env string, def time.Duration, limit int64) int64 {
	exp := now.Add(envDuration(env, def)).Unix()
	if exp > limit {
		return limit
	}
	return exp
}

// mcpMintPair mints an access token and a fresh refresh token for the grant.
func (rt *Router) mcpMintPair(ctx context.Context, client *store.MCPOAuthClient, g store.MCPOAuthGrant, now time.Time) (access, tokenID, refresh string, accessExp time.Time, err error) {
	accessExp = time.Unix(mcpBoundedExpiry(now, envMCPOAuthAccessTTL, mcpOAuthDefaultAccessTTL, g.ExpiresAt), 0)
	access, tokenID, err = rt.mintMCPOAuthToken(ctx, g.UserID, client.Name, mcpAbilitiesFor(g.Scopes), accessExp)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	refresh, err = randomURLToken(32)
	if err != nil {
		_ = rt.libraryRevokeToken(ctx, tokenID)
		return "", "", "", time.Time{}, err
	}
	return access, tokenID, refresh, accessExp, nil
}

func (rt *Router) mcpRefreshGrant(w http.ResponseWriter, r *http.Request, st mcpOAuthStore, client *store.MCPOAuthClient) {
	invalid := func(desc string) { oauthErrorBody(w, http.StatusBadRequest, "invalid_grant", desc) }
	old := r.PostFormValue("refresh_token")
	if old == "" {
		invalid("refresh_token is required")
		return
	}
	oldHash := sha256Hex(old)
	g, err := st.GetMCPOAuthGrantByRefresh(r.Context(), oldHash)
	if errors.Is(err, store.ErrMCPOAuthNotFound) {
		invalid("the refresh token is invalid")
		return
	}
	if err != nil {
		rt.internalError(w, "api: mcp oauth refresh: load grant failed", err)
		return
	}
	now := time.Now()
	if subtle.ConstantTimeCompare([]byte(g.ClientID), []byte(client.ID)) != 1 {
		invalid("the refresh token was issued to another client")
		return
	}
	if g.Revoked || g.ExpiresAt <= now.Unix() || rt.grantTokensRevoked(r.Context(), st, g.ID) {
		rt.revokeMCPGrant(r.Context(), st, g.ID)
		invalid("the grant was revoked or has expired")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), g.UserID)
	if err != nil {
		rt.revokeMCPGrant(r.Context(), st, g.ID)
		invalid("the approving user no longer exists")
		return
	}
	if gone, gerr := rt.tokenOwnerGone(r.Context(), &store.APIToken{OwnerUserID: user.ID}); gerr != nil || gone {
		rt.revokeMCPGrant(r.Context(), st, g.ID)
		invalid("the approving user no longer exists or is disabled")
		return
	}
	// Scopes are re-capped on every refresh so a demoted user cannot keep reading.
	g.Scopes = slices.DeleteFunc(slices.Clone(g.Scopes), func(s string) bool { return !slices.Contains(mcpGrantable(g.Scopes, user.Abilities), s) })
	if len(g.Scopes) == 0 {
		rt.revokeMCPGrant(r.Context(), st, g.ID)
		invalid("the approving user no longer holds the granted scopes")
		return
	}
	access, tokenID, refresh, accessExp, err := rt.mcpMintPair(r.Context(), client, *g, now)
	if err != nil {
		rt.internalError(w, "api: mcp oauth refresh: mint failed", err)
		return
	}
	refreshExp := mcpBoundedExpiry(now, envMCPOAuthRefreshTTL, mcpOAuthDefaultRefreshTTL, g.ExpiresAt)
	outcome, _, err := st.RotateMCPOAuthRefresh(r.Context(), oldHash, sha256Hex(refresh), refreshExp, tokenID, now.Unix())
	if err != nil || outcome != store.MCPRefreshOK {
		_ = rt.libraryRevokeToken(r.Context(), tokenID)
		if err != nil {
			rt.internalError(w, "api: mcp oauth refresh: rotate failed", err)
			return
		}
		if outcome == store.MCPRefreshReused {
			rt.revokeMCPGrant(r.Context(), st, g.ID)
			rt.mcpOAuthAudit(r.Context(), r, AuditActionMCPOAuthReuse, "token", g.ID, map[string]any{"client_id": client.ID, "kind": "refresh"})
		}
		invalid("the refresh token is invalid, expired or already used")
		return
	}
	writeMCPTokens(w, access, refresh, accessExp.Sub(now), g.Scopes)
}

func writeMCPTokens(w http.ResponseWriter, access, refresh string, ttl time.Duration, scopes []string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	scope := ""
	for i, s := range scopes {
		if i > 0 {
			scope += " "
		}
		scope += s
	}
	writeJSON(w, http.StatusOK, mcpTokenResponse{AccessToken: access, TokenType: "Bearer", ExpiresIn: int(ttl.Seconds()), RefreshToken: refresh, Scope: scope})
}
