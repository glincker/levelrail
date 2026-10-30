package api

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// oauthRedirect appends params to a client redirect URI, keeping its query.
func oauthRedirect(base string, params map[string]string) string {
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	q := u.Query()
	for k, v := range params {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func oauthErrorBody(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, map[string]string{"error": code, "error_description": desc})
}

// handleMCPAuthorize validates an authorization request, parks it server side
// and sends the browser to the consent page. Nothing from the query is trusted
// again at approval time.
func (rt *Router) handleMCPAuthorize(w http.ResponseWriter, r *http.Request) {
	st, issuer, ok := rt.mcpOAuthReady(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	client, err := st.GetMCPOAuthClient(r.Context(), q.Get("client_id"))
	if errors.Is(err, store.ErrMCPOAuthNotFound) {
		oauthErrorBody(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}
	if err != nil {
		rt.internalError(w, "api: mcp oauth authorize: load client failed", err)
		return
	}
	redirect := q.Get("redirect_uri")
	if redirect == "" && len(client.RedirectURIs) == 1 {
		redirect = client.RedirectURIs[0]
	}
	if !redirectMatches(client.RedirectURIs, redirect) {
		oauthErrorBody(w, http.StatusBadRequest, "invalid_request", "redirect_uri does not match a registered URI")
		return
	}
	state := q.Get("state")
	fail := func(code, desc string) {
		http.Redirect(w, r, oauthRedirect(redirect, map[string]string{"error": code, "error_description": desc, "state": state, "iss": issuer}), http.StatusFound)
	}
	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "response_type must be code")
		return
	}
	if q.Get("code_challenge_method") != "S256" || !validPKCEChallenge(q.Get("code_challenge")) {
		fail("invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	scopes, valid := parseMCPScopes(q.Get("scope"))
	if !valid {
		fail("invalid_scope", "only mcp:read and mcp:read:sensitive can be requested")
		return
	}
	for _, s := range scopes {
		if !slices.Contains(client.Scopes, s) {
			fail("invalid_scope", "scope "+s+" is not allowed for this client")
			return
		}
	}
	resource := mcpResourceURL(issuer)
	if got := q.Get("resource"); got != "" && got != resource {
		fail("invalid_target", "resource must be "+resource)
		return
	}
	id, err := randomURLToken(24)
	if err != nil {
		rt.internalError(w, "api: mcp oauth authorize: id generation failed", err)
		return
	}
	now := time.Now()
	req := store.MCPOAuthRequest{
		ID: id, ClientID: client.ID, RedirectURI: redirect, CodeChallenge: q.Get("code_challenge"),
		Scopes: scopes, Resource: resource, State: state,
		ExpiresAt: now.Add(envDuration(envMCPOAuthRequestTTL, mcpOAuthDefaultRequestTTL)).Unix(),
	}
	if err := st.CreateMCPOAuthRequest(r.Context(), req, now.Unix()); err != nil {
		rt.internalError(w, "api: mcp oauth authorize: save request failed", err)
		return
	}
	http.Redirect(w, r, issuer+mcpOAuthConsentPath+"?request="+url.QueryEscape(id), http.StatusFound)
}

type mcpConsentResource struct {
	ID               string   `json:"id"`
	ClientID         string   `json:"client_id"`
	ClientName       string   `json:"client_name"`
	RedirectHost     string   `json:"redirect_host"`
	RequestedScopes  []string `json:"requested_scopes"`
	GrantableScopes  []string `json:"grantable_scopes"`
	Resource         string   `json:"resource"`
	AccessTTLSeconds int      `json:"access_ttl_seconds"`
	RefreshTTLSecond int      `json:"refresh_ttl_seconds"`
	ExpiresAt        string   `json:"expires_at"`
}

// loadConsent resolves a pending request for the signed-in user.
func (rt *Router) loadConsent(w http.ResponseWriter, r *http.Request) (mcpOAuthStore, *store.MCPOAuthRequest, *store.MCPOAuthClient, *store.User, bool) {
	st := rt.mcpOAuth()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "OAuth for remote MCP is not available on this control plane")
		return nil, nil, nil, nil, false
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return nil, nil, nil, nil, false
	}
	req, err := st.GetMCPOAuthRequest(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrMCPOAuthNotFound) || (err == nil && (req.Decided || req.ExpiresAt <= nowUnix())) {
		writeError(w, http.StatusNotFound, "no pending authorization request with this id")
		return nil, nil, nil, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: mcp oauth consent: load request failed", err)
		return nil, nil, nil, nil, false
	}
	client, err := st.GetMCPOAuthClient(r.Context(), req.ClientID)
	if err != nil {
		writeError(w, http.StatusNotFound, "the client of this request no longer exists")
		return nil, nil, nil, nil, false
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: mcp oauth consent: load user failed", err)
		return nil, nil, nil, nil, false
	}
	return st, req, client, user, true
}

// handleMCPConsentGet shows what an approval would grant, already capped at
// the signed-in user's own abilities.
func (rt *Router) handleMCPConsentGet(w http.ResponseWriter, r *http.Request) {
	_, req, client, user, ok := rt.loadConsent(w, r)
	if !ok {
		return
	}
	host := ""
	if u, err := url.Parse(req.RedirectURI); err == nil {
		host = u.Host
	}
	writeJSON(w, http.StatusOK, mcpConsentResource{
		ID: req.ID, ClientID: client.ID, ClientName: client.Name, RedirectHost: host,
		RequestedScopes: req.Scopes, GrantableScopes: mcpGrantable(req.Scopes, user.Abilities), Resource: req.Resource,
		AccessTTLSeconds: int(envDuration(envMCPOAuthAccessTTL, mcpOAuthDefaultAccessTTL).Seconds()),
		RefreshTTLSecond: int(envDuration(envMCPOAuthRefreshTTL, mcpOAuthDefaultRefreshTTL).Seconds()),
		ExpiresAt:        time.Unix(req.ExpiresAt, 0).UTC().Format(time.RFC3339),
	})
}

type mcpConsentDecision struct {
	Scopes []string `json:"scopes"`
}

type mcpConsentResult struct {
	RedirectTo string `json:"redirect_to"`
}

func (rt *Router) handleMCPConsentApprove(w http.ResponseWriter, r *http.Request) {
	rt.decideMCPConsent(w, r, true)
}

func (rt *Router) handleMCPConsentDeny(w http.ResponseWriter, r *http.Request) {
	rt.decideMCPConsent(w, r, false)
}

func (rt *Router) decideMCPConsent(w http.ResponseWriter, r *http.Request, approve bool) {
	if refuseUnsafeSignInPOST(w, r) {
		return
	}
	st, req, client, user, ok := rt.loadConsent(w, r)
	if !ok {
		return
	}
	issuer, _ := mcpIssuer(r)
	now := time.Now()
	if !approve {
		if _, err := st.DecideMCPOAuthRequest(r.Context(), req.ID, user.ID, false, nil, "", 0, now.Unix()); err != nil {
			rt.internalError(w, "api: mcp oauth consent: deny failed", err)
			return
		}
		rt.mcpOAuthAudit(r.Context(), r, AuditActionMCPOAuthDenied, auditActorSession, user.ID, map[string]any{"client_id": client.ID})
		writeJSON(w, http.StatusOK, mcpConsentResult{RedirectTo: oauthRedirect(req.RedirectURI, map[string]string{"error": "access_denied", "state": req.State, "iss": issuer})})
		return
	}
	var body mcpConsentDecision
	_ = decodeJSONBody(r, &body)
	grantable := mcpGrantable(req.Scopes, user.Abilities)
	chosen := grantable
	if len(body.Scopes) > 0 {
		chosen = nil
		for _, s := range body.Scopes {
			if !slices.Contains(grantable, s) {
				writeError(w, http.StatusForbidden, "scope "+s+" cannot be granted: it was not requested or your account lacks it")
				return
			}
			chosen = append(chosen, s)
		}
	}
	if len(chosen) == 0 {
		writeError(w, http.StatusForbidden, "your account holds none of the requested scopes")
		return
	}
	code, err := randomURLToken(32)
	if err != nil {
		rt.internalError(w, "api: mcp oauth consent: code generation failed", err)
		return
	}
	exp := now.Add(envDuration(envMCPOAuthCodeTTL, mcpOAuthDefaultCodeTTL)).Unix()
	won, err := st.DecideMCPOAuthRequest(r.Context(), req.ID, user.ID, true, chosen, sha256Hex(code), exp, now.Unix())
	if err != nil {
		rt.internalError(w, "api: mcp oauth consent: approve failed", err)
		return
	}
	if !won {
		writeError(w, http.StatusConflict, "this request was already decided or has expired")
		return
	}
	rt.mcpOAuthAudit(r.Context(), r, AuditActionMCPOAuthGrant, auditActorSession, user.ID, map[string]any{"client_id": client.ID, "scopes": chosen, "stage": "approved"})
	writeJSON(w, http.StatusOK, mcpConsentResult{RedirectTo: oauthRedirect(req.RedirectURI, map[string]string{"code": code, "state": req.State, "iss": issuer})})
}
