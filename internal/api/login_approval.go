package api

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
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
	// StepUp approves with the account password instead of the number, only for sign-ins from this computer.
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

// approvalSessions is the slice of the session engine the pause needs; a
// test swaps it to make a revoke fail.
type approvalSessions interface {
	OtherSessionCount(ctx context.Context, userID, exceptToken string, within time.Duration) (int, error)
	RevokeChecked(ctx context.Context, token string) error
}

func (rt *Router) approvalSessionEngine() approvalSessions {
	if rt.approvalSessionsOverride != nil {
		return rt.approvalSessionsOverride
	}
	return rt.libSessions
}

// ApprovalMatch is the two digit number a waiting browser shows for its sign-in.
func ApprovalMatch(browserHash string) int { return approvalMatch(browserHash) }

// approvalMatch derives the two digit number the waiting browser shows, and
// the approver must type, from the browser binding nobody else holds.
func approvalMatch(browserHash string) int {
	sum := sha256.Sum256([]byte("approval-match\x00" + browserHash))
	return 10 + int(binary.BigEndian.Uint16(sum[0:2]))%90
}

// approvalHold is what holdForApproval decided for a sign-in it stopped.
type approvalHold struct {
	approval store.LoginApproval
	limited  bool
	retry    time.Duration
}

// holdForApproval stops a correct sign-in from an untrusted browser while
// another session of the account was used recently enough that someone can
// still approve from it; with none it lets the sign-in through so nobody is
// locked out by sessions they can no longer reach. A nil hold and nil error mean "let it
// through". token, when set, is the already issued session; it is revoked
// whenever the sign-in is held or the check fails.
func (rt *Router) holdForApproval(w http.ResponseWriter, r *http.Request, user store.User, token, method string) (*approvalHold, error) {
	if !rt.newDeviceApproval || rt.trustedDeviceFor(r, user.ID) || !rt.approvalRequiredFor(r.Context(), user.ID, method) {
		return nil, nil
	}
	ctx := r.Context()
	if rt.firstLoginTrusted(ctx, r, user, token, method) {
		rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionFirstLoginTrust, approvalAuditPrefix+"first_login#"+clientKindFromUserAgent(r.UserAgent()), http.StatusOK)
		rt.trustDevice(w, r, user.ID)
		return nil, nil
	}
	sessions := rt.approvalSessionEngine()
	others, err := sessions.OtherSessionCount(ctx, user.ID, token, rt.codeLogin.approverFreshness)
	if err != nil {
		if rerr := sessions.RevokeChecked(ctx, token); rerr != nil {
			rt.logger.Error("api: login: revoke unapproved session failed", slog.String("user_id", user.ID), slog.String("error", rerr.Error()))
		}
		return nil, fmt.Errorf("count other sessions: %w", err)
	}
	if others == 0 {
		return nil, nil
	}
	if err := sessions.RevokeChecked(ctx, token); err != nil {
		return nil, fmt.Errorf("revoke unapproved session: %w", err)
	}
	if a, ok := rt.pendingApprovalForBrowser(r, user.ID); ok {
		rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceRequest, approvalAuditPrefix+a.ID+"#retry", http.StatusAccepted)
		return &approvalHold{approval: a}, nil
	}
	if ok, retry := rt.codeLogin.approvals.allow(user.ID); !ok {
		return &approvalHold{limited: true, retry: retry}, nil
	}
	a, err := rt.createLoginApproval(w, r, user)
	if err != nil {
		return nil, err
	}
	return &approvalHold{approval: a}, nil
}

// pendingApprovalForBrowser finds the still-open approval this browser (or
// CLI, by its binding cookie) already holds for userID, so a retried sign-in
// keeps one request and one number instead of stacking new ones.
func (rt *Router) pendingApprovalForBrowser(r *http.Request, userID string) (store.LoginApproval, bool) {
	h, ok := cookieHash(r, approvalCookie)
	if !ok {
		return store.LoginApproval{}, false
	}
	a, err := rt.loginCodes.GetLoginApprovalByBrowser(r.Context(), h)
	if err != nil || a.UserID != userID || a.Status != store.LoginApprovalPending || !time.Now().Before(a.ExpiresAt) {
		return store.LoginApproval{}, false
	}
	return a, true
}

// inFirstLoginWindow reports whether user is the setup admin, still inside the
// window after creation in which first-run shortcuts may apply.
func (rt *Router) inFirstLoginWindow(user store.User) bool {
	window := rt.codeLogin.firstLoginWindow
	if window <= 0 || !user.IsFirstUser || user.CreatedAt.IsZero() {
		return false
	}
	age := time.Since(user.CreatedAt)
	return age >= 0 && age <= window
}

// firstLoginTrusted is the bounded trust on first use for a fresh install: the
// setup admin's first password sign-in skips approval once, inside the window,
// with no trusted browser and no session other than the one setup opened, and
// only from that setup device (its session cookie or registering address).
// Any doubt falls back to the normal approval flow.
func (rt *Router) firstLoginTrusted(ctx context.Context, r *http.Request, user store.User, token, method string) bool {
	if method != signInMethodPassword || user.FirstLoginTrustUsed || !rt.inFirstLoginWindow(user) {
		return false
	}
	now := time.Now()
	devices, err := rt.loginCodes.ListTrustedDevices(ctx, user.ID, now)
	if err != nil || len(devices) > 0 {
		return false
	}
	views, err := rt.libSessions.ListUserSessions(ctx, user.ID, token)
	if err != nil {
		return false
	}
	setupLive := false
	for _, v := range views {
		if v.Current {
			continue
		}
		if user.SetupSessionID == "" || v.ID != user.SetupSessionID || v.CreatedAt.After(user.CreatedAt.Add(rt.codeLogin.firstLoginSetupGrace)) {
			return false
		}
		setupLive = true
	}
	if setupLive && !rt.fromSetupDevice(ctx, r, user) {
		return false
	}
	won, err := rt.auth.ClaimFirstLoginTrust(ctx, user.ID)
	if err != nil {
		rt.logger.Warn("api: claim first login trust failed", slog.String("user_id", user.ID), slog.String("error", err.Error()))
		return false
	}
	return won
}

