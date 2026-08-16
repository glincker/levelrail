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

// handleChangePassword handles PUT /api/v1/auth/password: session-only,
// the caller's own password. CurrentPassword is required and verified
// only when the account already has one; an OAuth-only account
// (PasswordHash nil) skips that check and just sets its first password,
// the live session already being proof enough. On success every other
// live session for this user is revoked.
func (rt *Router) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	userID, ok := rt.sessions.lookup(cookie.Value)
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

	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: change password: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.PasswordHash != nil {
		if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: change password: hash new password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	newHashStr := string(newHash)
	if err := rt.auth.UpdateUserPasswordHash(r.Context(), userID, &newHashStr); err != nil {
		rt.logger.Error("api: change password: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.sessions.revokeAllExcept(userID, cookie.Value)
	w.WriteHeader(http.StatusNoContent)
}

type sessionInfoResponse struct {
	Email       string `json:"username"`
	DisplayName string `json:"display_name"`
	HasPassword bool   `json:"has_password"`
	ExpiresAt   string `json:"expires_at"`
}

// handleGetSession handles GET /api/v1/auth/session: "signed in as X,
// until Y." Email stays under the "username" wire key for frontend
// compatibility. HasPassword lets the account page decide whether
// "change password" needs a current-password field.
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
	user, err := rt.auth.GetUserByID(r.Context(), sess.userID)
	if err != nil {
		rt.logger.Error("api: get session: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sessionInfoResponse{
		Email:       user.Email,
		DisplayName: user.DisplayName,
		HasPassword: user.PasswordHash != nil,
		ExpiresAt:   sess.expiresAt.UTC().Format(time.RFC3339),
	})
}

