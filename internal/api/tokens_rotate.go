package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/authengine"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// rotateTokenResponse is the new token with its secret, shown once, and when the old one stops working.
type rotateTokenResponse struct {
	createTokenResponse
	Replaces string `json:"replaces"`
	// PreviousValidUntil is null when the old token was revoked at once.
	PreviousValidUntil *time.Time `json:"previous_valid_until"`
}

// sessionOrSelfToken lets a signed-in session or a write token reach the handler;
// the handler then limits a bearer token to rotating itself.
func (rt *Router) sessionOrSelfToken(next http.HandlerFunc) http.HandlerFunc {
	session := rt.requireAuth(next)
	bearer := rt.requireAbility(AbilityWrite, next)
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := bearerToken(r); ok {
			bearer(w, r)
			return
		}
		session(w, r)
	}
}

// rotationGrace is how long the old token keeps working, 0 without a store to remember the revocation.
func (rt *Router) rotationGrace(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil || rt.gitConnSecrets == nil {
		return 0
	}
	return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyTokensRotationGrace)
}

// rotationAfterMint runs between minting the replacement and rechecking the old token; tests set it.
var rotationAfterMint func(oldID string)

// handleRotateToken handles POST /api/v1/auth/tokens/{id}/rotate: a replacement
// with the same name, abilities, scope, policies and a fresh lifetime. The old
// token is revoked after the grace period. Platform tokens are refused.
func (rt *Router) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	if _, ok := bearerToken(r); ok {
		// requireAbility already audits this request with the token as the actor.
		rt.rotateToken(w, r, "", false)
		return
	}
	ctx := r.Context()
	callerID, _ := rt.currentSessionUserID(r)
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	rt.rotateToken(rec, r, callerID, true)
	rt.recordAudit(ctx, r, AbilityWrite, auditActorSession, callerID, rt.auditActorName(ctx, auditActorSession, callerID, ""), rec.status)
}

// rotateToken does the rotation for a session caller, or for a bearer token rotating itself.
func (rt *Router) rotateToken(w http.ResponseWriter, r *http.Request, callerID string, fromSession bool) {
	ctx, id := r.Context(), r.PathValue("id")
	if !fromSession {
		raw, _ := bearerToken(r)
		self, err := rt.lookupBearerToken(ctx, raw)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if self.ID != id {
			writeError(w, http.StatusForbidden, "a token can only rotate itself")
			return
		}
		callerID = self.OwnerUserID
	}
	old, ok := rt.findTokenForRotation(ctx, id, callerID, rt.callerIsAdmin(r))
	switch {
	case !ok:
		writeError(w, http.StatusNotFound, "token not found")
		return
	case isPlatformToken(old) || old.OwnerUserID == "":
		writeError(w, http.StatusForbidden, "platform tokens rotate themselves and cannot be rotated here")
		return
	case old.RevokedAt != nil:
		writeError(w, http.StatusConflict, "token is already revoked")
		return
	case isMCPOAuthTokenResource(old):
		writeError(w, http.StatusConflict, "tokens issued to an MCP client through OAuth sign-in are renewed by that client: reconnect the client instead of rotating")
		return
	}
	release, claimed := rt.tokenState.claimRotation(id)
	if !claimed {
		writeError(w, http.StatusConflict, "a rotation of this token is already in progress")
		return
	}
	defer release()
	pending, err := rt.tokenStateLoad(ctx, tokenStateRotation)
	if err != nil {
		rt.internalError(w, "api: rotate token: read pending rotations failed", err, slog.String("token_id", id))
		return
	}
	if _, rotating := pending[id]; rotating {
		writeError(w, http.StatusConflict, "token was already rotated and stays valid only until its grace period ends: use its replacement")
		return
	}
	ownerAbilities, status, problem := rt.rotationAbilitiesProblem(r, old, callerID)
	if problem != "" {
		writeError(w, status, problem)
		return
	}
	expiresAt, problem := rt.rotationExpiry(ctx, old, hasAbility(ownerAbilities, AbilityRoot))
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}
	raw, fresh, err := rt.mintReplacement(ctx, old, expiresAt)
	if err != nil {
		rt.internalError(w, "api: rotate token: mint replacement failed", err, slog.String("token_id", id))
		return
	}
	if rotationAfterMint != nil {
		rotationAfterMint(id)
	}
	// A revoke that landed while minting must not leave a live replacement behind.
	if cur, live := rt.findTokenForRotation(ctx, id, old.OwnerUserID, false); !live || cur.RevokedAt != nil {
		rt.abandonReplacement(ctx, fresh.ID)
		writeError(w, http.StatusConflict, "token was revoked during the rotation, so its replacement was revoked too")
		return
	}
	grace := rt.rotationGrace(ctx)
	var validUntil *time.Time
	if grace > 0 {
		until := time.Now().Add(grace)
		validUntil = &until
		already := false
		err := rt.tokenStateUpdate(ctx, tokenStateRotation, func(m map[string]int64) {
			// An existing pending revocation keeps its earlier time.
			if _, ok := m[id]; ok {
				already = true
				return
			}
			m[id] = until.Unix()
		})
		switch {
		case err == nil && already:
			rt.abandonReplacement(ctx, fresh.ID)
			writeError(w, http.StatusConflict, "token was already rotated and stays valid only until its grace period ends: use its replacement")
			return
		case err != nil:
			validUntil = nil
			grace = 0
			rt.logger.Warn("api: rotate token: remember grace failed", slog.String("token_id", id), slog.String("error", err.Error()))
		}
	}
	if grace == 0 {
		if err := rt.libraryRevokeToken(ctx, id); err != nil && !isTokenGone(err) {
			rt.internalError(w, "api: rotate token: revoke old failed", err, slog.String("token_id", id))
			return
		}
	}
	rt.EmitEvent(eventTokenRotated, map[string]any{"token_id": id, "replacement_id": fresh.ID, "name": old.Name})
	_ = rt.tokenStateUpdate(ctx, tokenStateNotices, func(m map[string]int64) { delete(m, id) })
	writeJSON(w, http.StatusCreated, rotateTokenResponse{
		createTokenResponse: createTokenResponse{tokenResource: fresh, Token: raw}, Replaces: id, PreviousValidUntil: validUntil,
	})
}

