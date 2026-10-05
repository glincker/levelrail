package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// authLibState holds the library hooks. Every field is nil in legacy mode.
type authLibState struct {
	tokens *authengine.Engine
	device *authengine.Engine
	shadow *authengine.Shadow
	mode   string
}

// WithAuthEngineLibrary routes the areas listed in APP_AUTH_ENGINE_AREAS
// (tokens, device) through the library. A nil engine is a no-op.
func WithAuthEngineLibrary(e *authengine.Engine) Option {
	return func(rt *Router) {
		if e == nil {
			return
		}
		if authengine.AreaActive(authengine.AreaTokens) {
			rt.authLib.tokens = e
		}
		if authengine.AreaActive(authengine.AreaDevice) {
			rt.authLib.device = e
		}
	}
}

// WithAuthEngineShadow compares bearer tokens against the library in the
// background while the legacy engine keeps deciding. A nil engine is a no-op.
func WithAuthEngineShadow(e *authengine.Engine, cfg authengine.ShadowConfig) Option {
	return func(rt *Router) {
		if e == nil {
			return
		}
		rt.authLib.shadow = authengine.NewShadow(e, rt.legacyOutcome, cfg, rt.logger)
	}
}

// CloseAuthShadow stops the shadow workers, if any.
func (rt *Router) CloseAuthShadow() {
	if rt.authLib.shadow != nil {
		rt.authLib.shadow.Close()
	}
}

func (rt *Router) observeShadow(raw string) {
	if rt.authLib.shadow != nil {
		rt.authLib.shadow.Observe(raw)
	}
}

// legacyOutcome is the legacy engine's decision for a bearer secret, in the
// form the shadow comparison needs.
func (rt *Router) legacyOutcome(ctx context.Context, raw string) (authengine.LegacyOutcome, error) {
	rec, err := rt.tokens.GetAPITokenByHash(ctx, hashToken(raw))
	if errors.Is(err, store.ErrAPITokenNotFound) {
		return authengine.LegacyOutcome{}, nil
	}
	if err != nil {
		return authengine.LegacyOutcome{}, fmt.Errorf("api: shadow legacy lookup: %w", err)
	}
	out := authengine.LegacyOutcome{TokenID: rec.ID, OwnerID: rec.OwnerUserID, Abilities: rec.Abilities}
	if rec.RevokedAt != nil || (rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt)) {
		return out, nil
	}
	gone, err := rt.tokenOwnerGone(ctx, rec)
	if err != nil {
		return authengine.LegacyOutcome{}, err
	}
	out.Accepted = !gone
	return out, nil
}

// lookupBearerToken resolves a raw bearer secret to a token record: the
// legacy table by default, the library when the tokens area is cut over.
func (rt *Router) lookupBearerToken(ctx context.Context, raw string) (*store.APIToken, error) {
	if rt.authLib.tokens == nil {
		return rt.tokens.GetAPITokenByHash(ctx, hashToken(raw))
	}
	rec, err := rt.authLib.tokens.LookupBearer(ctx, raw)
	switch {
	case errors.Is(err, authengine.ErrTokenUnknown):
		return rt.legacySystemToken(ctx, raw)
	case errors.Is(err, authengine.ErrTokenRejected):
		return nil, store.ErrAPITokenNotFound
	case err != nil:
		return nil, fmt.Errorf("api: library token lookup: %w", err)
	}
	return libraryTokenToStore(rec), nil
}

// legacySystemToken serves tokens the library never holds (system-minted, no
// owner). An owned token missing from the library means the backfill has not
// run, so it is refused rather than silently served by the old path.
func (rt *Router) legacySystemToken(ctx context.Context, raw string) (*store.APIToken, error) {
	rec, err := rt.tokens.GetAPITokenByHash(ctx, hashToken(raw))
	if err != nil {
		return nil, err
	}
	if rec.OwnerUserID != "" {
		rt.logger.Warn("api: token missing from the auth library, run the auth backfill", slog.String("token_id", rec.ID))
		return nil, store.ErrAPITokenNotFound
	}
	return rec, nil
}

func libraryTokenToStore(r authengine.TokenRecord) *store.APIToken {
	return &store.APIToken{
		ID: r.ID, Name: r.Name, TokenHash: r.HashHex, Abilities: r.Abilities, CreatedAt: r.CreatedAt,
		LastUsedAt: r.LastUsedAt, ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
		AgentName: r.AgentName, OwnerUserID: r.OwnerLegacyID,
	}
}

