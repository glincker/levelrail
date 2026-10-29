package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// teamInviteTTLEnv overrides how long a team invite link stays valid.
const teamInviteTTLEnv = "APP_TEAM_INVITE_TTL_HOURS"

const defaultTeamInviteTTL = 7 * 24 * time.Hour

func teamInviteTTL(requestedHours int) time.Duration {
	ttl := defaultTeamInviteTTL
	if n, err := strconv.Atoi(os.Getenv(teamInviteTTLEnv)); err == nil && n > 0 {
		ttl = time.Duration(n) * time.Hour
	}
	if requestedHours > 0 && time.Duration(requestedHours)*time.Hour < ttl {
		return time.Duration(requestedHours) * time.Hour
	}
	return ttl
}

type teamInviteResource struct {
	ID         string  `json:"id"`
	TeamID     string  `json:"team_id"`
	Email      string  `json:"email"`
	Role       string  `json:"role"`
	CreatedAt  string  `json:"created_at"`
	ExpiresAt  string  `json:"expires_at"`
	AcceptedAt *string `json:"accepted_at,omitempty"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
	Status     string  `json:"status"`
	// Token and Link are returned once, on creation only.
	Token string `json:"token,omitempty"`
	Link  string `json:"link,omitempty"`
}

func toTeamInviteResource(inv store.TeamInvite, now time.Time) teamInviteResource {
	out := teamInviteResource{ID: inv.ID, TeamID: inv.TeamID, Email: inv.Email, Role: inv.Role,
		CreatedAt: inv.CreatedAt.UTC().Format(time.RFC3339), ExpiresAt: inv.ExpiresAt.UTC().Format(time.RFC3339)}
	switch {
	case inv.AcceptedAt != nil:
		s := inv.AcceptedAt.UTC().Format(time.RFC3339)
		out.AcceptedAt, out.Status = &s, "accepted"
	case inv.RevokedAt != nil:
		s := inv.RevokedAt.UTC().Format(time.RFC3339)
		out.RevokedAt, out.Status = &s, "revoked"
	case !now.Before(inv.ExpiresAt):
		out.Status = "expired"
	default:
		out.Status = "pending"
	}
	return out
}

// handleListTeamInvites handles GET /api/v1/teams/{id}/invites.
func (rt *Router) handleListTeamInvites(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: list team invites: caller", err)
		return
	}
	if !caller.canManage(teamID) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(teamID))
		return
	}
	invites, err := rt.teamAdmin().ListTeamInvites(r.Context(), teamID)
	if err != nil {
		rt.internalError(w, "api: list team invites", err)
		return
	}
	now := time.Now()
	out := make([]teamInviteResource, 0, len(invites))
	for _, inv := range invites {
		out = append(out, toTeamInviteResource(inv, now))
	}
	writeJSON(w, http.StatusOK, out)
}

type createTeamInviteRequest struct {
	Email          string `json:"email"`
	Role           string `json:"role"`
	ExpiresInHours int    `json:"expires_in_hours"`
}

// handleCreateTeamInvite handles POST /api/v1/teams/{id}/invites. An empty
// email makes a link anyone holding it can redeem once.
func (rt *Router) handleCreateTeamInvite(w http.ResponseWriter, r *http.Request) {
	teamID := r.PathValue("id")
	var req createTeamInviteRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Role == store.TeamRoleOwner || !store.ValidTeamRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be admin, developer or viewer; use transfer to make an owner")
		return
	}
	if req.Email != "" && !strings.Contains(req.Email, "@") {
		writeError(w, http.StatusBadRequest, "email is not valid")
		return
	}
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: create team invite: caller", err)
		return
	}
	if !caller.canManage(teamID) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(teamID))
		return
	}
	if !caller.mayAssign(req.Role) {
		writeError(w, http.StatusForbidden, "an admin can only invite developers and viewers")
		return
	}
	plain, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: create team invite: token", err)
		return
	}
	id, err := randomOpaqueID("tinv_")
	if err != nil {
		rt.internalError(w, "api: create team invite: id", err)
		return
	}
	now := time.Now()
	inv := store.TeamInvite{ID: id, TeamID: teamID, Email: req.Email, Role: req.Role, TokenHash: hashToken(plain),
		CreatedBy: caller.userID, CreatedAt: now, ExpiresAt: now.Add(teamInviteTTL(req.ExpiresInHours))}
	if err := rt.teamAdmin().CreateTeamInvite(r.Context(), inv); err != nil {
		rt.internalError(w, "api: create team invite", err)
		return
	}
	rt.recordTeamEvent(r, teamID, caller.userID, "invite.created", inviteSubject(inv), req.Role)
	out := toTeamInviteResource(inv, now)
	out.Token = plain
	out.Link = rt.teamInviteLink(r, plain)
	writeJSON(w, http.StatusCreated, out)
}

func inviteSubject(inv store.TeamInvite) string {
	if inv.Email == "" {
		return "link " + inv.ID
	}
	return inv.Email
}

// teamInviteLink never uses the request Host, which a caller could forge.
func (rt *Router) teamInviteLink(r *http.Request, token string) string {
	return rt.emailLinkBase(r.Context()) + "/accept-team-invite?token=" + token
}

// handleRevokeTeamInvite handles DELETE /api/v1/teams/{id}/invites/{invite}.
func (rt *Router) handleRevokeTeamInvite(w http.ResponseWriter, r *http.Request) {
	teamID, inviteID := r.PathValue("id"), r.PathValue("invite")
	caller, err := rt.resolveTeamCaller(r, teamID)
	if err != nil {
		rt.internalError(w, "api: revoke team invite: caller", err)
		return
	}
	if !caller.canManage(teamID) {
		writeError(w, http.StatusForbidden, manageDeniedMessage(teamID))
		return
	}
	if err := rt.teamAdmin().RevokeTeamInvite(r.Context(), teamID, inviteID, time.Now()); err != nil {
		if errors.Is(err, store.ErrTeamInviteInvalid) {
			writeError(w, http.StatusNotFound, "no pending invite with that id")
			return
		}
		rt.internalError(w, "api: revoke team invite", err)
		return
	}
	rt.recordTeamEvent(r, teamID, caller.userID, "invite.revoked", inviteID, "")
	w.WriteHeader(http.StatusNoContent)
}

type acceptTeamInviteRequest struct {
	Token       string `json:"token"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type acceptTeamInviteResponse struct {
	TeamID string `json:"team_id"`
	Role   string `json:"role"`
	Email  string `json:"email"`
}

// handleAcceptTeamInvite handles POST /api/v1/team-invites/accept. A signed
// in user joins the team; anyone else creates an account in that team only.
// Authenticated by the single-use token; every failure looks the same.
func (rt *Router) handleAcceptTeamInvite(w http.ResponseWriter, r *http.Request) {
	if !rt.allowTokenRedeem(w, r, "team-invite-accept") {
		return
	}
	db := rt.teamAdmin()
	var req acceptTeamInviteRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Token == "" || db == nil {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	hash := hashToken(req.Token)
	inv, err := db.GetTeamInviteByHash(r.Context(), hash)
	if err != nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || !time.Now().Before(inv.ExpiresAt) {
		writeError(w, http.StatusBadRequest, store.ErrTeamInviteInvalid.Error())
		return
	}
	sessionUser, signedIn := rt.acceptingUserID(r)
	if !signedIn && rt.refuseInsecureLogin(w, r) {
		return
	}
	if status, msg := rt.precheckAccepter(r, inv, req, signedIn); status != 0 {
		writeError(w, status, msg)
		return
	}
	// Claimed before any account or membership exists, so a replay racing the
	// first use can never add a second person.
	if _, err := db.ClaimTeamInvite(r.Context(), hash, sessionUser, time.Now()); err != nil {
		writeError(w, http.StatusBadRequest, store.ErrTeamInviteInvalid.Error())
		return
	}
	user, status, msg := rt.inviteAccepter(r, inv, req)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	if user.created {
		// Audited before the session step so a session failure cannot leave a created account without a row.
		rt.recordAuthAudit(r, user.ID, accountEventTeamInviteAccepted, http.StatusOK, map[string]string{"team_id": inv.TeamID, "role": inv.Role})
		if err := rt.establishSession(w, r, user.User); err != nil {
			rt.internalError(w, "api: accept team invite: session", err)
			return
		}
	} else if err := db.AddTeamMember(r.Context(), inv.TeamID, user.ID, inv.Role); err != nil && !errors.Is(err, store.ErrTeamMemberExists) {
		rt.writeMembershipError(w, "accept team invite", err)
		return
	}
	rt.recordTeamEvent(r, inv.TeamID, user.ID, "invite.accepted", user.Email, inv.Role)
	if !user.created {
		rt.recordAuthAudit(r, user.ID, accountEventTeamInviteAccepted, http.StatusOK, map[string]string{"team_id": inv.TeamID, "role": inv.Role})
	}
	writeJSON(w, http.StatusOK, acceptTeamInviteResponse{TeamID: inv.TeamID, Role: inv.Role, Email: user.Email})
}

// precheckAccepter rejects a request that could never succeed before the
// invite is spent on it.
func (rt *Router) precheckAccepter(r *http.Request, inv store.TeamInvite, req acceptTeamInviteRequest, signedIn bool) (int, string) {
	if signedIn {
		userID, _ := rt.acceptingUserID(r)
		u, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return http.StatusUnauthorized, "authentication required"
		}
		if inv.Email != "" && !strings.EqualFold(inv.Email, u.Email) {
			return http.StatusBadRequest, store.ErrTeamInviteInvalid.Error()
		}
		return 0, ""
	}
	email := inv.Email
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(req.Email))
	}
	if email == "" || !strings.Contains(email, "@") {
		return http.StatusBadRequest, "email is required"
	}
	if len(req.Password) < minPasswordLength {
		return http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength)
	}
	if _, err := rt.auth.GetUserByEmail(r.Context(), email); err == nil {
		return http.StatusConflict, "an account with this email exists; sign in first, then open the invite link again"
	}
	return 0, ""
}

