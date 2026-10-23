package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	samlFlowCookie       = "saml_flow"
	samlRequestTTL       = 10 * time.Minute
	samlMaxInFlight      = 10000
	samlMaxFormBytes     = 300 << 10
	samlCompleteRedirect = "/oauth/complete"

	samlErrUnavailable    = "saml_unavailable"
	samlErrInvalidState   = "invalid_state"
	samlErrInvalid        = "saml_invalid"
	samlErrRateLimited    = "rate_limited"
	samlErrInternal       = "internal_error"
	samlErrNotProvisioned = "sso_not_provisioned"
	samlErrDomain         = "domain_not_allowed"
	samlErrNoRole         = "sso_no_role"
	samlErrNoEmail        = "sso_no_email"
	samlErrDisabled       = "account_disabled"

	auditActionSAMLSignIn   = "sso.saml.signed_in"
	auditActionSAMLRejected = "sso.saml.rejected"
	auditActorTypeSession   = "session"
)

// samlFlowStore holds outstanding AuthnRequest IDs bound to a browser secret.
// Each one is consumed once, which is what refuses a replayed response.
type samlFlowStore struct {
	mu      sync.Mutex
	pending map[string]samlFlow
}

type samlFlow struct {
	connectionID string
	requestID    string
	testUser     string
	expires      time.Time
}

func newSAMLFlowStore() *samlFlowStore { return &samlFlowStore{pending: map[string]samlFlow{}} }

func (s *samlFlowStore) put(browserHash string, f samlFlow, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) >= samlMaxInFlight {
		for k, v := range s.pending {
			if now.After(v.expires) {
				delete(s.pending, k)
			}
		}
		if len(s.pending) >= samlMaxInFlight {
			return false
		}
	}
	s.pending[browserHash] = f
	return true
}

func (s *samlFlowStore) consume(browserHash, connectionID string, now time.Time) (samlFlow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.pending[browserHash]
	delete(s.pending, browserHash)
	if !ok || f.connectionID != connectionID || now.After(f.expires) {
		return samlFlow{}, false
	}
	return f, true
}

type publicSAMLProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func samlUsable(c store.SSOConnection) bool { return c.Enabled && c.LibraryID != "" }

