package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const msgNoApprovalWaiting = "no sign-in is waiting for approval in this browser"

// Statuses the waiting browser sees while it polls.
const (
	approvalPollPending  = "pending"
	approvalPollApproved = "approved"
	approvalPollDenied   = "denied"
	approvalPollExpired  = "expired"
)

type loginApprovalPollResponse struct {
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expires_at,omitzero"`
	Email       string    `json:"username,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	MFARequired bool      `json:"mfa_required,omitempty"`
	MFAToken    string    `json:"mfa_token,omitempty"`
}

// pauseForDeviceApproval holds a correct password login from an untrusted
// browser while another session of the account is live; with none it lets the
// login through so nobody is locked out. Reports whether it wrote a response.
func (rt *Router) pauseForDeviceApproval(w http.ResponseWriter, r *http.Request, user store.User, token string) bool {
	if !rt.newDeviceApproval || rt.trustedDeviceFor(r, user.ID) {
		return false
	}
	ctx := r.Context()
	others, err := rt.libSessions.OtherSessionCount(ctx, user.ID, token)
	if err != nil {
		rt.libSessions.Revoke(ctx, token)
		rt.internalError(w, "api: login: count other sessions failed", err, slog.String("user_id", user.ID))
		return true
	}
	if others == 0 {
		return false
	}
	rt.libSessions.Revoke(ctx, token)
	binding, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: login: generate approval binding failed", err)
		return true
	}
	id, err := randomOpaqueID("la_")
	if err != nil {
		rt.internalError(w, "api: login: generate approval id failed", err)
		return true
	}
	now := time.Now()
	ttl := rt.codeLogin.approvalTTL
	a := store.LoginApproval{
		ID: id, UserID: user.ID, BrowserHash: hashToken(binding), RequesterIP: clientIP(r),
		UserAgent: truncateUA(r.UserAgent()), CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := rt.loginCodes.CreateLoginApproval(ctx, a); err != nil {
		rt.internalError(w, "api: login: save approval failed", err, slog.String("user_id", user.ID))
		return true
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceRequest, approvalAuditPrefix+a.ID, http.StatusAccepted)
	setBindingCookie(w, r, approvalCookie, approvalCookiePath, binding, ttl)
	writeJSON(w, http.StatusOK, loginResponse{ApprovalRequired: true, ApprovalID: a.ID, ApprovalExpiresAt: &a.ExpiresAt})
	return true
}

// handlePollLoginApproval handles POST /api/v1/auth/login-approval/poll: the
// waiting browser's check, bound to its approval cookie. Once approved it
// issues the session exactly once.
func (rt *Router) handlePollLoginApproval(w http.ResponseWriter, r *http.Request) {
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	browser, ok := cookieHash(r, approvalCookie)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgNoApprovalWaiting)
		return
	}
	ctx := r.Context()
	a, err := rt.loginCodes.GetLoginApprovalByBrowser(ctx, browser)
	if errors.Is(err, store.ErrLoginApprovalNotFound) {
		writeError(w, http.StatusUnauthorized, msgNoApprovalWaiting)
		return
	}
	if err != nil {
		rt.internalError(w, "api: poll login approval failed", err)
		return
	}
	now := time.Now()
	switch {
	case a.Status == store.LoginApprovalPending && now.Before(a.ExpiresAt):
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollPending, ExpiresAt: a.ExpiresAt})
		return
	case a.Status == store.LoginApprovalDenied:
		setBindingCookie(w, r, approvalCookie, approvalCookiePath, "", 0)
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollDenied})
		return
	case a.Status != store.LoginApprovalApproved:
		setBindingCookie(w, r, approvalCookie, approvalCookiePath, "", 0)
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollExpired})
		return
	}
	consumed, err := rt.loginCodes.ConsumeLoginApproval(ctx, a.ID, now)
	if err != nil {
		rt.internalError(w, "api: consume login approval failed", err)
		return
	}
	setBindingCookie(w, r, approvalCookie, approvalCookiePath, "", 0)
	if !consumed {
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollExpired})
		return
	}
	user, err := rt.auth.GetUserByID(ctx, a.UserID)
	if err != nil {
		rt.internalError(w, "api: login approval: load user failed", err, slog.String("user_id", a.UserID))
		return
	}
	totpOn, err := rt.totpEnabled(ctx, user.ID)
	if err != nil {
		rt.internalError(w, "api: login approval: read two-factor status failed", err, slog.String("user_id", a.UserID))
		return
	}
	if totpOn {
		pending, perr := rt.mfaPending.create(user.ID)
		if perr != nil {
			rt.internalError(w, "api: login approval: create mfa pending token failed", perr, slog.String("user_id", a.UserID))
			return
		}
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollApproved, MFARequired: true, MFAToken: pending})
		return
	}
	if err := rt.establishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: login approval: establish session failed", err, slog.String("user_id", a.UserID))
		return
	}
	rt.trustDevice(w, r, user.ID)
	writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollApproved, Email: user.Email, DisplayName: user.DisplayName})
}

// handleApproveLoginApproval handles POST /api/v1/auth/login-approvals/{id}/approve.
func (rt *Router) handleApproveLoginApproval(w http.ResponseWriter, r *http.Request) {
	rt.decideLoginApproval(w, r, true)
}

// handleDenyLoginApproval handles POST /api/v1/auth/login-approvals/{id}/deny.
func (rt *Router) handleDenyLoginApproval(w http.ResponseWriter, r *http.Request) {
	rt.decideLoginApproval(w, r, false)
}

func (rt *Router) decideLoginApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	owner, actor, ok := rt.signInOwner(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	decided, err := rt.loginCodes.DecideLoginApproval(r.Context(), id, owner, approve, actor.kind+":"+actor.id, time.Now())
	if err != nil {
		rt.internalError(w, "api: decide login approval failed", err)
		return
	}
	if !decided {
		writeError(w, http.StatusNotFound, "no pending sign-in with this id")
		return
	}
	action := store.AuditActionNewDeviceDeny
	if approve {
		action = store.AuditActionNewDeviceApprove
	}
	rt.auditSignIn(r.Context(), r, actor, action, approvalAuditPrefix+id, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
