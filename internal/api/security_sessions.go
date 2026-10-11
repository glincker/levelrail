package api

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const securitySessionsAuditPath = "/api/v1/security/sessions/"

type sessionResource struct {
	ID         string    `json:"id"`
	Browser    string    `json:"browser"`
	IP         string    `json:"ip"`
	Network    string    `json:"network"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

type securitySessionsResponse struct {
	UserID         string                  `json:"user_id"`
	Self           bool                    `json:"self"`
	Sessions       []sessionResource       `json:"sessions"`
	TrustedDevices []trustedDeviceResource `json:"trusted_devices"`
	Tokens         []tokenResource         `json:"tokens"`
}

type revokeSessionsResponse struct {
	Revoked int `json:"revoked"`
}

// sessionsTarget resolves whose sessions the caller acts on: its own, or,
// for an admin naming ?user_id=, anyone's. A token must belong to a user
// and hold write:sensitive or signin:approve, as for sign-in requests.
func (rt *Router) sessionsTarget(w http.ResponseWriter, r *http.Request) (target string, actor signInActor, self, ok bool) {
	owner, actor, ok := rt.signInOwner(w, r, false)
	if !ok {
		return "", actor, false, false
	}
	want := strings.TrimSpace(r.URL.Query().Get("user_id"))
	if want == "" || want == owner {
		return owner, actor, true, true
	}
	if !rt.callerIsAdmin(r) {
		writeError(w, http.StatusForbidden, "only an admin can manage another account's sessions")
		return "", actor, false, false
	}
	if _, err := rt.auth.GetUserByID(r.Context(), want); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return "", actor, false, false
		}
		rt.internalError(w, "api: sessions: load target user failed", err)
		return "", actor, false, false
	}
	return want, actor, false, true
}

func currentSessionToken(r *http.Request) string {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		return c.Value
	}
	return ""
}

// handleListSecuritySessions handles GET /api/v1/security/sessions: live
// sessions, trusted browsers and API tokens of the caller or, for an admin,
// of ?user_id=.
func (rt *Router) handleListSecuritySessions(w http.ResponseWriter, r *http.Request) {
	target, _, self, ok := rt.sessionsTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	now := time.Now()
	current := ""
	if self {
		current = currentSessionToken(r)
	}
	views, err := rt.libSessions.ListUserSessions(ctx, target, current)
	if err != nil {
		rt.internalError(w, "api: list sessions failed", err, slog.String("user_id", target))
		return
	}
	out := securitySessionsResponse{UserID: target, Self: self, Sessions: []sessionResource{}, TrustedDevices: []trustedDeviceResource{}, Tokens: []tokenResource{}}
	for _, v := range views {
		out.Sessions = append(out.Sessions, sessionResource{ID: v.ID, Browser: browserLabel(v.UserAgent), IP: safeSignInIP(v.IP), Network: ipNetworkPrefix(v.IP),
			CreatedAt: v.CreatedAt, LastSeenAt: v.LastSeenAt, ExpiresAt: v.ExpiresAt, Current: v.Current})
	}
	devices, err := rt.loginCodes.ListTrustedDevices(ctx, target, now)
	if err != nil {
		rt.internalError(w, "api: list sessions: trusted devices failed", err, slog.String("user_id", target))
		return
	}
	cookie, _ := cookieHash(r, trustedCookie)
	for _, d := range devices {
		out.TrustedDevices = append(out.TrustedDevices, trustedDeviceResource{ID: d.ID, Label: safeSignInText(d.Label), IP: safeSignInIP(d.IP), CreatedAt: d.CreatedAt,
			LastUsedAt: d.LastUsedAt, ExpiresAt: d.ExpiresAt, Current: self && cookie != "" && cookie == d.TokenHash})
	}
	tokens, err := rt.libraryListTokens(ctx, target, false)
	if err != nil {
		rt.internalError(w, "api: list sessions: tokens failed", err, slog.String("user_id", target))
		return
	}
	for _, t := range tokens {
		if t.RevokedAt == nil {
			out.Tokens = append(out.Tokens, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeSecuritySession handles DELETE /api/v1/security/sessions/{id}.
// The session must belong to the target account, or it reads as not found.
func (rt *Router) handleRevokeSecuritySession(w http.ResponseWriter, r *http.Request) {
	target, actor, self, ok := rt.sessionsTarget(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	ctx := r.Context()
	live, err := rt.libSessions.ListUserSessions(ctx, target, "")
	if err != nil {
		rt.internalError(w, "api: revoke session: list failed", err, slog.String("user_id", target))
		return
	}
	if !slices.ContainsFunc(live, func(v authengine.SessionView) bool { return v.ID == id }) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	revoked, err := rt.libSessions.RevokeUserSession(ctx, target, id)
	if err != nil {
		rt.internalError(w, "api: revoke session failed", err, slog.String("user_id", target))
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	path := securitySessionsAuditPath + id
	if !self {
		path += "#user:" + target
	}
	rt.auditSignIn(ctx, r, actor, store.AuditActionSessionRevoke, path, http.StatusNoContent)
	rt.sec.posture.invalidate()
	w.WriteHeader(http.StatusNoContent)
}

// handleRevokeOtherSecuritySessions handles POST
// /api/v1/security/sessions/revoke-others: every session of the target but
// the caller's own, plus its trusted browsers and waiting sign-ins.
func (rt *Router) handleRevokeOtherSecuritySessions(w http.ResponseWriter, r *http.Request) {
	target, actor, self, ok := rt.sessionsTarget(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	before, err := rt.libSessions.ListUserSessions(ctx, target, "")
	if err != nil {
		rt.internalError(w, "api: revoke other sessions: list failed", err, slog.String("user_id", target))
		return
	}
	keep := ""
	if self {
		keep = currentSessionToken(r)
	}
	if keep != "" {
		err = rt.libSessions.RevokeAllExcept(ctx, target, keep)
	} else {
		err = rt.libSessions.RevokeAll(ctx, target)
		rt.sessions.revokeAll(target)
	}
	if err != nil {
		rt.internalError(w, "api: revoke other sessions failed", err, slog.String("user_id", target))
		return
	}
	reason := "sessions_revoked"
	path := securitySessionsAuditPath + "others"
	if !self {
		reason, path = "sessions_revoked_by_admin", path+"#user:"+target
	}
	rt.resetSignInTrust(ctx, r, target, reason)
	n := len(before)
	if keep != "" && n > 0 {
		n--
	}
	rt.auditSignIn(ctx, r, actor, store.AuditActionSessionRevokeOthers, path, http.StatusOK)
	rt.sec.posture.invalidate()
	writeJSON(w, http.StatusOK, revokeSessionsResponse{Revoked: n})
}

type accountSecurityResource struct {
	RequireNewDeviceApproval bool       `json:"require_new_device_approval"`
	ResetFlagged             bool       `json:"reset_flagged"`
	ResetFlaggedAt           *time.Time `json:"reset_flagged_at,omitempty"`
	ResetFlagReason          string     `json:"reset_flag_reason,omitempty"`
}

type accountSecurityRequest struct {
	RequireNewDeviceApproval *bool `json:"require_new_device_approval"`
}

func toAccountSecurity(s store.UserSecuritySettings) accountSecurityResource {
	out := accountSecurityResource{RequireNewDeviceApproval: s.RequireNewDeviceApproval}
	if !s.ResetFlaggedAt.IsZero() {
		at := s.ResetFlaggedAt
		out.ResetFlagged, out.ResetFlaggedAt, out.ResetFlagReason = true, &at, s.ResetFlagReason
	}
	return out
}

// handleGetAccountSecurity handles GET /api/v1/security/account: the
// caller's own security switches.
func (rt *Router) handleGetAccountSecurity(w http.ResponseWriter, r *http.Request) {
	owner, _, ok := rt.signInOwner(w, r, false)
	if !ok {
		return
	}
	s, err := rt.security.GetUserSecuritySettings(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: get account security failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toAccountSecurity(s))
}

// handlePutAccountSecurity handles PUT /api/v1/security/account. Turning a
// protection off needs a signed-in dashboard session, never a token.
func (rt *Router) handlePutAccountSecurity(w http.ResponseWriter, r *http.Request) {
	owner, actor, ok := rt.signInOwner(w, r, false)
	if !ok {
		return
	}
	var req accountSecurityRequest
	if err := decodeJSONBody(r, &req); err != nil || req.RequireNewDeviceApproval == nil {
		writeError(w, http.StatusBadRequest, "require_new_device_approval is required")
		return
	}
	if !*req.RequireNewDeviceApproval && !rt.signedInUserSession(r) {
		writeError(w, http.StatusForbidden, "turning this protection off needs a signed-in dashboard session")
		return
	}
	ctx := r.Context()
	if err := rt.security.SetRequireNewDeviceApproval(ctx, owner, *req.RequireNewDeviceApproval, time.Now()); err != nil {
		rt.internalError(w, "api: put account security failed", err)
		return
	}
	state := "off"
	if *req.RequireNewDeviceApproval {
		state = "on"
	}
	rt.auditSignIn(ctx, r, actor, store.AuditActionDeviceApprovalToggle, "/api/v1/security/account#"+state, http.StatusOK)
	rt.sec.posture.invalidate()
	rt.handleGetAccountSecurity(w, r)
}
