package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type signInCodeItem struct {
	ID          string    `json:"id"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	// Revealable is false after a control plane restart: the plaintext lived
	// only in memory, so the requester must ask for a new code.
	Revealable bool `json:"revealable"`
}

type signInApprovalItem struct {
	ID          string    `json:"id"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type signInRequestsResponse struct {
	Codes     []signInCodeItem     `json:"codes"`
	Approvals []signInApprovalItem `json:"approvals"`
}

type revealLoginCodeResponse struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

const msgNeedsSignInApprove = "token lacks the required ability (needs " + AbilitySignInApprove +
	"; mint one from a signed-in dashboard session, or use the dashboard)"

// signInOwner resolves whose sign-in requests the caller may see and act on.
// A session acts for its own user. A bearer token acts only for the user that
// owns it, and only with signin:approve when it would reveal a code or decide
// an approval (approve true): root and write:sensitive never imply it, so a
// leaked device-login token cannot turn itself into a session.
func (rt *Router) signInOwner(w http.ResponseWriter, r *http.Request, approve bool) (string, signInActor, bool) {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeError(w, http.StatusForbidden, "a session link cannot manage sign-in requests")
		return "", signInActor{}, false
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		return userID, signInActor{kind: auditActorSession, id: userID, name: rt.auditActorName(r.Context(), auditActorSession, userID, "")}, true
	}
	rec, ok := r.Context().Value(tokenIdentityKey{}).(*store.APIToken)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return "", signInActor{}, false
	}
	if rec.OwnerUserID == "" {
		writeError(w, http.StatusForbidden, "this token is not tied to a user account")
		return "", signInActor{}, false
	}
	switch {
	case approve && !hasSignInApprove(rec.Abilities):
		writeError(w, http.StatusForbidden, msgNeedsSignInApprove)
		return "", signInActor{}, false
	case !approve && !hasSignInApprove(rec.Abilities) && !hasAbility(rec.Abilities, AbilityWriteSensitive):
		writeError(w, http.StatusForbidden, "token lacks the required ability (needs "+AbilityWriteSensitive+" or "+AbilitySignInApprove+")")
		return "", signInActor{}, false
	}
	return rec.OwnerUserID, signInActor{kind: "token", id: rec.ID, name: rec.Name}, true
}

