package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// tokenResource is the wire shape for a token in list responses: never
// the token secret itself (that's returned exactly once, by
// handleCreateToken's response, and never again), only enough for an
// operator to recognize and manage it.
type tokenResource struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Abilities  []string   `json:"abilities"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func toTokenResource(t store.APIToken) tokenResource {
	return tokenResource{
		ID:         t.ID,
		Name:       t.Name,
		Abilities:  t.Abilities,
		CreatedAt:  t.CreatedAt,
		LastUsedAt: t.LastUsedAt,
		ExpiresAt:  t.ExpiresAt,
		RevokedAt:  t.RevokedAt,
	}
}

type createTokenRequest struct {
	Name      string   `json:"name"`
	Abilities []string `json:"abilities"`
	// ExpiresInDays is optional; omitted or 0 means the token never
	// expires (Dokploy's own "never" option, per
	// docs-local/research/competitor-onboarding-auth-ux.md finding 10,
	// which Coolify's forced-expiry-only picker doesn't offer).
	ExpiresInDays int `json:"expires_in_days,omitempty"`
}

type createTokenResponse struct {
	tokenResource
	// Token is the plaintext credential, present only in this one
	// response, GitHub PAT convention: shown once, never recoverable
	// again, matching TASKS.md's note on this exact endpoint.
	Token string `json:"token"`
}

// handleCreateToken handles POST /api/v1/auth/tokens. Session-only (see
// router.go's registration comment): a token can never mint another
// token on its own behalf.
func (rt *Router) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validateAbilities(req.Abilities); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ExpiresInDays < 0 {
		writeError(w, http.StatusBadRequest, "expires_in_days must not be negative")
		return
	}

	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: create token: generate token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomTokenID()
	if err != nil {
		rt.logger.Error("api: create token: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rec := store.APIToken{
		ID:        id,
		Name:      req.Name,
		TokenHash: hashToken(plaintext),
		Abilities: req.Abilities,
		CreatedAt: time.Now(),
	}
	if req.ExpiresInDays > 0 {
		expires := rec.CreatedAt.Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		rec.ExpiresAt = &expires
	}

	if err := rt.tokens.SaveAPIToken(r.Context(), rec); err != nil {
		rt.logger.Error("api: create token: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, createTokenResponse{
		tokenResource: toTokenResource(rec),
		Token:         plaintext,
	})
}

// handleListTokens handles GET /api/v1/auth/tokens. Never returns a
// token secret, including for already-revoked rows: once a token is
// minted, its plaintext is gone from this API's world entirely.
func (rt *Router) handleListTokens(w http.ResponseWriter, r *http.Request) {
	recs, err := rt.tokens.ListAPITokens(r.Context())
	if err != nil {
		rt.logger.Error("api: list tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]tokenResource, 0, len(recs))
	for _, t := range recs {
		out = append(out, toTokenResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeToken handles DELETE /api/v1/auth/tokens/{id}. Idempotent
// at the store layer (RevokeAPIToken), so this only 404s for a token id
// that never existed at all, not one already revoked.
func (rt *Router) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := rt.tokens.RevokeAPIToken(r.Context(), id)
	if errors.Is(err, store.ErrAPITokenNotFound) {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: revoke token failed", slog.String("error", err.Error()), slog.String("token_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// randomTokenID generates a short, URL-safe, non-secret identifier for
// a token row: distinct from the token's own secret value (hashToken
// covers that), this is just a stable handle a client uses to name the
// token in list/revoke calls, safe to log and display.
func randomTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate token id: %w", err)
	}
	return "tok_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
