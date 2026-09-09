package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultInviteTTL is the fallback when the control plane isn't given an
// explicit one (Router.inviteTTL, set via WithInviteTTL): long enough for
// an invited teammate to actually see the email, short enough that a
// forgotten, unopened invite doesn't stay valid indefinitely.
const defaultInviteTTL = 7 * 24 * time.Hour

// inviteSendTimeout bounds handleCreateInvite's background send, the
// same reasoning as passwordResetSendTimeout (password_reset.go).
const inviteSendTimeout = 30 * time.Second

func randomInviteID() (string, error) {
	return randomOpaqueID("inv_")
}

// effectiveInviteTTL returns rt.inviteTTL, falling back to
// defaultInviteTTL when unset, the same "0 means use the default" shape
// sessionTTL/previewTTL already use.
func (rt *Router) effectiveInviteTTL() time.Duration {
	if rt.inviteTTL > 0 {
		return rt.inviteTTL
	}
	return defaultInviteTTL
}

// inviteResource is the wire shape for an invite in list/create
// responses. Expired is computed at read time from ExpiresAt rather than
// stored, so it's always accurate even for an invite that's sat in the
// list past its expiry without anyone revoking it.
type inviteResource struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role,omitempty"`
	Abilities []string  `json:"abilities"`
	CreatedBy string    `json:"created_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Expired   bool      `json:"expired"`
}

func toInviteResource(inv store.Invite) inviteResource {
	return inviteResource{
		ID:        inv.ID,
		Email:     inv.Email,
		Role:      inv.Role,
		Abilities: inv.Abilities,
		CreatedBy: inv.CreatedBy,
		CreatedAt: inv.CreatedAt,
		ExpiresAt: inv.ExpiresAt,
		Expired:   time.Now().After(inv.ExpiresAt),
	}
}

type createInviteRequest struct {
	Email string `json:"email"`
	// Role, when non-empty, names a curated preset (roles.go) applied
	// instead of Abilities, same precedence as createUserRequest.Role.
	Role      string   `json:"role,omitempty"`
	Abilities []string `json:"abilities,omitempty"`
}

// createInviteResponse is handleCreateInvite's own response shape: the
// invite plus Link, the accept URL carrying the plaintext token. Unlike
// a token's plaintext (CreateTokenDialog's "shown once" secret), Link is
// meant to be re-shareable by the admin (Slack, email forward), so it's
// not a one-time reveal, just never persisted anywhere itself, only its
// hash is (store.Invite.TokenHash).
type createInviteResponse struct {
	inviteResource
	Link string `json:"link"`
}

// handleCreateInvite handles POST /api/v1/invites: mints an invite token
// for one specific, named email and best-effort emails a link to it.
// AbilityRoot-gated (routes.go): an invite is a deferred version of
// POST /api/v1/auth/users's own "hand out access" action, so it needs
// the same authority, for the same reason that route documents. The
// response always carries Link itself, regardless of whether email
// delivery is configured or succeeds: CLAUDE.md's "email is best-effort,
// not required" rule means a control plane with no SMTP configured must
// stay fully usable via copy/paste, not degrade into a dead end.
func (rt *Router) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req createInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	abilities, err := resolveAbilities(req.Role, req.Abilities)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: create invite: generate token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	id, err := randomInviteID()
	if err != nil {
		rt.logger.Error("api: create invite: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	callerID, _ := rt.currentSessionUserID(r)
	now := time.Now().UTC()
	inv := store.Invite{
		ID:        id,
		Email:     req.Email,
		Role:      req.Role,
		Abilities: abilities,
		TokenHash: hashToken(plaintext),
		CreatedBy: callerID,
		CreatedAt: now,
		ExpiresAt: now.Add(rt.effectiveInviteTTL()),
	}
	if err := rt.invites.SaveInvite(r.Context(), inv); errors.Is(err, store.ErrInviteEmailExists) {
		writeError(w, http.StatusConflict, "a pending invite already exists for this email")
		return
	} else if err != nil {
		rt.logger.Error("api: create invite: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	link := rt.inviteURL(r.Context(), plaintext)
	go rt.sendInviteEmail(context.Background(), inv.Email, link) //nolint:gosec // deliberately outlives the request: the response is already written before this runs, same as sendPasswordResetEmail's own background send

	writeJSON(w, http.StatusCreated, createInviteResponse{inviteResource: toInviteResource(inv), Link: link})
}

// sendInviteEmail is handleCreateInvite's background half: every early
// return here is expected and silent to the caller, the response was
// already written with the link before this runs, matching
// sendPasswordResetEmail's own shape.
func (rt *Router) sendInviteEmail(ctx context.Context, email, link string) {
	ctx, cancel := context.WithTimeout(ctx, inviteSendTimeout)
	defer cancel()

	if rt.emailSender == nil {
		rt.logger.Warn("api: create invite: no email capability configured on this control plane")
		return
	}
	subject := fmt.Sprintf("[%s] You're invited", rt.brand.Name)
	body := fmt.Sprintf(
		"You've been invited to join %s.\n\nAccept the invite and set a password: %s\n\nThis link expires in %d days.",
		rt.brand.Name, link, int(rt.effectiveInviteTTL().Hours()/24),
	)
	if err := rt.emailSender.Send(ctx, email, subject, body); err != nil {
		rt.logger.Error("api: create invite: send email failed", slog.String("error", err.Error()))
	}
}

// inviteURL builds the link an invite's email points at and its create
// response returns, the same "absolute against the primary domain when
// set, bare path otherwise" shape passwordResetURL uses.
func (rt *Router) inviteURL(ctx context.Context, token string) string {
	const path = "/accept-invite?token="
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil || settings.PrimaryDomain == "" {
		return path + token
	}
	return "https://" + settings.PrimaryDomain + path + token
}

// handleListInvites handles GET /api/v1/invites: every invite not yet
// accepted or revoked, for the Users settings page's pending-invites
// list. AbilityRead, the same tier GET /api/v1/users uses: this is who
// has been invited, not a credential (the plaintext token itself is
// never returned here, only ever in the create response above).
func (rt *Router) handleListInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := rt.invites.ListPendingInvites(r.Context())
	if err != nil {
		rt.logger.Error("api: list invites failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]inviteResource, 0, len(invites))
	for _, inv := range invites {
		out = append(out, toInviteResource(inv))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeInvite handles DELETE /api/v1/invites/{id}: AbilityRoot,
// same tier as handleCreateInvite. Revoking an already-accepted invite is
// a 400, not a 404: the caller almost certainly meant "stop this from
// being usable," and an already-accepted invite already can't be, so the
// clearest response is saying why, not pretending the row never existed.
func (rt *Router) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := rt.invites.RevokeInvite(r.Context(), id)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrInviteNotFound):
		writeError(w, http.StatusNotFound, "invite not found")
	case errors.Is(err, store.ErrInviteAlreadyAccepted):
		writeError(w, http.StatusBadRequest, "invite already accepted")
	case errors.Is(err, store.ErrInviteAlreadyRevoked):
		writeError(w, http.StatusBadRequest, "invite already revoked")
	default:
		rt.logger.Error("api: revoke invite failed", slog.String("invite_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

type acceptInviteRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// errInvalidOrExpiredInvite covers every way a token can fail (not
// found, expired, already accepted, revoked): distinguishing them isn't
// worth the complexity, matching errInvalidOrExpiredResetToken's own
// reasoning in password_reset.go.
var errInvalidOrExpiredInvite = errors.New("invalid or expired invite")

// handleAcceptInvite handles POST /api/v1/invites/accept: necessarily
// unauthenticated, gated by possession of the invite token itself,
// exactly like handleResetPassword. Unlike that flow there is no
// existing account to update: this creates exactly the one user the
// invite named, through createLocalUser (users.go), the same insertion
// POST /api/v1/auth/users uses, so an invited account is never anything
// other than a normal user with the abilities the invite carried. Never
// open-ended: a token only ever accepts as the email it was minted for,
// with the abilities that were fixed at creation, not whatever the
// caller sends.
func (rt *Router) handleAcceptInvite(w http.ResponseWriter, r *http.Request) {
	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token is required")
		return
	}
	if len(req.Password) < minPasswordLength {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("password must be at least %d characters", minPasswordLength))
		return
	}

	inv, err := rt.invites.GetInviteByHash(r.Context(), hashToken(req.Token))
	if errors.Is(err, store.ErrInviteNotFound) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredInvite.Error())
		return
	}
	if err != nil {
		rt.logger.Error("api: accept invite: load invite failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv.AcceptedAt != nil || inv.RevokedAt != nil || time.Now().After(inv.ExpiresAt) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredInvite.Error())
		return
	}

	// Claimed before the user is created, the same "single atomic race
	// point" reasoning ClaimPasswordResetToken's own doc comment gives:
	// at most one of two concurrent accepts can win this update, so at
	// most one user ever gets created from one invite.
	if err := rt.invites.ClaimInvite(r.Context(), inv.ID); err != nil {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredInvite.Error())
		return
	}

	user, err := rt.createLocalUser(r.Context(), inv.Email, "", req.Password, inv.Abilities)
	if errors.Is(err, store.ErrUserEmailExists) {
		writeError(w, http.StatusConflict, "a user with this email already exists")
		return
	} else if err != nil {
		rt.logger.Error("api: accept invite: create user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.establishSession(r.Context(), w, user); err != nil {
		rt.logger.Error("api: accept invite: establish session failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}
