package api

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

type gateLoginRequired struct {
	Error    string `json:"error"`
	LoginURL string `json:"login_url,omitempty"`
}

// handleGateCheck handles GET /api/v1/gate/check, the built-in provider's
// forward auth endpoint. Only this process's ingress can call it: a
// request without the gate secret gets a plain 404.
func (rt *Router) handleGateCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !rt.gateRequestAuthentic(r) {
		http.NotFound(w, r)
		return
	}
	req, err := parseGateRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid gate request")
		return
	}
	ctx := r.Context()
	g, found, err := rt.gate.GetDomainAuthGate(ctx, req.host)
	if err != nil {
		rt.internalError(w, "api: gate check: load gate failed", err, slog.String("domain", req.host))
		return
	}
	if !found || !g.Enabled || g.Provider != store.GateProviderBuiltin {
		rt.gateRefuse(w, r, req, nil, gateReasonNotBuiltin)
		return
	}
	if !req.https && !rt.effectiveGateSettings().AllowHTTP {
		rt.gateRefuse(w, r, req, nil, gateReasonHTTPSRequired)
		return
	}
	if req.token != "" {
		rt.gateExchange(w, r, req)
		return
	}
	reason := gateReasonNoSession
	if c, err := r.Cookie(gateCookieName(gateCookieSession, req.https)); err == nil && c.Value != "" {
		user, why, err := rt.gateSessionUser(r, req, c.Value)
		if err != nil {
			rt.internalError(w, "api: gate check: session lookup failed", err, slog.String("domain", req.host))
			return
		}
		if user != nil {
			rt.gateAuthorizeAndAllow(w, r, req, user)
			return
		}
		reason = why
		setGateCookie(w, gateCookieSession, "", req.https, 0)
	}
	rt.gateLoginRedirect(w, r, req, reason)
}

// gateSessionUser resolves a gate session cookie. A nil user with a reason
// means the cookie is no longer good; revocations are logged here.
func (rt *Router) gateSessionUser(r *http.Request, req gateRequest, token string) (*store.User, string, error) {
	if !validGateTokenShape(token) {
		return nil, gateReasonNoSession, nil
	}
	ctx := r.Context()
	s, found, err := rt.gate.GetGateSession(ctx, hashGateToken(token))
	if err != nil || !found {
		return nil, gateReasonNoSession, err
	}
	reason := ""
	switch {
	case s.Domain != req.host:
		reason = gateReasonWrongHost
	case s.Revoked:
		reason = gateReasonSessionRevoked
	case time.Now().After(s.ExpiresAt):
		reason = gateReasonSessionExpired
	}
	if reason == "" {
		live, err := rt.gate.DashboardSessionLive(ctx, s.ParentSessionID)
		if err != nil {
			return nil, "", err
		}
		if !live {
			reason = gateReasonSignedOut
		}
	}
	var user *store.User
	if reason == "" {
		user, err = rt.gate.GetUserByID(ctx, s.UserID)
		if errors.Is(err, store.ErrUserNotFound) {
			reason, err = gateReasonUserRemoved, nil
		}
		if err != nil {
			return nil, "", err
		}
	}
	if reason != "" {
		rt.recordGateEvent(ctx, store.GateEvent{Domain: req.host, UserID: s.UserID, Decision: store.GateDecisionSessionRevoked, Reason: reason, IP: req.ip})
		return nil, reason, nil
	}
	return user, "", nil
}

func (rt *Router) gateAuthorizeAndAllow(w http.ResponseWriter, r *http.Request, req gateRequest, user *store.User) {
	allowed, reason, err := rt.authorizeGateUser(r.Context(), req.host, user)
	if err != nil {
		rt.internalError(w, "api: gate check: authorize failed", err, slog.String("domain", req.host))
		return
	}
	if !allowed {
		rt.gateRefuse(w, r, req, user, reason)
		return
	}
	rt.recordGateCoalesced(r.Context(), store.GateEvent{Domain: req.host, UserID: user.ID, UserEmail: user.Email, Decision: store.GateDecisionAllowed, Reason: reason, IP: req.ip})
	w.Header().Set("X-User", user.Email)
	w.Header().Set("X-Email", user.Email)
	w.Header().Set("X-Name", user.DisplayName)
	w.WriteHeader(http.StatusOK)
}