// fromSetupDevice reports whether the request carries the setup session or
// comes from the address that registered the account.
func (rt *Router) fromSetupDevice(ctx context.Context, r *http.Request, user store.User) bool {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		if sid, ok := rt.libSessions.SessionIDFor(ctx, c.Value); ok && sid == user.SetupSessionID {
			return true
		}
	}
	return user.SetupIP != "" && clientIP(r) == user.SetupIP
}

// pauseForDeviceApproval is holdForApproval for the JSON sign-in routes.
// Reports whether it wrote a response.
func (rt *Router) pauseForDeviceApproval(w http.ResponseWriter, r *http.Request, user store.User, token, method string) bool {
	hold, err := rt.holdForApproval(w, r, user, token, method)
	switch {
	case err != nil:
		rt.internalError(w, "api: login: new browser approval failed", err, slog.String("user_id", user.ID))
		return true
	case hold == nil:
		return false
	case hold.limited:
		writeRateLimited(w, hold.retry)
		return true
	}
	a := hold.approval
	writeJSON(w, http.StatusOK, loginResponse{ApprovalRequired: true, ApprovalID: a.ID, ApprovalExpiresAt: &a.ExpiresAt, ApprovalMatch: approvalMatch(a.BrowserHash)})
	return true
}

func (rt *Router) createLoginApproval(w http.ResponseWriter, r *http.Request, user store.User) (store.LoginApproval, error) {
	ctx := r.Context()
	binding, err := randomToken()
	if err != nil {
		return store.LoginApproval{}, fmt.Errorf("generate approval binding: %w", err)
	}
	id, err := randomOpaqueID("la_")
	if err != nil {
		return store.LoginApproval{}, fmt.Errorf("generate approval id: %w", err)
	}
	now := time.Now()
	ttl := rt.codeLogin.approvalTTL
	a := store.LoginApproval{
		ID: id, UserID: user.ID, BrowserHash: hashToken(binding), RequesterIP: clientIP(r),
		UserAgent: browserLabel(r.UserAgent()), CreatedAt: now, ExpiresAt: now.Add(ttl),
	}
	replaced, err := rt.loginCodes.ReplaceLoginApproval(ctx, a)
	if err != nil {
		return a, fmt.Errorf("save approval: %w", err)
	}
	for _, old := range replaced {
		rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceReplace, approvalAuditPrefix+old.ID, http.StatusOK)
	}
	rt.auditSignIn(ctx, r, anonymousSignIn(user.ID), store.AuditActionNewDeviceRequest, approvalAuditPrefix+a.ID, http.StatusAccepted)
	setBindingCookie(w, r, approvalCookie, approvalCookiePath, binding, ttl)
	return a, nil
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
		match := approvalMatch(a.BrowserHash)
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
	if err := rt.establishSession(w, withSignInNoticeSuppressed(r), *user); err != nil {
		rt.internalError(w, "api: login approval: establish session failed", err, slog.String("user_id", a.UserID))
		return
	}
	if !strings.HasPrefix(a.DecidedBy, rootApproverPrefix) {
		rt.trustDevice(w, r, user.ID)
	}
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
	owner, actor, rootOnly, ok := rt.signInDecider(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	auditPath := approvalAuditPrefix + id
	mismatch := false
	if approve {
		var body loginApprovalDecision
		err := decodeJSONBody(r, &body)
		byPassword := err == nil && body.Match == 0 && body.StepUp != nil && body.StepUp.Password != ""
		if err != nil || (!byPassword && (body.Match < 10 || body.Match > 99)) {
			writeError(w, http.StatusBadRequest, "match is required: type the two digit number shown in the waiting browser")
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
		switch {
		case byPassword && rootOnly:
			writeError(w, http.StatusForbidden, "a root token can approve only with the number shown in the waiting browser")
			return
		case byPassword:
			if !isLoopbackSignIn(a.RequesterIP) {
				writeError(w, http.StatusForbidden, "approving with a password is only allowed for sign-ins from this computer: type the number shown on the waiting device")
				return
			}
			// A wrong password is refused without denying, so a typo cannot cancel a real sign-in.
			if err := rt.verifyPasswordProof(r, owner, *body.StepUp); err != nil {
				writeStepUpError(w, err)
				return
			}
			auditPath += "#password"
		case body.Match != approvalMatch(a.BrowserHash):
			approve, mismatch = false, true
			auditPath += "#mismatch"
		}
	}
	decidedBy := actor.kind + ":" + actor.id
	if rootOnly {
		decidedBy = rootApproverPrefix + actor.id
	}
	decided, err := rt.loginCodes.DecideLoginApproval(ctx, id, owner, approve, decidedBy, time.Now())
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

// isLoopbackSignIn reports whether the sign-in came from this machine, the only source
// where a password may stand in for the number on the waiting device.
func isLoopbackSignIn(ip string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	return err == nil && addr.Unmap().IsLoopback()
}
