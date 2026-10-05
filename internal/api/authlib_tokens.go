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

// authLibState holds the library engine that serves tokens and device login.
type authLibState struct {
	tokens *authengine.Engine
	device *authengine.Engine
}

// lookupBearerToken resolves a raw bearer secret to a token record through
// the library, falling back to the platform table for system-minted tokens.
func (rt *Router) lookupBearerToken(ctx context.Context, raw string) (*store.APIToken, error) {
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
// owner). An owned token missing from the library is refused.
func (rt *Router) legacySystemToken(ctx context.Context, raw string) (*store.APIToken, error) {
	rec, err := rt.tokens.GetAPITokenByHash(ctx, hashToken(raw))
	if err != nil {
		return nil, err
	}
	if rec.OwnerUserID != "" {
		rt.logger.Warn("api: owned token missing from the auth library", slog.String("token_id", rec.ID))
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
// library. The platform table gets a matching row for the agent description and system listing.
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
