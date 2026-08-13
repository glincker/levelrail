package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

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
