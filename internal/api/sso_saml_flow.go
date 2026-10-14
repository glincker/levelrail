package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/samlsso"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	samlFlowCookie       = "saml_flow"
	samlRequestTTL       = 10 * time.Minute
	signInMethodSAML     = "saml"
	samlCompleteRedirect = "/oauth/complete"

	samlErrUnavailable    = "saml_unavailable"
	samlErrInvalidState   = "invalid_state"
	samlErrInvalid        = "saml_invalid"
	samlErrReplayed       = "saml_replayed"
	samlErrRateLimited    = "rate_limited"
	samlErrInternal       = "internal_error"
	samlErrNotProvisioned = "sso_not_provisioned"
	samlErrDomain         = "domain_not_allowed"
	samlErrNoRole         = "sso_no_role"
	samlErrNoEmail        = "sso_no_email"
	samlErrLinked         = "already_linked"
	samlErrDisabled       = "account_disabled"

	auditActionSAMLSignIn   = "sso.saml.signed_in"
	auditActionSAMLRejected = "sso.saml.rejected"
	auditActorTypeSession   = "session"
)

type publicSAMLProvider struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// handleListPublicSAMLProviders lists enabled, fully configured connections for the login screen.
func (rt *Router) handleListPublicSAMLProviders(w http.ResponseWriter, r *http.Request) {
	out := []publicSAMLProvider{}
	if rt.sso == nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	conns, err := rt.sso.ListSAMLConnections(r.Context())
	if err != nil {
		rt.internalError(w, "api: list public saml providers failed", err)
		return
	}
	for _, c := range conns {
		if c.Enabled && samlConfigured(c) {
			out = append(out, publicSAMLProvider{ID: c.ID, Name: c.Name})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSAMLMetadata serves the SP metadata IDP administrators import.
func (rt *Router) handleSAMLMetadata(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.loadSAMLConnection(w, r)
	if !ok {
		return
	}
	sp, err := rt.samlSPFor(r.Context(), c)
	if err != nil {
		rt.writeSAMLSetupError(w, err)
		return
	}
	md, err := sp.MetadataXML()
	if err != nil {
		rt.internalError(w, "api: render saml metadata failed", err, slog.String("connection_id", c.ID))
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	_, _ = w.Write(md)
}

func (rt *Router) writeSAMLSetupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSAMLNeedsHTTPS):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, errSAMLNoMasterKey):
		writeError(w, http.StatusNotImplemented, err.Error())
	default:
		rt.internalError(w, "api: saml service provider setup failed", err)
	}
}

func redirectSAMLError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/login?oauth_error="+url.QueryEscape(code), http.StatusFound)
}

func (rt *Router) samlRateLimited(r *http.Request) bool {
	ok, _ := rt.samlLimiter.allow(clientIP(r))
	return !ok
}

// samlFlowConnection loads an enabled, configured connection for the public
// flow, answering with a login-screen redirect when it cannot be used.
func (rt *Router) samlFlowConnection(w http.ResponseWriter, r *http.Request) (store.SAMLConnection, bool) {
	if rt.samlRateLimited(r) {
		redirectSAMLError(w, r, samlErrRateLimited)
		return store.SAMLConnection{}, false
	}
	if rt.sso == nil || !requestIsHTTPS(r) {
		redirectSAMLError(w, r, samlErrUnavailable)
		return store.SAMLConnection{}, false
	}
	c, err := rt.sso.GetSAMLConnection(r.Context(), r.PathValue("id"))
	if err != nil || !c.Enabled || !samlConfigured(c) {
		redirectSAMLError(w, r, samlErrUnavailable)
		return store.SAMLConnection{}, false
	}
	return c, true
}