// libraryCreateToken is handleCreateToken's mint step when tokens run on the
// library. The legacy table gets a matching row so a rollback loses nothing.
func (rt *Router) libraryCreateToken(w http.ResponseWriter, r *http.Request, req createTokenRequest, agent agentIdentity, ownerID string, expiresAt *time.Time) {
	legacyID, err := randomTokenID()
	if err != nil {
		rt.internalError(w, "api: create token: generate id failed", err)
		return
	}
	raw, rec, err := rt.authLib.tokens.MintToken(r.Context(), authengine.MintInput{
		OwnerLegacyID: ownerID, Name: req.Name, Abilities: req.Abilities,
		ExpiresAt: expiresAt, AgentName: agent.Name,
	})
	if err != nil {
		rt.internalError(w, "api: create token: library mint failed", err)
		return
	}
	legacy := store.APIToken{
		ID: legacyID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: req.Abilities, CreatedAt: rec.CreatedAt,
		ExpiresAt: expiresAt, AgentName: agent.Name, AgentDescription: agent.Description, OwnerUserID: ownerID,
	}
	if err := rt.tokens.SaveAPIToken(r.Context(), legacy); err != nil {
		if rerr := rt.authLib.tokens.RevokeToken(r.Context(), rec.EngineID); rerr != nil {
			rt.logger.Error("api: create token: revoke after failed mirror", slog.String("token_id", legacyID), slog.String("error", rerr.Error()))
		}
		rt.internalError(w, "api: create token: mirror save failed", err, slog.String("token_id", legacyID))
		return
	}
	if err := rt.authLib.tokens.LinkToken(r.Context(), legacyID, rec.EngineID); err != nil {
		rt.internalError(w, "api: create token: link failed", err, slog.String("token_id", legacyID))
		return
	}
	rt.recordAudit(r.Context(), r, AbilityWrite, auditActorSession, ownerID, "", http.StatusCreated)
	writeJSON(w, http.StatusCreated, createTokenResponse{tokenResource: toTokenResource(legacy), Token: raw})
}

// libraryListTokens is handleListTokens when tokens run on the library.
func (rt *Router) libraryListTokens(ctx context.Context, callerID string, admin bool) ([]tokenResource, error) {
	owner := callerID
	if admin {
		owner = ""
	}
	recs, err := rt.authLib.tokens.ListTokens(ctx, owner)
	if err != nil {
		return nil, fmt.Errorf("api: list tokens: %w", err)
	}
	legacy, err := rt.tokens.ListAPITokens(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: list tokens: legacy mirror: %w", err)
	}
	byID := make(map[string]store.APIToken, len(legacy))
	for _, l := range legacy {
		byID[l.ID] = l
	}
	merged := make([]store.APIToken, 0, len(recs))
	for _, r := range recs {
		t := *libraryTokenToStore(r)
		t.AgentDescription = byID[r.ID].AgentDescription
		merged = append(merged, t)
		delete(byID, r.ID)
	}
	if admin {
		for _, l := range byID {
			if l.OwnerUserID == "" {
				merged = append(merged, l)
			}
		}
	}
	slices.SortStableFunc(merged, func(a, b store.APIToken) int { return b.CreatedAt.Compare(a.CreatedAt) })
	out := make([]tokenResource, 0, len(merged))
	for _, t := range merged {
		out = append(out, toTokenResource(t))
	}
	return out, nil
}

// libraryRevokeToken revokes in the library and in the legacy mirror.
// It returns store.ErrAPITokenNotFound for an unknown id.
func (rt *Router) libraryRevokeToken(ctx context.Context, id string) error {
	err := rt.authLib.tokens.RevokeToken(ctx, id)
	if errors.Is(err, authengine.ErrTokenUnknown) {
		return rt.tokens.RevokeAPIToken(ctx, id)
	}
	if err != nil {
		return fmt.Errorf("api: revoke token: %w", err)
	}
	if lerr := rt.tokens.RevokeAPIToken(ctx, id); lerr != nil && !errors.Is(lerr, store.ErrAPITokenNotFound) {
		return fmt.Errorf("api: revoke token: legacy mirror: %w", lerr)
	}
	return nil
}
