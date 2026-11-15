package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
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

// handleRotateToken handles POST /api/v1/auth/tokens/{id}/rotate: a replacement
// with the same name, abilities, scope, policies and a fresh lifetime. The old
// token is revoked after the grace period. Platform tokens are refused.
func (rt *Router) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	ctx, id := r.Context(), r.PathValue("id")
	callerID, isSession := rt.currentSessionUserID(r)
	if raw, ok := bearerToken(r); ok && !isSession {
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
	admin := rt.callerIsAdmin(r)
	old, ok := rt.findTokenForRotation(ctx, id, callerID, admin)
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
	}
	expiresAt, problem := rt.rotationExpiry(ctx, old)
	if problem != "" {
		writeError(w, http.StatusBadRequest, problem)
		return
	}
	raw, fresh, err := rt.mintReplacement(ctx, old, expiresAt)
	if err != nil {
		rt.internalError(w, "api: rotate token: mint replacement failed", err, slog.String("token_id", id))
		return
	}
	grace := rt.rotationGrace(ctx)
	var validUntil *time.Time
	if grace > 0 {
		until := time.Now().Add(grace)
		validUntil = &until
		if err := rt.tokenStateUpdate(ctx, tokenStateRotation, func(m map[string]int64) { m[id] = until.Unix() }); err != nil {
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
	rt.recordAudit(ctx, r, AbilityWrite, auditActorSession, callerID, "", http.StatusCreated)
	rt.EmitEvent(eventTokenRotated, map[string]any{"token_id": id, "replacement_id": fresh.ID, "name": old.Name})
	_ = rt.tokenStateUpdate(ctx, tokenStateNotices, func(m map[string]int64) { delete(m, id) })
	writeJSON(w, http.StatusCreated, rotateTokenResponse{
		createTokenResponse: createTokenResponse{tokenResource: fresh, Token: raw}, Replaces: id, PreviousValidUntil: validUntil,
	})
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

// rotationExpiry keeps the old lifetime, or no expiry when the old token had none.
func (rt *Router) rotationExpiry(ctx context.Context, old tokenResource) (*time.Time, string) {
	if old.ExpiresAt == nil {
		return nil, ""
	}
	days := int((old.ExpiresAt.Sub(old.CreatedAt) + 24*time.Hour - 1) / (24 * time.Hour))
	if days < 1 {
		days = rt.defaultTokenLifetimeDays(ctx)
	}
	msg, err := rt.tokenLifetimeProblem(ctx, days)
	if err != nil {
		return nil, "could not load the security policy"
	}
	if msg != "" {
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
	sc := rt.tokenScopeOf(ctx, fromID)
	if sc == nil {
		return nil
	}
	scope := &store.APITokenScope{TokenID: toID, TeamID: sc.TeamID, ProjectID: sc.ProjectID, Role: sc.TeamRole}
	if sc.StackID != "" {
		scope.Stack = &store.APITokenStackScope{TokenID: toID, StackID: sc.StackID, Verbs: sc.StackVerbs}
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
