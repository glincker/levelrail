package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/crypto/bcrypt"
)

// userResource is the wire shape for a user in list/create responses:
// never PasswordHash, only whether one is set.
type userResource struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	HasPassword bool       `json:"has_password"`
	Providers   []string   `json:"providers"`
	Abilities   []string   `json:"abilities"`
	Role        string     `json:"role,omitempty"`
	IsFirstUser bool       `json:"is_first_user"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// toUserResource sets Role from roleForAbilities: empty (omitted from
// the response) when Abilities doesn't exactly match a curated preset,
// the "Custom" case in the UI.
func toUserResource(u store.User, providers []string) userResource {
	role, _ := roleForAbilities(u.Abilities)
	return userResource{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		HasPassword: u.PasswordHash != nil,
		Providers:   providers,
		Abilities:   u.Abilities,
		Role:        role,
		IsFirstUser: u.IsFirstUser,
		CreatedAt:   u.CreatedAt,
		LastLoginAt: u.LastLoginAt,
	}
}

// handleListUsers handles GET /api/v1/users: every account with access
// to this control plane. AbilityRead, the same tier GET /api/v1/apps
// uses: this is who has access, not a credential.
func (rt *Router) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := rt.auth.ListUsers(r.Context())
	if err != nil {
		rt.logger.Error("api: list users failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]userResource, 0, len(users))
	for _, u := range users {
		identities, err := rt.oauthIdentities.ListOAuthIdentitiesForUser(r.Context(), u.ID)
		if err != nil {
			rt.logger.Warn("api: list oauth identities for user failed", slog.String("user_id", u.ID), slog.String("error", err.Error()))
			identities = nil
		}
		providers := make([]string, 0, len(identities))
		for _, i := range identities {
			providers = append(providers, i.Provider)
		}
		out = append(out, toUserResource(u, providers))
	}
	writeJSON(w, http.StatusOK, out)
}

type createUserRequest struct {
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"password"`
	Abilities   []string `json:"abilities"`
	// Role, when non-empty, names a curated preset (roles.go) applied
	// instead of Abilities: a convenience for the common cases, not a
	// second permission model. Abilities is still required when Role is
	// empty, unchanged from before this field existed.
	Role string `json:"role,omitempty"`
}

// handleCreateUser handles POST /api/v1/auth/users: how every user after
// the first gets created, now that POST /auth/register only ever creates
// the first (see that handler's doc comment). AbilityRoot-gated (see
// router.go's registration): the caller picks the new user's Abilities
// (directly, or via Role), so only a caller who already has every
// ability may hand out any subset of them. One of Abilities/Role is
// required, no default: the root caller must explicitly choose what the
// new account can do.
func (rt *Router) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	abilities, err := resolveAbilities(req.Role, req.Abilities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	displayName := req.DisplayName
	if displayName == "" {
		displayName = req.Email
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		rt.logger.Error("api: create user: hash password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := randomOpaqueID("user_")
	if err != nil {
		rt.logger.Error("api: create user: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	hashStr := string(hash)
	user := store.User{
		ID:           id,
		Email:        req.Email,
		DisplayName:  displayName,
		PasswordHash: &hashStr,
		Abilities:    abilities,
		CreatedAt:    time.Now(),
	}
	if err := rt.auth.CreateUser(r.Context(), user); errors.Is(err, store.ErrUserEmailExists) {
		writeError(w, http.StatusConflict, "a user with this email already exists")
		return
	} else if err != nil {
		rt.logger.Error("api: create user: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toUserResource(user, nil))
}

type updateUserAbilitiesRequest struct {
	Abilities []string `json:"abilities"`
	// Role, when non-empty, names a curated preset (roles.go) applied
	// instead of Abilities, same as createUserRequest.Role.
	Role string `json:"role,omitempty"`
}

// handleUpdateUserAbilities handles PUT /api/v1/users/{id}/abilities:
// AbilityRoot-gated (see router.go's registration). Refuses editing the
// caller's own abilities outright: this is the one rule that prevents a
// root user from ever locking themselves out, accidentally or via a bug
// elsewhere, so it stays this simple rather than growing into a
// last-root-user quorum system.
func (rt *Router) handleUpdateUserAbilities(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if callerID, ok := rt.currentSessionUserID(r); ok && callerID == id {
		writeError(w, http.StatusBadRequest, "you cannot change your own abilities: ask another root user")
		return
	}

	var req updateUserAbilitiesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	abilities, err := resolveAbilities(req.Role, req.Abilities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := rt.auth.UpdateUserAbilities(r.Context(), id, abilities); errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		rt.logger.Error("api: update user abilities failed", slog.String("user_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	user, err := rt.auth.GetUserByID(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: update user abilities: reload failed", slog.String("user_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toUserResource(*user, nil))
}

// handleDeleteUser handles DELETE /api/v1/users/{id}: AbilityRoot, same
// tier as node deletion. Refuses self-deletion (a footgun) and deleting
// the last remaining user (would lock everyone out). Session revocation
// happens only after the store delete succeeds.
func (rt *Router) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if callerID, ok := rt.currentSessionUserID(r); ok && callerID == id {
		writeError(w, http.StatusBadRequest, "cannot remove your own account")
		return
	}

	n, err := rt.auth.CountUsers(r.Context())
	if err != nil {
		rt.logger.Error("api: delete user: count users failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if n <= 1 {
		writeError(w, http.StatusBadRequest, "cannot remove the last remaining user")
		return
	}

	if err := rt.auth.DeleteUser(r.Context(), id); errors.Is(err, store.ErrUserNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	} else if err != nil {
		rt.logger.Error("api: delete user failed", slog.String("user_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.sessions.revokeAll(id)
	w.WriteHeader(http.StatusNoContent)
}
