package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	accountDeleteStepUpApp = "account-delete"

	deleteBlockLastUser  = "last_user"
	deleteBlockLastRoot  = "last_root"
	deleteBlockTeamOwner = "team_owner"
)

type deleteBlocker struct {
	Code         string `json:"code"`
	ResourceID   string `json:"resource_id,omitempty"`
	ResourceName string `json:"resource_name,omitempty"`
	FixPath      string `json:"fix_path"`
}

type deleteCheckResponse struct {
	CanDelete bool            `json:"can_delete"`
	Email     string          `json:"email"`
	Blockers  []deleteBlocker `json:"blockers"`
}

// deleteBlockers lists what stops this account from being deleted today. Each
// one names where the operator can fix it, so the UI can offer a button.
func (rt *Router) deleteBlockers(ctx context.Context, user *store.User) ([]deleteBlocker, error) {
	out := []deleteBlocker{}
	n, err := rt.auth.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	if n <= 1 {
		out = append(out, deleteBlocker{Code: deleteBlockLastUser, FixPath: "/settings/users"})
	}
	if slices.Contains(user.Abilities, AbilityRoot) && rt.roles == nil {
		out = append(out, deleteBlocker{Code: deleteBlockLastRoot, FixPath: "/settings/users"})
	} else if slices.Contains(user.Abilities, AbilityRoot) {
		others, err := rt.roles.CountRootUsersExcept(ctx, user.ID)
		if err != nil {
			return nil, fmt.Errorf("count root users: %w", err)
		}
		if others == 0 {
			out = append(out, deleteBlocker{Code: deleteBlockLastRoot, FixPath: "/settings/users"})
		}
	}
	teams, err := rt.soleOwnedTeams(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	return append(out, teams...), nil
}

// soleOwnedTeams finds teams the user alone owns that still have other members or projects.
func (rt *Router) soleOwnedTeams(ctx context.Context, userID string) ([]deleteBlocker, error) {
	db := rt.teamAdmin()
	if db == nil {
		return nil, nil
	}
	roles, err := db.UserTeamRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("team roles: %w", err)
	}
	names := map[string]string{}
	if rt.organizations != nil {
		if orgs, err := rt.organizations.ListOrganizations(ctx); err == nil {
			for _, o := range orgs {
				names[o.ID] = o.Name
			}
		}
	}
	var out []deleteBlocker
	for teamID, role := range roles {
		if role != "owner" {
			continue
		}
		members, err := db.ListTeamMembers(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("team members: %w", err)
		}
		owners := 0
		for _, m := range members {
			if m.Role == "owner" {
				owners++
			}
		}
		if owners > 1 {
			continue
		}
		projects, err := db.CountTeamProjects(ctx, teamID)
		if err != nil {
			return nil, fmt.Errorf("team projects: %w", err)
		}
		if len(members) > 1 || projects > 0 {
			out = append(out, deleteBlocker{Code: deleteBlockTeamOwner, ResourceID: teamID, ResourceName: names[teamID], FixPath: "/settings/teams"})
		}
	}
	slices.SortFunc(out, func(a, b deleteBlocker) int { return strings.Compare(a.ResourceID, b.ResourceID) })
	return out, nil
}

// handleDeleteAccountCheck handles GET /api/v1/auth/delete-account/check.
func (rt *Router) handleDeleteAccountCheck(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityRead)
	if !ok {
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: delete check: load user failed", err)
		return
	}
	blockers, err := rt.deleteBlockers(r.Context(), user)
	if err != nil {
		rt.internalError(w, "api: delete check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, deleteCheckResponse{CanDelete: len(blockers) == 0, Email: user.Email, Blockers: blockers})
}

type deleteAccountRequest struct {
	Confirm string `json:"confirm"`
	stepUpProof
}

// handleDeleteAccount handles DELETE /api/v1/auth/account: the caller deletes
// itself after typing its email and proving the password (or a fresh step-up).
func (rt *Router) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWriteSensitive)
	if !ok {
		return
	}
	var req deleteAccountRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: delete account: load user failed", err)
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Confirm), user.Email) {
		writeCodedError(w, http.StatusBadRequest, "confirmation_mismatch", "type your account email to confirm")
		return
	}
	if !rt.accountDeleteProven(w, r, userID, viaSession, req.stepUpProof) {
		return
	}
	accountMutationMu.Lock()
	defer accountMutationMu.Unlock()
	blockers, err := rt.deleteBlockers(r.Context(), user)
	if err != nil {
		rt.internalError(w, "api: delete account failed", err)
		return
	}
	if len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, deleteCheckResponse{CanDelete: false, Email: user.Email, Blockers: blockers})
		return
	}
	rt.recordAccountAction(r, userID, viaSession, accountEventDeleted, http.StatusNoContent)
	rt.revokeOwnedTokens(r.Context(), userID)
	if err := rt.auth.DeleteUser(r.Context(), userID); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		rt.internalError(w, "api: delete account failed", err, slog.String("user_id", userID))
		return
	}
	rt.sessions.revokeAll(userID)
	rt.revokeTrustedDevices(r.Context(), r, anonymousSignIn(userID), userID, "user_deleted")
	rt.retireSignInRequests(r.Context(), r, userID, "user_deleted")
	w.WriteHeader(http.StatusNoContent)
}

// accountDeleteProven accepts a live step-up grant for this session, or an
// inline password (and code) proof, which is how a CLI token supplies it.
func (rt *Router) accountDeleteProven(w http.ResponseWriter, r *http.Request, userID string, viaSession bool, proof stepUpProof) bool {
	if viaSession {
		if c, err := r.Cookie(sessionCookieName); err == nil {
			if _, granted := rt.envProtect.grants.check(c.Value, userID, stepUpActionManage, accountDeleteStepUpApp, time.Now()); granted {
				return true
			}
		}
	}
	if proof.empty() {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm your password to delete your account")
		return false
	}
	if err := rt.verifyPasswordProof(r, userID, proof); err != nil {
		writeStepUpError(w, err)
		return false
	}
	return true
}

func (rt *Router) revokeOwnedTokens(ctx context.Context, userID string) {
	tokens, err := rt.libraryListTokens(ctx, userID, false)
	if err != nil {
		rt.logger.Warn("api: delete account: list tokens failed", slog.String("user_id", userID), slog.String("error", err.Error()))
		return
	}
	for _, t := range tokens {
		if t.RevokedAt != nil {
			continue
		}
		if err := rt.libraryRevokeToken(ctx, t.ID); err != nil {
			rt.logger.Warn("api: delete account: revoke token failed", slog.String("token_id", t.ID), slog.String("error", err.Error()))
		}
	}
}