// gateRefuse answers 403 without a redirect, so a denied user never loops
// between the app and the dashboard.
func (rt *Router) gateRefuse(w http.ResponseWriter, r *http.Request, req gateRequest, user *store.User, reason string) {
	e := store.GateEvent{Domain: req.host, Decision: store.GateDecisionDenied, Reason: reason, IP: req.ip}
	if user != nil {
		e.UserID, e.UserEmail = user.ID, user.Email
	}
	rt.recordGateCoalesced(r.Context(), e)
	if req.xhr {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusForbidden, gateLoginRequired{Error: "access denied"})
		return
	}
	msg := fmt.Sprintf("You do not have access to %s. Ask an administrator to add you.", req.host)
	if user != nil {
		msg = fmt.Sprintf("You are signed in as %s, but you do not have access to %s. Ask an administrator to add you.", user.Email, req.host)
	}
	if reason == gateReasonHTTPSRequired {
		msg = fmt.Sprintf("Sign-in for %s needs HTTPS.", req.host)
	}
	rt.writeGatePage(w, http.StatusForbidden, gatePage{Title: "Access denied", Message: msg})
}

func (rt *Router) gateLoginRedirect(w http.ResponseWriter, r *http.Request, req gateRequest, reason string) {
	ctx := r.Context()
	rt.recordGateCoalesced(ctx, store.GateEvent{Domain: req.host, Decision: store.GateDecisionLoginRedirect, Reason: reason, IP: req.ip})
	base, err := rt.controlPlaneBaseURL(ctx)
	if err != nil {
		rt.logger.Warn("api: gate check: no dashboard address for sign-in", slog.String("domain", req.host), slog.String("error", err.Error()))
		if req.xhr {
			writeJSON(w, http.StatusServiceUnavailable, gateLoginRequired{Error: "sign-in is not available"})
			return
		}
		rt.writeGatePage(w, http.StatusServiceUnavailable, gatePage{Title: "Sign-in unavailable", Message: "Sign-in is not set up for this site yet. Ask an administrator to set the platform's primary domain."})
		return
	}
	state, _, err := newGateSecretToken()
	if err != nil {
		rt.internalError(w, "api: gate check: mint state failed", err)
		return
	}
	scheme := "http"
	if req.https {
		scheme = "https"
	}
	rd := scheme + "://" + strings.ToLower(req.authority) + req.uri
	loginURL := base + "/gate/login?" + url.Values{"rd": {rd}, "state": {state}}.Encode()
	setGateCookie(w, gateCookieState, state, req.https, gateStateMaxAge)
	if req.xhr {
		writeJSON(w, http.StatusUnauthorized, gateLoginRequired{Error: "sign in required", LoginURL: loginURL})
		return
	}
	w.Header().Set("Location", loginURL)
	w.WriteHeader(http.StatusFound)
}

// gateExchange trades a single-use handoff token for a host-bound gate
// session, then redirects to the same URL without the token.
func (rt *Router) gateExchange(w http.ResponseWriter, r *http.Request, req gateRequest) {
	ctx := r.Context()
	fail := func(reason string, userID string) {
		rt.recordGateEvent(ctx, store.GateEvent{Domain: req.host, UserID: userID, Decision: store.GateDecisionDenied, Reason: reason, IP: req.ip})
		setGateCookie(w, gateCookieState, "", req.https, 0)
		rt.writeGatePage(w, http.StatusForbidden, gatePage{
			Title: "Sign-in could not be completed", Message: "The sign-in link was already used, expired, or was opened in a different browser.",
			Link: req.uri, LinkLabel: "Try again",
		})
	}
	if !validGateTokenShape(req.token) {
		fail(gateReasonTokenInvalid, "")
		return
	}
	t, found, err := rt.gate.ConsumeGateToken(ctx, hashGateToken(req.token))
	if err != nil {
		rt.internalError(w, "api: gate exchange: consume token failed", err, slog.String("domain", req.host))
		return
	}
	switch {
	case !found:
		fail(gateReasonTokenInvalid, "")
		return
	case time.Now().After(t.ExpiresAt):
		fail(gateReasonTokenExpired, t.UserID)
		return
	case t.Domain != req.host:
		fail(gateReasonWrongHost, t.UserID)
		return
	}
	sc, err := r.Cookie(gateCookieName(gateCookieState, req.https))
	if err != nil || subtle.ConstantTimeCompare([]byte(hashGateToken(sc.Value)), []byte(t.StateHash)) != 1 {
		fail(gateReasonStateMismatch, t.UserID)
		return
	}
	live, err := rt.gate.DashboardSessionLive(ctx, t.ParentSessionID)
	if err != nil {
		rt.internalError(w, "api: gate exchange: dashboard session check failed", err)
		return
	}
	if !live {
		fail(gateReasonSignedOut, t.UserID)
		return
	}
	user, err := rt.gate.GetUserByID(ctx, t.UserID)
	if errors.Is(err, store.ErrUserNotFound) {
		fail(gateReasonUserRemoved, t.UserID)
		return
	}
	if err != nil {
		rt.internalError(w, "api: gate exchange: load user failed", err)
		return
	}
	allowed, reason, err := rt.authorizeGateUser(ctx, req.host, user)
	if err != nil {
		rt.internalError(w, "api: gate exchange: authorize failed", err)
		return
	}
	if !allowed {
		setGateCookie(w, gateCookieState, "", req.https, 0)
		rt.gateRefuse(w, r, req, user, reason)
		return
	}
	token, hash, err := newGateSecretToken()
	if err != nil {
		rt.internalError(w, "api: gate exchange: mint session failed", err)
		return
	}
	ttl := rt.effectiveGateSettings().SessionTTL
	if err := rt.gate.CreateGateSession(ctx, store.GateSession{TokenHash: hash, Domain: req.host, UserID: user.ID, ParentSessionID: t.ParentSessionID, ExpiresAt: time.Now().Add(ttl)}); err != nil {
		rt.internalError(w, "api: gate exchange: save session failed", err)
		return
	}
	rt.recordGateEvent(ctx, store.GateEvent{Domain: req.host, UserID: user.ID, UserEmail: user.Email, Decision: store.GateDecisionTokenExchange, Reason: reason, IP: req.ip})
	setGateCookie(w, gateCookieSession, token, req.https, ttl)
	setGateCookie(w, gateCookieState, "", req.https, 0)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Location", req.uri)
	w.WriteHeader(http.StatusFound)
}