// handleSAMLLogin starts SP-initiated sign-in. The AuthnRequest ID is stored
// against a hash of a fresh browser secret, so only this browser can finish it.
func (rt *Router) handleSAMLLogin(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.samlFlowConnection(w, r)
	if !ok {
		return
	}
	sp, err := rt.samlSPFor(r.Context(), c)
	if err != nil {
		rt.logger.Error("api: saml login: service provider unavailable", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		redirectSAMLError(w, r, samlErrUnavailable)
		return
	}
	redirect, requestID, err := sp.BeginLogin()
	if err != nil {
		rt.logger.Error("api: saml login: build request failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		redirectSAMLError(w, r, samlErrInternal)
		return
	}
	secret, err := randomToken()
	if err != nil {
		redirectSAMLError(w, r, samlErrInternal)
		return
	}
	now := time.Now()
	if err := rt.sso.SaveSAMLAuthnRequest(r.Context(), requestID, c.ID, hashToken(secret), now.Add(samlRequestTTL), now); err != nil {
		rt.logger.Error("api: saml login: save request failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
		redirectSAMLError(w, r, samlErrInternal)
		return
	}
	setSAMLFlowCookie(w, c.ID, secret, int(samlRequestTTL/time.Second))
	http.Redirect(w, r, redirect, http.StatusFound)
}

// SameSite=None because the IDP posts the response cross-site; Secure is
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

// handleSAMLACS consumes the browser-bound request, validates the response,
// refuses a replayed assertion, resolves the user and only then issues a session.
func (rt *Router) handleSAMLACS(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.samlFlowConnection(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, samlsso.MaxResponseBytes+4096)
	samlResponse := r.PostFormValue("SAMLResponse")
	cookie, cerr := r.Cookie(samlFlowCookie)
	setSAMLFlowCookie(w, c.ID, "", 0)
	if cerr != nil || cookie.Value == "" {
		rt.samlReject(w, r, c, samlErrInvalidState, "no flow cookie")
		return
	}
	now := time.Now()
	requestID, err := rt.sso.ConsumeSAMLAuthnRequest(ctx, c.ID, hashToken(cookie.Value), now)
	if err != nil {
		rt.samlReject(w, r, c, samlErrInvalidState, err.Error())
		return
	}
	sp, err := rt.samlSPFor(ctx, c)
	if err != nil {
		rt.samlReject(w, r, c, samlErrUnavailable, err.Error())
		return
	}
	ident, err := sp.Validate(samlResponse, requestID, samlsso.Attributes{Email: c.EmailAttribute, Name: c.NameAttribute, Groups: c.GroupsAttribute})
	if err != nil {
		rt.samlReject(w, r, c, samlErrInvalid, err.Error())
		return
	}
	if err := rt.sso.RecordSAMLAssertion(ctx, c.ID, ident.AssertionID, ident.ReplayUntil, now); err != nil {
		code := samlErrInternal
		if errors.Is(err, store.ErrSAMLAssertionReplayed) {
			code = samlErrReplayed
		}
		rt.samlReject(w, r, c, code, err.Error())
		return
	}
	user, code, err := rt.samlResolveUser(ctx, c, ident)
	if code != "" {
		detail := code
		if err != nil {
			detail = err.Error()
		}
		rt.samlReject(w, r, c, code, detail)
		return
	}
	if rt.accountDisabled(ctx, user.ID) {
		rt.samlReject(w, r, c, samlErrDisabled, "account disabled")
		return
	}
	rt.finishSAMLSignIn(w, r, c, ident, user)
}

func (rt *Router) finishSAMLSignIn(w http.ResponseWriter, r *http.Request, c store.SAMLConnection, ident samlsso.Identity, user store.User) {
	ctx := r.Context()
	token, err := rt.libSessions.IssueSession(ctx, toLibUser(user), r.UserAgent(), clientIP(r))
	if err != nil {
		rt.samlReject(w, r, c, samlErrInternal, err.Error())
		return
	}
	hold, err := rt.holdForApproval(w, r, user, token, signInMethodSAML)
	switch {
	case err != nil:
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
	if err := rt.sso.TouchSAMLIdentity(ctx, c.ID, ident.NameID, time.Now()); err != nil {
		rt.logger.Warn("api: saml: touch identity failed", slog.String("connection_id", c.ID), slog.String("error", err.Error()))
	}
	rt.finishLibSession(w, r, user, token)
	rt.auditSignIn(ctx, r, signInActor{kind: auditActorTypeSession, id: user.ID, name: user.Email}, auditActionSAMLSignIn, samlPathPrefix+c.ID+"/acs", http.StatusFound)
	http.Redirect(w, r, samlCompleteRedirect, http.StatusFound)
}

// samlReject audits a refused sign-in with its internal reason, then sends the
// browser to the login screen with only the short public code.
func (rt *Router) samlReject(w http.ResponseWriter, r *http.Request, c store.SAMLConnection, code, detail string) {
	rt.logger.Warn("api: saml sign-in refused", slog.String("connection_id", c.ID), slog.String("code", code), slog.String("detail", detail))
	rt.auditSignIn(r.Context(), r, anonymousSignIn(""), auditActionSAMLRejected, samlPathPrefix+c.ID+"/acs?code="+code, http.StatusForbidden)
	redirectSAMLError(w, r, code)
}