// handleListPublicSAMLProviders lists enabled, fully configured connections for the login screen.
func (rt *Router) handleListPublicSAMLProviders(w http.ResponseWriter, r *http.Request) {
	out := []publicSAMLProvider{}
	if rt.sso == nil || rt.authEngine == nil || !rt.authEngine.SAMLEnabled() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	conns, err := rt.sso.ListSSOConnections(r.Context())
	if err != nil {
		rt.internalError(w, "api: list public saml providers failed", err)
		return
	}
	for _, c := range conns {
		if samlUsable(c) {
			out = append(out, publicSAMLProvider{ID: c.ID, Name: c.Name})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSAMLMetadata serves the library's SP metadata for one connection.
func (rt *Router) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	if !rt.samlReady(w) {
		return
	}
	c, err := rt.sso.GetSSOConnection(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrSSOConnectionNotFound) || (err == nil && c.LibraryID == "") {
		writeError(w, http.StatusNotFound, "saml connection not found or not configured")
		return
	}
	if err != nil {
		rt.internalError(w, "api: saml metadata: load connection failed", err)
		return
	}
	if err := rt.syncSAMLEndpoints(r.Context(), c); err != nil {
		if errors.Is(err, errSAMLNeedsHTTPS) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		rt.internalError(w, "api: saml metadata: sync endpoints failed", err, slog.String("connection_id", c.ID))
		return
	}
	md, err := rt.authEngine.SAMLMetadata(r.Context(), c.LibraryID)
	if err != nil {
		rt.internalError(w, "api: render saml metadata failed", err, slog.String("connection_id", c.ID))
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write(md)
}

func redirectSAMLError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?oauth_error="+url.QueryEscape(code), http.StatusFound)
}

// samlFlowConnection loads an enabled, configured connection for the public
// flow, answering with a login-screen redirect when it cannot be used.
func (rt *Router) samlFlowConnection(w http.ResponseWriter, r *http.Request) (store.SSOConnection, bool) {
	if ok, _ := rt.samlLimiter.allow(clientIP(r)); !ok {
		redirectSAMLError(w, r, samlErrRateLimited)
		return store.SSOConnection{}, false
	}
	if rt.sso == nil || rt.authEngine == nil || !rt.authEngine.SAMLEnabled() || !requestIsHTTPS(r) {
		redirectSAMLError(w, r, samlErrUnavailable)
		return store.SSOConnection{}, false
	}
	c, err := rt.sso.GetSSOConnection(r.Context(), r.PathValue("id"))
	if err != nil || !samlUsable(c) {
		redirectSAMLError(w, r, samlErrUnavailable)
		return store.SSOConnection{}, false
	}
	if err := rt.syncSAMLEndpoints(r.Context(), c); err != nil {
		rt.logger.Error("api: saml: sync endpoints failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		redirectSAMLError(w, r, samlErrUnavailable)
		return store.SSOConnection{}, false
	}
	return c, true
}

// handleSAMLLogin starts SP-initiated sign-in through the library and binds
// the AuthnRequest ID to a fresh browser secret, so only this browser can finish it.
func (rt *Router) handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.samlFlowConnection(w, r)
	if !ok {
		return
	}
	rt.beginSAMLFlow(w, r, c, "")
}

// beginSAMLFlow sends the browser to the IdP. A non-empty testUser marks a
// dry run for that root user: the response is evaluated, never signed in.
func (rt *Router) beginSAMLFlow(w http.ResponseWriter, r *http.Request, c store.SSOConnection, testUser string) {
	redirect, requestID, err := rt.authEngine.BeginSAMLLogin(r.Context(), c.LibraryID)
	if err != nil {
		rt.logger.Error("api: saml login: begin failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		redirectSAMLError(w, r, samlErrInternal)
		return
	}
	secret, err := randomToken()
	if err != nil {
		redirectSAMLError(w, r, samlErrInternal)
		return
	}
	now := time.Now()
	if !rt.samlFlows.put(hashToken(secret), samlFlow{connectionID: c.ID, requestID: requestID, testUser: testUser, expires: now.Add(samlRequestTTL)}, now) {
		redirectSAMLError(w, r, samlErrRateLimited)
		return
	}
	setSAMLFlowCookie(w, c.ID, secret, int(samlRequestTTL/time.Second))
	http.Redirect(w, r, redirect, http.StatusFound)
}

// SameSite=None because the IdP posts the response cross-site; Secure is
// mandatory with it, which is why the flow refuses plain HTTP.
func setSAMLFlowCookie(w http.ResponseWriter, connectionID, value string, maxAge int) {
	c := &http.Cookie{ //nolint:gosec // SameSite=None is required for the IdP cross-site POST; Secure and HttpOnly are set
		Name: samlFlowCookie, Value: value, Path: samlPathPrefix + url.PathEscape(connectionID) + "/acs",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteNoneMode, MaxAge: maxAge,
	}
	if value == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

// handleSAMLACS consumes the browser-bound request, runs the strict gate,
// settles the platform user, and only then lets the library link the
// identity and issue the session.
func (rt *Router) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.samlFlowConnection(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, samlMaxFormBytes)
	samlResponse := r.PostFormValue("SAMLResponse")
	cookie, cerr := r.Cookie(samlFlowCookie)
	setSAMLFlowCookie(w, c.ID, "", 0)
	if cerr != nil || cookie.Value == "" {
		rt.samlReject(w, r, c, samlErrInvalidState, "no flow cookie")
		return
	}
	flow, ok := rt.samlFlows.consume(hashToken(cookie.Value), c.ID, time.Now())
	if !ok {
		rt.samlReject(w, r, c, samlErrInvalidState, "no outstanding request for this browser")
		return
	}
	if flow.testUser != "" {
		rt.finishSAMLTest(w, r, c, flow, samlResponse)
		return
	}
	requestID := flow.requestID
	ident, err := rt.authEngine.VerifySAMLResponse(ctx, c.LibraryID, samlResponse, requestID)
	if err != nil {
		rt.samlReject(w, r, c, samlErrInvalid, err.Error())
		return
	}
	plan, code, err := rt.samlPlanSignIn(ctx, c, ident)
	if code != "" {
		detail := code
		if err != nil {
			detail = err.Error()
		}
		rt.samlReject(w, r, c, code, detail)
		return
	}
	token, engineUser, err := rt.authEngine.FinishSAMLLogin(ctx, c.LibraryID, samlResponse, r.UserAgent(), clientIP(r))
	if err != nil {
		rt.samlRollback(ctx, plan)
		code := samlErrInvalid
		if authengine.IsCode(err, authengine.CodeAccountDisabled) {
			code = samlErrDisabled
		}
		rt.samlReject(w, r, c, code, err.Error())
		return
	}
	user, code, err := rt.samlSettleUser(ctx, c, plan, engineUser)
	if code != "" {
		rt.libSessions.Revoke(ctx, token)
		rt.samlRollback(ctx, plan)
		detail := code
		if err != nil {
			detail = err.Error()
		}
		rt.samlReject(w, r, c, code, detail)
		return
	}
	rt.finishSAMLSignIn(w, r, c, user, token)
}

func (rt *Router) finishSAMLSignIn(w http.ResponseWriter, r *http.Request, c store.SSOConnection, user store.User, token string) {
	ctx := r.Context()
	hold, err := rt.holdForApproval(w, r, user, token, signInMethodSAML)
	switch {
	case err != nil:
		rt.libSessions.Revoke(ctx, token)
		rt.samlReject(w, r, c, samlErrInternal, err.Error())
		return
	case hold != nil && hold.limited:
		redirectSAMLError(w, r, samlErrRateLimited)
		return
	case hold != nil:
		http.Redirect(w, r, "/login?approval="+url.QueryEscape(hold.approval.ID), http.StatusFound)
		return
	}
	if sid, ok := rt.libSessions.SessionIDFor(ctx, token); ok && rt.accessPolicy != nil {
		if err := rt.accessPolicy.SetSessionMethod(ctx, sid, user.ID, signInMethodSAML, time.Now()); err != nil {
			rt.logger.Warn("api: saml: record session method failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
		}
	}
	rt.finishLibSession(w, r, user, token)
	rt.auditSignIn(ctx, r, signInActor{kind: auditActorTypeSession, id: user.ID, name: user.Email}, auditActionSAMLSignIn, samlPathPrefix+c.ID+"/acs", http.StatusFound)
	http.Redirect(w, r, samlCompleteRedirect, http.StatusFound)
}

// samlReject audits a refused sign-in with its internal reason, then sends the
// browser to the login screen with only the short public code.
func (rt *Router) samlReject(w http.ResponseWriter, r *http.Request, c store.SSOConnection, code, detail string) {
	rt.logger.Warn("api: saml sign-in refused", slog.String("connection_id", c.ID), slog.String("code", code), slog.String("detail", detail))
	rt.auditSignIn(r.Context(), r, anonymousSignIn(""), auditActionSAMLRejected, samlPathPrefix+c.ID+"/acs?code="+code, http.StatusForbidden)
	redirectSAMLError(w, r, code)
}