// acceptingUserID is the signed in user, or the owner of a live bearer token,
// so the CLI can accept an invite for its own account.
func (rt *Router) acceptingUserID(r *http.Request) (string, bool) {
	if id, ok := rt.currentSessionUserID(r); ok {
		return id, true
	}
	raw, ok := bearerToken(r)
	if !ok {
		return "", false
	}
	rec, err := rt.lookupBearerToken(r.Context(), raw)
	if err != nil || rec.RevokedAt != nil || rec.OwnerUserID == "" || (rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt)) {
		return "", false
	}
	if gone, gerr := rt.tokenOwnerGone(r.Context(), rec); gerr != nil || gone {
		return "", false
	}
	// This route runs outside requireAbility, so the stack gate never sees it.
	if rt.tokenStackScoped(r.Context(), rec.ID) {
		return "", false
	}
	return rec.OwnerUserID, true
}

type accepter struct {
	store.User
	created bool
}

// inviteAccepter resolves who redeems an invite: the signed in user, or a new
// account created straight into the invite's team. An email bound invite
// only accepts that email.
func (rt *Router) inviteAccepter(r *http.Request, inv store.TeamInvite, req acceptTeamInviteRequest) (accepter, int, string) {
	if userID, ok := rt.acceptingUserID(r); ok {
		u, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return accepter{}, http.StatusUnauthorized, "authentication required"
		}
		if inv.Email != "" && !strings.EqualFold(inv.Email, u.Email) {
			return accepter{}, http.StatusBadRequest, store.ErrTeamInviteInvalid.Error()
		}
		return accepter{User: *u}, 0, ""
	}
	email := inv.Email
	if email == "" {
		email = strings.ToLower(strings.TrimSpace(req.Email))
	}
	if email == "" || !strings.Contains(email, "@") {
		return accepter{}, http.StatusBadRequest, "email is required"
	}
	if len(req.Password) < minPasswordLength {
		return accepter{}, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength)
	}
	if _, err := rt.auth.GetUserByEmail(r.Context(), email); err == nil {
		return accepter{}, http.StatusConflict, "an account with this email exists; sign in first, then open the invite link again"
	}
	u, err := rt.createLocalUserInTeam(r.Context(), email, req.DisplayName, req.Password, store.TeamRoleAbilities(inv.Role), inv.TeamID, inv.Role)
	if errors.Is(err, store.ErrUserEmailExists) {
		return accepter{}, http.StatusConflict, "an account with this email exists; sign in first, then open the invite link again"
	}
	if err != nil {
		rt.logger.Error("api: accept team invite: create user failed", slog.String("error", err.Error()), slog.String("team_id", inv.TeamID))
		return accepter{}, http.StatusInternalServerError, "internal error"
	}
	return accepter{User: u, created: true}, 0, ""
}

type teamInvitePreview struct {
	TeamName  string `json:"team_name"`
	Role      string `json:"role"`
	Email     string `json:"email"`
	ExpiresAt string `json:"expires_at"`
}

// handlePreviewTeamInvite handles GET /api/v1/team-invites/{token} for a signed
// in user deciding whether to accept.
func (rt *Router) handlePreviewTeamInvite(w http.ResponseWriter, r *http.Request) {
	db := rt.teamAdmin()
	inv, err := db.GetTeamInviteByHash(r.Context(), hashToken(r.PathValue("token")))
	if err != nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || !time.Now().Before(inv.ExpiresAt) {
		writeError(w, http.StatusNotFound, store.ErrTeamInviteInvalid.Error())
		return
	}
	org, err := rt.organizations.GetOrganization(r.Context(), inv.TeamID)
	if err != nil {
		writeError(w, http.StatusNotFound, store.ErrTeamInviteInvalid.Error())
		return
	}
	writeJSON(w, http.StatusOK, teamInvitePreview{TeamName: org.Name, Role: inv.Role, Email: inv.Email, ExpiresAt: inv.ExpiresAt.UTC().Format(time.RFC3339)})
}
