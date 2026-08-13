package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// handleChangePassword handles PUT /api/v1/auth/password: session-only
// (requireAuth, never requireAbility), the same "a credential can never
// modify credentials on its own behalf" rule token management already
// follows, applied here to the admin's own password. Requires the
// current password to be re-supplied and verified, not just a valid
// session, matching the password-reconfirmation principle
// docs-local/research/competitor-onboarding-auth-ux.md finding 5
// establishes for destructive account changes.
//
// On success, every other live session for this admin is revoked
// (finding 6: rotate sessions on password change) except the one that
// made this request, so changing your password from one browser doesn't
// also sign you out of it.
func (rt *Router) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	username, ok := rt.sessions.lookup(cookie.Value)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	var req changePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("new password must be at least %d characters", minPasswordLength))
		return
	}

	admin, err := rt.auth.GetAdminUser(r.Context())
	if err != nil {
		rt.logger.Error("api: change password: load admin user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: change password: hash new password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.auth.UpsertAdminUser(r.Context(), username, string(newHash)); err != nil {
		rt.logger.Error("api: change password: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.sessions.revokeAllExcept(username, cookie.Value)
	w.WriteHeader(http.StatusNoContent)
}

type sessionInfoResponse struct {
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// handleGetSession handles GET /api/v1/auth/session: the security
// settings screen's read model for "you are signed in as X, until Y."
// Session-only like every other handler in this file, deliberately not
// requireAbility: a bearer token has no session of its own to report on.
func (rt *Router) handleGetSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	sess, ok := rt.sessions.get(cookie.Value)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	writeJSON(w, http.StatusOK, sessionInfoResponse{
		Username:  sess.username,
		ExpiresAt: sess.expiresAt.UTC().Format(time.RFC3339),
	})
}

// handleRevokeOtherSessions handles POST /api/v1/auth/sessions/revoke-others:
// the standalone, user-triggered version of the same revokeAllExcept
// rotation handleChangePassword already performs automatically. Exists
// for the case an operator wants to end other signed-in sessions (a
// forgotten browser, a shared machine) without also being forced to
// change their password to get there.
func (rt *Router) handleRevokeOtherSessions(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	username, ok := rt.sessions.lookup(cookie.Value)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rt.sessions.revokeAllExcept(username, cookie.Value)
	w.WriteHeader(http.StatusNoContent)
}