// dashboardSessionInfo resolves the dashboard sign-in on r. It fails closed
// on a session still waiting for its second factor.
func (rt *Router) dashboardSessionInfo(r *http.Request) (authengine.SessionInfo, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" || rt.sessions == nil || rt.sessions.lib == nil {
		return authengine.SessionInfo{}, false
	}
	info, ok := rt.sessions.lib.Lookup(r.Context(), c.Value)
	if !ok || info.LegacyUserID == "" || info.SessionID == "" {
		return authengine.SessionInfo{}, false
	}
	return info, true
}

// handleGateLogin handles GET /gate/login on the dashboard host: a signed
// in, authorized user is sent back to the gated app with a handoff token.
func (rt *Router) handleGateLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if rt.gate == nil || rt.gateSecret == "" {
		rt.writeGatePage(w, http.StatusServiceUnavailable, gatePage{Title: "Sign-in unavailable", Message: "App sign-in is not enabled on this server."})
		return
	}
	ctx := r.Context()
	q := r.URL.Query()
	state := q.Get("state")
	target, host, err := rt.validateGateRedirect(ctx, q.Get("rd"))
	if err == nil && !validGateTokenShape(state) {
		err = errGateBadRedirect
	}
	if errors.Is(err, errGateBadRedirect) {
		rt.writeGatePage(w, http.StatusBadRequest, gatePage{Title: "Invalid sign-in link", Message: "This sign-in link does not point to an app protected by this server."})
		return
	}
	if err != nil {
		rt.internalError(w, "api: gate login: validate redirect failed", err)
		return
	}
	info, ok := rt.dashboardSessionInfo(r)
	if !ok {
		back := "/gate/login?" + url.Values{"rd": {q.Get("rd")}, "state": {state}}.Encode()
		w.Header().Set("Location", "/login?"+url.Values{"redirect": {back}}.Encode())
		w.WriteHeader(http.StatusFound)
		return
	}
	user, err := rt.gate.GetUserByID(ctx, info.LegacyUserID)
	if err != nil {
		rt.internalError(w, "api: gate login: load user failed", err)
		return
	}
	allowed, reason, err := rt.authorizeGateUser(ctx, host, user)
	if err != nil {
		rt.internalError(w, "api: gate login: authorize failed", err)
		return
	}
	ip := clientIP(r)
	if !allowed {
		rt.recordGateEvent(ctx, store.GateEvent{Domain: host, UserID: user.ID, UserEmail: user.Email, Decision: store.GateDecisionDenied, Reason: reason, IP: ip})
		rt.writeGatePage(w, http.StatusForbidden, gatePage{Title: "Access denied", Message: fmt.Sprintf("You are signed in as %s, but you do not have access to %s. Ask an administrator to add you.", user.Email, host)})
		return
	}
	token, hash, err := newGateSecretToken()
	if err != nil {
		rt.internalError(w, "api: gate login: mint token failed", err)
		return
	}
	if err := rt.gate.CreateGateToken(ctx, store.GateToken{
		TokenHash: hash, Domain: host, UserID: user.ID, ParentSessionID: info.SessionID,
		StateHash: hashGateToken(state), ExpiresAt: time.Now().Add(rt.effectiveGateSettings().TokenTTL),
	}); err != nil {
		rt.internalError(w, "api: gate login: save token failed", err)
		return
	}
	rt.recordGateEvent(ctx, store.GateEvent{Domain: host, UserID: user.ID, UserEmail: user.Email, Decision: store.GateDecisionTokenIssued, Reason: reason, IP: ip})
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	w.Header().Set("Location", target+sep+gateTokenParam+"="+token)
	w.WriteHeader(http.StatusFound)
}
