package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type passwordChangeRequiredRequest struct {
	Required *bool `json:"required"`
}

// attachAccountStates fills the library's lifecycle flags into a user list.
// A read failure leaves them unset rather than failing the whole list.
func (rt *Router) attachAccountStates(ctx context.Context, users []userResource) {
	if rt.authEngine == nil || len(users) == 0 {
		return
	}
	states, err := rt.authEngine.AccountStates(ctx)
	if err != nil {
		rt.logger.Warn("api: list users: read account states failed", slog.String("error", err.Error()))
		return
	}
	for i := range users {
		st := states[users[i].ID]
		users[i].Disabled, users[i].MustChangePassword = st.Disabled, st.MustChangePassword
	}
}

// handleSetPasswordChangeRequired handles PUT /api/v1/users/{id}/password-change-required.
// Requiring a change ends the user's sessions at once; every sign-in is then
// refused until the user completes a password reset, which clears the flag.
func (rt *Router) handleSetPasswordChangeRequired(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req passwordChangeRequiredRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil || req.Required == nil {
		writeError(w, http.StatusBadRequest, "body must be {\"required\": true|false}")
		return
	}
	if callerID, ok := rt.currentSessionUserID(r); ok && callerID == id {
		writeError(w, http.StatusBadRequest, "cannot require a password change on your own account")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), id)
	if errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: password change required: load user failed", err, slog.String("user_id", id))
		return
	}
	if *req.Required && user.PasswordHash == nil {
		writeError(w, http.StatusConflict, "this account has no password to change; it signs in with single sign-on or OAuth")
		return
	}
	if rt.authEngine == nil {
		writeError(w, http.StatusNotImplemented, errSSOUnavailable.Error())
		return
	}
	if err := rt.authEngine.SetMustChangePassword(r.Context(), id, *req.Required); err != nil {
		rt.internalError(w, "api: set password change required failed", err, slog.String("user_id", id))
		return
	}
	if *req.Required {
		rt.sessions.revokeAll(id)
	}
	res := []userResource{rt.userResourceFor(r.Context(), *user, nil)}
	rt.attachAccountStates(r.Context(), res)
	writeJSON(w, http.StatusOK, res[0])
}