// isMCPOAuthTokenResource reports a token issued through MCP OAuth sign-in: its grant names the old token id.
func isMCPOAuthTokenResource(t tokenResource) bool {
	return t.Agent != nil && strings.HasPrefix(t.Agent.Name, mcpOAuthAgentPrefix)
}

// rotationAbilitiesProblem holds the replacement to the checks a new token
// gets: every ability must still be held by the owner, and the session-only
// abilities carry over only when the owner rotates from a signed-in session.
func (rt *Router) rotationAbilitiesProblem(r *http.Request, old tokenResource, callerID string) ([]string, int, string) {
	owner, err := rt.auth.GetUserByID(r.Context(), old.OwnerUserID)
	if errors.Is(err, store.ErrUserNotFound) {
		return nil, http.StatusForbidden, "the token owner no longer exists, so the token cannot be rotated"
	}
	if err != nil {
		rt.logger.Warn("api: rotate token: load owner failed", slog.String("token_id", old.ID), slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "could not load the token owner"
	}
	ownerSession := rt.signedInUserSession(r) && callerID == old.OwnerUserID
	for _, a := range old.Abilities {
		switch {
		case a == AbilitySignInApprove && !ownerSession:
			return nil, http.StatusForbidden, AbilitySignInApprove + " carries over only when the token owner rotates from a signed-in session"
		case a == AbilitySecretsReveal && (!ownerSession || !hasAbility(owner.Abilities, AbilityReadSensitive)):
			return nil, http.StatusForbidden, AbilitySecretsReveal + " carries over only when the token owner rotates from a signed-in session and still holds " + AbilityReadSensitive
		case a == AbilitySignInApprove || a == AbilitySecretsReveal:
		case !hasAbility(owner.Abilities, a):
			return nil, http.StatusForbidden, fmt.Sprintf("the token owner no longer holds %s: create a new token with the abilities they hold now", a)
		}
	}
	return owner.Abilities, 0, ""
}

// abandonReplacement revokes a replacement that must not outlive a refused rotation.
func (rt *Router) abandonReplacement(ctx context.Context, id string) {
	if err := rt.libraryRevokeToken(ctx, id); err != nil && !isTokenGone(err) {
		rt.logger.Warn("api: rotate token: revoke abandoned replacement failed", slog.String("token_id", id), slog.String("error", err.Error()))
	}
}

func (rt *Router) findTokenForRotation(ctx context.Context, id, callerID string, admin bool) (tokenResource, bool) {
	owner := callerID
	if admin {
		owner = ""
	}
	toks, err := rt.libraryListTokens(ctx, owner, admin)
	if err != nil {
		rt.logger.Warn("api: rotate token: list failed", slog.String("error", err.Error()))
		return tokenResource{}, false
	}
	for _, t := range toks {
		if t.ID == id && (admin || t.OwnerUserID == callerID) {
			return t, true
		}
	}
	return tokenResource{}, false
}

// rotationExpiry keeps the old lifetime within the policy. A token without an
// expiry keeps none only while its owner holds root and no maximum lifetime is set.
func (rt *Router) rotationExpiry(ctx context.Context, old tokenResource, ownerRoot bool) (*time.Time, string) {
	days := 0
	if old.ExpiresAt != nil {
		days = int((old.ExpiresAt.Sub(old.CreatedAt) + 24*time.Hour - 1) / (24 * time.Hour))
		if days < 1 {
			days = rt.defaultTokenLifetimeDays(ctx)
		}
	}
	msg, err := rt.tokenLifetimeProblem(ctx, days)
	if err != nil {
		return nil, "could not load the security policy"
	}
	if days == 0 && msg == "" && ownerRoot {
		return nil, ""
	}
	if msg != "" || days == 0 {
		days = rt.defaultTokenLifetimeDays(ctx)
	}
	exp := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return &exp, ""
}

// mintReplacement mints the new token and copies the old one's scope and policies.
func (rt *Router) mintReplacement(ctx context.Context, old tokenResource, expiresAt *time.Time) (string, tokenResource, error) {
	legacyID, err := randomTokenID()
	if err != nil {
		return "", tokenResource{}, fmt.Errorf("generate id: %w", err)
	}
	var agent agentIdentity
	if old.Agent != nil {
		agent = *old.Agent
	}
	raw, rec, err := rt.authLib.tokens.MintToken(ctx, authengine.MintInput{
		OwnerLegacyID: old.OwnerUserID, Name: old.Name, Abilities: old.Abilities, ExpiresAt: expiresAt, AgentName: agent.Name,
	})
	if err != nil {
		return "", tokenResource{}, fmt.Errorf("library mint: %w", err)
	}
	legacy := store.APIToken{
		ID: legacyID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: old.Abilities, CreatedAt: rec.CreatedAt,
		ExpiresAt: expiresAt, AgentName: agent.Name, AgentDescription: agent.Description, OwnerUserID: old.OwnerUserID,
	}
	undo := func() { _ = rt.authLib.tokens.RevokeToken(ctx, rec.EngineID) }
	if err := rt.tokens.SaveAPIToken(ctx, legacy); err != nil {
		undo()
		return "", tokenResource{}, fmt.Errorf("mirror save: %w", err)
	}
	if err := rt.copyTokenScope(ctx, old.ID, legacyID); err != nil {
		undo()
		_ = rt.tokens.RevokeAPIToken(ctx, legacyID)
		return "", tokenResource{}, err
	}
	if err := rt.authLib.tokens.LinkToken(ctx, legacyID, rec.EngineID); err != nil {
		undo()
		_ = rt.tokens.RevokeAPIToken(ctx, legacyID)
		return "", tokenResource{}, fmt.Errorf("link: %w", err)
	}
	rt.copyTokenPolicies(ctx, old.ID, legacyID)
	res := rt.withPolicyAccess(ctx, toTokenResource(legacy))
	res.Scope = rt.tokenScopeOf(ctx, legacyID)
	return raw, res, nil
}

func (rt *Router) copyTokenScope(ctx context.Context, fromID, toID string) error {
	scope := &store.APITokenScope{TokenID: toID}
	found := false
	if db := rt.teamDB(); db != nil {
		s, err := db.GetAPITokenScope(ctx, fromID)
		if err != nil {
			return fmt.Errorf("read team scope: %w", err)
		}
		if s != nil {
			scope.TeamID, scope.ProjectID, scope.Role, found = s.TeamID, s.ProjectID, s.Role, true
		}
	}
	if ss := rt.tokenSiteScopes(); ss != nil {
		s, err := ss.GetAPITokenSiteScope(ctx, fromID)
		if err != nil {
			return fmt.Errorf("read site scope: %w", err)
		}
		if s != nil {
			scope.Site, found = &store.APITokenSiteScope{TokenID: toID, SiteName: s.SiteName}, true
		}
	}
	if ss := rt.tokenStackScopes(); ss != nil {
		s, err := ss.GetAPITokenStackScope(ctx, fromID)
		if err != nil {
			return fmt.Errorf("read stack scope: %w", err)
		}
		if s != nil {
			scope.Stack, found = &store.APITokenStackScope{TokenID: toID, StackID: s.StackID, Verbs: s.Verbs}, true
		}
	}
	if !found {
		return nil
	}
	if err := rt.storeTokenScope(ctx, scope); err != nil {
		return fmt.Errorf("copy scope: %w", err)
	}
	return nil
}

// copyTokenPolicies is best effort: the replacement is never broader than the old token.
func (rt *Router) copyTokenPolicies(ctx context.Context, fromID, toID string) {
	if rt.policies == nil {
		return
	}
	attached, err := rt.policies.ListPoliciesForPrincipal(ctx, store.PrincipalTypeToken, fromID)
	if err != nil {
		rt.logger.Warn("api: rotate token: list policies failed", slog.String("token_id", fromID), slog.String("error", err.Error()))
		return
	}
	for _, p := range attached {
		aid, err := store.NewPolicyAttachmentID()
		if err == nil {
			err = rt.policies.AttachPolicy(ctx, aid, p.ID, store.PrincipalTypeToken, toID)
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			rt.logger.Warn("api: rotate token: attach policy failed", slog.String("token_id", toID), slog.String("policy_id", p.ID))
		}
	}
}
