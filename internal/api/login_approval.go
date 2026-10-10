package api

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	msgNoApprovalWaiting = "no sign-in is waiting for approval in this browser"
	msgMatchMismatch     = "the number did not match the one shown in the waiting browser, so the sign-in was denied"
)

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
	MatchNumber int       `json:"match_number,omitempty"`
	Email       string    `json:"username,omitempty"`
	DisplayName string    `json:"display_name,omitempty"`
	MFARequired bool      `json:"mfa_required,omitempty"`
	MFAToken    string    `json:"mfa_token,omitempty"`
}

type loginApprovalDecision struct {
	Match int `json:"match"`
}

// approvalSessions is the slice of the session engine the pause needs; a
// test swaps it to make a revoke fail.
type approvalSessions interface {
	OtherSessionCount(ctx context.Context, userID, exceptToken string) (int, error)
	RevokeChecked(ctx context.Context, token string) error
}

func (rt *Router) approvalSessionEngine() approvalSessions {
	if rt.approvalSessionsOverride != nil {
		return rt.approvalSessionsOverride
	}
	return rt.libSessions
}

// approvalMatch derives the two digit number the waiting browser shows and
// the three choices the approver sees, from the browser binding nobody else
// holds, so no extra state is stored.
func approvalMatch(browserHash string) (number int, options []int) {
	sum := sha256.Sum256([]byte("approval-match\x00" + browserHash))
	pick := func(i int) int { return 10 + int(binary.BigEndian.Uint16(sum[i:i+2]))%90 }
	number = pick(0)
	options = []int{number}
	for i := 2; i+2 <= len(sum) && len(options) < 3; i += 2 {
		if n := pick(i); !slices.Contains(options, n) {
			options = append(options, n)
		}
	}
	for n := 10; len(options) < 3; n++ {
		if !slices.Contains(options, n) {
			options = append(options, n)
		}
	}
	slices.Sort(options)
	return number, options
}

// pauseForDeviceApproval holds a correct password login from an untrusted
// browser while another session of the account is live; with none it lets the
// login through so nobody is locked out. Reports whether it wrote a response.
func (rt *Router) pauseForDeviceApproval(w http.ResponseWriter, r *http.Request, user store.User, token string) bool {
	if !rt.newDeviceApproval || rt.trustedDeviceFor(r, user.ID) {
		return false
	}
	ctx := r.Context()
	sessions := rt.approvalSessionEngine()
	others, err := sessions.OtherSessionCount(ctx, user.ID, token)
	if err != nil {
		if rerr := sessions.RevokeChecked(ctx, token); rerr != nil {
			rt.logger.Error("api: login: revoke unapproved session failed", slog.String("user_id", user.ID), slog.String("error", rerr.Error()))
		}
		rt.internalError(w, "api: login: count other sessions failed", err, slog.String("user_id", user.ID))
		return true
	}
	if others == 0 {
		return false
	}
	if err := sessions.RevokeChecked(ctx, token); err != nil {
		rt.internalError(w, "api: login: revoke unapproved session failed", err, slog.String("user_id", user.ID))
		return true
	}
	if ok, retry := rt.codeLogin.approvals.allow(user.ID); !ok {
		writeRateLimited(w, retry)
		return true
	}
	rt.createLoginApproval(w, r, user)
	return true
}

func (rt *Router) createLoginApproval(w http.ResponseWriter, r *http.Request, user store.User) {
	ctx := r.Context()
	binding, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: login: generate approval binding failed", err)
		return
	}
	id, err := randomOpaqueID("la_")
	if err != nil {
		rt.internalError(w, "api: login: generate approval id failed", err)
		return
	}
	now := time.Now()
	replaced, err := rt.loginCodes.SupersedeLoginApprovals(ctx, user.ID, now)
	for _, old := range replaced {
		rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceReplace, approvalAuditPrefix+old.ID, http.StatusOK)
	}
	if err != nil {
		rt.internalError(w, "api: login: supersede approvals failed", err, slog.String("user_id", user.ID))
		return
	}
	ttl := rt.codeLogin.approvalTTL
	a := store.LoginApproval{
		ID: id, UserID: user.ID, BrowserHash: hashToken(binding), RequesterIP: clientIP(r),
		UserAgent: browserLabel(r.UserAgent()), CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	if err := rt.loginCodes.CreateLoginApproval(ctx, a); err != nil {
		rt.internalError(w, "api: login: save approval failed", err, slog.String("user_id", user.ID))
		return
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceRequest, approvalAuditPrefix+a.ID, http.StatusAccepted)
	setBindingCookie(w, r, approvalCookie, approvalCookiePath, binding, ttl)
	match, _ := approvalMatch(a.BrowserHash)
	writeJSON(w, http.StatusOK, loginResponse{ApprovalRequired: true, ApprovalID: a.ID, ApprovalExpiresAt: &a.ExpiresAt, ApprovalMatch: match})
}

// handlePollLoginApproval handles POST /api/v1/auth/login-approval/poll: the
// waiting browser's check, bound to its approval cookie. Once approved it
// issues the session exactly once.
func (rt *Router) handlePollLoginApproval(w http.ResponseWriter, r *http.Request) {
	if rt.refuseInsecureLogin(w, r) || refuseUnsafeSignInPOST(w, r) {
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
		match, _ := approvalMatch(a.BrowserHash)
		writeJSON(w, http.StatusOK, loginApprovalPollResponse{Status: approvalPollPending, ExpiresAt: a.ExpiresAt, MatchNumber: match})
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
// The body names the number the waiting browser shows; a wrong one denies.
func (rt *Router) handleApproveLoginApproval(w http.ResponseWriter, r *http.Request) {
	rt.decideLoginApproval(w, r, true)
}

// handleDenyLoginApproval handles POST /api/v1/auth/login-approvals/{id}/deny.
func (rt *Router) handleDenyLoginApproval(w http.ResponseWriter, r *http.Request) {
	rt.decideLoginApproval(w, r, false)
}

func (rt *Router) decideLoginApproval(w http.ResponseWriter, r *http.Request, approve bool) {
	owner, actor, ok := rt.signInOwner(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	auditPath := approvalAuditPrefix + id
	mismatch := false
	if approve {
		var body loginApprovalDecision
		if err := decodeJSONBody(r, &body); err != nil || body.Match == 0 {
			writeError(w, http.StatusBadRequest, "match is required: choose the number shown in the waiting browser")
			return
		}
		a, err := rt.loginCodes.GetLoginApproval(ctx, id)
		if errors.Is(err, store.ErrLoginApprovalNotFound) || (err == nil && a.UserID != owner) {
			writeError(w, http.StatusNotFound, "no pending sign-in with this id")
			return
		}
		if err != nil {
			rt.internalError(w, "api: decide login approval: load failed", err)
			return
		}
		if want, _ := approvalMatch(a.BrowserHash); body.Match != want {
			approve, mismatch = false, true
			auditPath += "#mismatch"
		}
	}
	decided, err := rt.loginCodes.DecideLoginApproval(ctx, id, owner, approve, actor.kind+":"+actor.id, time.Now())
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
	rt.auditSignIn(ctx, r, actor, action, auditPath, http.StatusNoContent)
	if mismatch {
		writeError(w, http.StatusConflict, msgMatchMismatch)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