// handleListSignInRequests handles GET /api/v1/auth/sign-in-requests: the
// caller's own waiting codes and new-device approvals. It never carries a
// code; revealing one is a separate, audited call.
func (rt *Router) handleListSignInRequests(w http.ResponseWriter, r *http.Request) {
	owner, _, ok := rt.signInOwner(w, r, false)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now()
	codes, err := rt.loginCodes.ListLiveLoginCodesForUser(ctx, owner, now)
	if err != nil {
		rt.internalError(w, "api: list sign-in codes failed", err)
		return
	}
	approvals, err := rt.loginCodes.ListPendingLoginApprovalsForUser(ctx, owner, now)
	if err != nil {
		rt.internalError(w, "api: list sign-in approvals failed", err)
		return
	}
	out := signInRequestsResponse{Codes: []signInCodeItem{}, Approvals: []signInApprovalItem{}}
	for _, c := range codes {
		_, revealable := rt.codeLogin.lookup(c.ID, owner, now)
		out.Codes = append(out.Codes, signInCodeItem{ID: c.ID, RequesterIP: safeSignInIP(c.RequesterIP), UserAgent: safeSignInText(c.UserAgent),
			CreatedAt: c.CreatedAt, ExpiresAt: c.ExpiresAt, Revealable: revealable})
	}
	for _, a := range approvals {
		out.Approvals = append(out.Approvals, signInApprovalItem{ID: a.ID, RequesterIP: safeSignInIP(a.RequesterIP), UserAgent: safeSignInText(a.UserAgent),
			CreatedAt: a.CreatedAt, ExpiresAt: a.ExpiresAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevealLoginCode handles POST /api/v1/auth/sign-in-requests/codes/{id}/reveal:
// the one route that returns a code, to its own account, audited as delivered.
func (rt *Router) handleRevealLoginCode(w http.ResponseWriter, r *http.Request) {
	owner, actor, ok := rt.signInOwner(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("id")
	now := time.Now()
	code, ok := rt.codeLogin.lookup(id, owner, now)
	if !ok {
		writeError(w, http.StatusNotFound, "no code to show for this request; ask for a new code")
		return
	}
	var expires time.Time
	codes, err := rt.loginCodes.ListLiveLoginCodesForUser(r.Context(), owner, now)
	if err != nil {
		rt.internalError(w, "api: reveal sign-in code: list failed", err)
		return
	}
	for _, c := range codes {
		if c.ID == id {
			expires = c.ExpiresAt
		}
	}
	if expires.IsZero() {
		rt.codeLogin.forget(id)
		writeError(w, http.StatusNotFound, "no code to show for this request; ask for a new code")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	rt.auditSignIn(r.Context(), r, actor, store.AuditActionLoginCodeDeliver, loginCodeAuditPrefix+id+"#"+clientKindFromUserAgent(r.UserAgent()), http.StatusOK)
	writeJSON(w, http.StatusOK, revealLoginCodeResponse{Code: formatLoginCode(code), ExpiresAt: expires})
}

type trustedDeviceResource struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	IP         string    `json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

type trustedDevicesResponse struct {
	Devices []trustedDeviceResource `json:"devices"`
}

// trustDevice marks this browser as trusted for userID's password logins.
// A browser already trusted for userID keeps its row, extended, rather than
// collecting a new one per sign-in. Best effort: a failure only means the
// next login may ask for approval.
func (rt *Router) trustDevice(w http.ResponseWriter, r *http.Request, userID string) {
	if !rt.newDeviceApproval {
		return
	}
	now := time.Now()
	ttl := rt.codeLogin.trustTTL
	if h, ok := cookieHash(r, trustedCookie); ok {
		if d, err := rt.loginCodes.GetTrustedDeviceByHash(r.Context(), h, now); err == nil && d.UserID == userID {
			if err := rt.loginCodes.ExtendTrustedDevice(r.Context(), d.ID, now.Add(ttl), now); err != nil {
				rt.logger.Warn("api: extend trusted device failed", slog.String("device_id", d.ID), slog.String("error", err.Error()))
				return
			}
			if c, err := r.Cookie(trustedCookie); err == nil {
				setBindingCookie(w, r, trustedCookie, trustedCookiePath, c.Value, ttl)
			}
			return
		}
	}
	token, err := randomToken()
	if err != nil {
		rt.logger.Warn("api: trusted device token failed", slog.String("error", err.Error()))
		return
	}
	id, err := randomOpaqueID("td_")
	if err != nil {
		rt.logger.Warn("api: trusted device id failed", slog.String("error", err.Error()))
		return
	}
	d := store.TrustedDevice{ID: id, UserID: userID, TokenHash: hashToken(token), Label: browserLabel(r.UserAgent()),
		IP: clientIP(r), CreatedAt: now, LastUsedAt: now, ExpiresAt: now.Add(ttl)}
	if err := rt.loginCodes.CreateTrustedDevice(r.Context(), d); err != nil {
		rt.logger.Warn("api: save trusted device failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		return
	}
	setBindingCookie(w, r, trustedCookie, trustedCookiePath, token, ttl)
}

// revokeTrustedDevices drops every trusted browser of userID after a
// credential or session reset. Failures are logged: the reset itself stands.
func (rt *Router) revokeTrustedDevices(ctx context.Context, r *http.Request, actor signInActor, userID, reason string) {
	if rt.loginCodes == nil || userID == "" {
		return
	}
	n, err := rt.loginCodes.RevokeAllTrustedDevices(ctx, userID, time.Now())
	if err != nil {
		rt.logger.Warn("api: revoke trusted devices failed", slog.String("user_id", userID), slog.String("reason", reason), slog.String("error", err.Error()))
		return
	}
	if n > 0 {
		rt.auditSignIn(ctx, r, actor, store.AuditActionTrustedDeviceDrop, trustedAuditPrefix+"all#"+reason+"#"+strconv.Itoa(n), http.StatusOK)
	}
}

// retireSignInRequests ends userID's waiting approvals, approved ones not yet
// collected included, and locks its live codes, so nothing requested before a
// credential or session reset can still turn into a session.
func (rt *Router) retireSignInRequests(ctx context.Context, r *http.Request, userID, reason string) {
	if rt.loginCodes == nil || userID == "" {
		return
	}
	now := time.Now()
	actor := anonymousSignIn(userID)
	rt.codeLogin.forgetUser(userID)
	retired, err := rt.loginCodes.SupersedeLoginApprovals(ctx, userID, now)
	if err != nil {
		rt.logger.Warn("api: retire login approvals failed", slog.String("user_id", userID), slog.String("reason", reason), slog.String("error", err.Error()))
	}
	for _, a := range retired {
		rt.auditSignIn(ctx, r, actor, store.AuditActionNewDeviceReplace, approvalAuditPrefix+a.ID+"#"+reason, http.StatusOK)
	}
	locked, err := rt.loginCodes.LockLoginCodesForUser(ctx, userID, now)
	if err != nil {
		rt.logger.Warn("api: lock login codes failed", slog.String("user_id", userID), slog.String("reason", reason), slog.String("error", err.Error()))
	}
	for _, id := range locked {
		rt.codeLogin.forget(id)
		rt.auditSignIn(ctx, r, actor, store.AuditActionLoginCodeLockout, loginCodeAuditPrefix+id+"#"+reason, http.StatusOK)
	}
}

// revokeSignInApproveTokens revokes userID's signin:approve tokens after a
// credential or session reset, so a leaked approver cannot outlive it.
func (rt *Router) revokeSignInApproveTokens(ctx context.Context, r *http.Request, userID, reason string) {
	if rt.authLib.tokens == nil || userID == "" {
		return
	}
	ids, err := rt.authLib.tokens.RevokeTokensWithAbility(ctx, userID, AbilitySignInApprove)
	if err != nil {
		rt.logger.Warn("api: revoke signin:approve tokens failed", slog.String("user_id", userID), slog.String("reason", reason), slog.String("error", err.Error()))
	}
	for _, id := range ids {
		if lerr := rt.tokens.RevokeAPIToken(ctx, id); lerr != nil && !errors.Is(lerr, store.ErrAPITokenNotFound) {
			rt.logger.Warn("api: revoke signin:approve token mirror failed", slog.String("token_id", id), slog.String("error", lerr.Error()))
		}
	}
	if len(ids) > 0 {
		rt.auditSignIn(ctx, r, anonymousSignIn(userID), store.AuditActionApproverRevoke, approverAuditPrefix+reason+"#"+strconv.Itoa(len(ids)), http.StatusOK)
	}
}

// resetSignInTrust is everything a password change, reset or "sign out other
// sessions" retires besides the sessions themselves.
func (rt *Router) resetSignInTrust(ctx context.Context, r *http.Request, userID, reason string) {
	rt.revokeTrustedDevices(ctx, r, anonymousSignIn(userID), userID, reason)
	rt.retireSignInRequests(ctx, r, userID, reason)
	rt.revokeSignInApproveTokens(ctx, r, userID, reason)
	if rt.security != nil && (reason == "password_changed" || reason == "password_reset") {
		if err := rt.security.ClearUserResetFlag(ctx, userID, time.Now()); err != nil {
			rt.logger.Warn("api: clear reset flag failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		}
		rt.sec.posture.invalidate()
	}
}

// trustedDeviceFor reports whether the request carries a live trusted-device
// cookie issued to userID.
func (rt *Router) trustedDeviceFor(r *http.Request, userID string) bool {
	h, ok := cookieHash(r, trustedCookie)
	if !ok {
		return false
	}
	d, err := rt.loginCodes.GetTrustedDeviceByHash(r.Context(), h, time.Now())
	if err != nil || d.UserID != userID {
		return false
	}
	if err := rt.loginCodes.TouchTrustedDevice(r.Context(), d.ID, time.Now()); err != nil {
		rt.logger.Warn("api: touch trusted device failed", slog.String("device_id", d.ID), slog.String("error", err.Error()))
	}
	return true
}

// handleListTrustedDevices handles GET /api/v1/auth/trusted-devices.
func (rt *Router) handleListTrustedDevices(w http.ResponseWriter, r *http.Request) {
	owner, _, ok := rt.signInOwner(w, r, false)
	if !ok {
		return
	}
	list, err := rt.loginCodes.ListTrustedDevices(r.Context(), owner, time.Now())
	if err != nil {
		rt.internalError(w, "api: list trusted devices failed", err)
		return
	}
	current, _ := cookieHash(r, trustedCookie)
	out := trustedDevicesResponse{Devices: []trustedDeviceResource{}}
	for _, d := range list {
		out.Devices = append(out.Devices, trustedDeviceResource{ID: d.ID, Label: safeSignInText(d.Label), IP: safeSignInIP(d.IP), CreatedAt: d.CreatedAt,
			LastUsedAt: d.LastUsedAt, ExpiresAt: d.ExpiresAt, Current: current != "" && current == d.TokenHash})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeTrustedDevice handles DELETE /api/v1/auth/trusted-devices/{id}.
func (rt *Router) handleRevokeTrustedDevice(w http.ResponseWriter, r *http.Request) {
	owner, actor, ok := rt.signInOwner(w, r, false)
	if !ok {
		return
	}
	id := r.PathValue("id")
	revoked, err := rt.loginCodes.RevokeTrustedDevice(r.Context(), owner, id, time.Now())
	if err != nil {
		rt.internalError(w, "api: revoke trusted device failed", err)
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, "trusted device not found")
		return
	}
	rt.auditSignIn(r.Context(), r, actor, store.AuditActionTrustedDeviceDrop, trustedAuditPrefix+id, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
