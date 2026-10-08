package api

import (
	"context"
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

// auditActorSession is the audit actor type for a cookie-session caller.
const auditActorSession = "session"

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
	// Agent is set when the token was issued to an AI agent.
	Agent *agentIdentity `json:"agent,omitempty"`
}

func toTokenResource(t store.APIToken) tokenResource {
	var agent *agentIdentity
	if t.AgentName != "" {
		agent = &agentIdentity{Name: t.AgentName, Description: t.AgentDescription}
	}
	return tokenResource{
		Agent:      agent,
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
	// expires (Dokploy's own "never" option, which Coolify's
	// forced-expiry-only picker doesn't offer).
	ExpiresInDays int `json:"expires_in_days,omitempty"`
	// Agent optionally labels the token as issued to an AI agent.
	Agent *agentIdentity `json:"agent,omitempty"`
}

type createTokenResponse struct {
	tokenResource
	// Token is the plaintext credential, present only in this one
	// response, GitHub PAT convention: shown once, never recoverable
	// again.
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

	callerAbilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.logger.Error("api: create token: resolve caller abilities failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	for _, a := range req.Abilities {
		if !hasAbility(callerAbilities, a) {
			writeError(w, http.StatusForbidden, fmt.Sprintf("cannot mint a token with abilities you don't hold yourself: %s", a))
			return
		}
	}

	var expiresAt *time.Time
	if req.ExpiresInDays > 0 {
		expires := time.Now().Add(time.Duration(req.ExpiresInDays) * 24 * time.Hour)
		expiresAt = &expires
	}

	agent, err := validateAgentIdentity(req.Agent)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ownerID, _ := rt.currentSessionUserID(r)
	rt.libraryCreateToken(w, r, req, agent, ownerID, expiresAt)
}

// sessionOrRootToken serves read-only token metadata to a session or to a
// bearer token holding root; creating and revoking stay session-only.
func (rt *Router) sessionOrRootToken(next http.HandlerFunc) http.HandlerFunc {
	session := rt.requireAuth(next)
	root := rt.requireAbility(AbilityRoot, next)
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := bearerToken(r); ok {
			root(w, r)
			return
		}
		session(w, r)
	}
}

// handleListTokens handles GET /api/v1/auth/tokens. Never returns a
// token secret, including for already-revoked rows: once a token is
// minted, its plaintext is gone from this API's world entirely.
func (rt *Router) handleListTokens(w http.ResponseWriter, r *http.Request) {
	callerID, _ := rt.currentSessionUserID(r)
	out, err := rt.libraryListTokens(r.Context(), callerID, rt.callerIsAdmin(r))
	if err != nil {
		rt.logger.Error("api: list tokens failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeToken handles DELETE /api/v1/auth/tokens/{id}. Idempotent
// at the store layer (RevokeAPIToken), so this only 404s for a token id
// that never existed at all, not one already revoked.
func (rt *Router) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	callerID, _ := rt.currentSessionUserID(r)
	if !rt.callerIsAdmin(r) {
		owned, oerr := rt.tokens.GetAPITokenByID(r.Context(), id)
		if oerr == nil && owned.OwnerUserID != callerID {
			oerr = store.ErrAPITokenNotFound
		}
		if errors.Is(oerr, store.ErrAPITokenNotFound) {
			writeError(w, http.StatusNotFound, "token not found")
			return
		}
		if oerr != nil {
			rt.logger.Error("api: revoke token: load failed", slog.String("error", oerr.Error()), slog.String("token_id", id))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	err := rt.libraryRevokeToken(r.Context(), id)
	if errors.Is(err, store.ErrAPITokenNotFound) {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: revoke token failed", slog.String("error", err.Error()), slog.String("token_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.recordAudit(r.Context(), r, AbilityWrite, auditActorSession, callerID, "", http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}

// callerIsAdmin reports whether the session caller holds root.
func (rt *Router) callerIsAdmin(r *http.Request) bool {
	abilities, err := rt.callerAbilities(r)
	return err == nil && hasAbility(abilities, AbilityRoot)
}

// MintAPIToken generates, hashes, and persists a new API token,
// returning its plaintext exactly once, same as handleCreateToken's own
// response. Exported so a non-HTTP caller (cmd/levelrail's own internal
// AI assistant self-call token, minted once at startup rather than by a
// human through this handler) can mint through the identical path
// instead of duplicating it.
func MintAPIToken(ctx context.Context, tokens TokenStore, name string, abilities []string, expiresAt *time.Time) (string, store.APIToken, error) {
	return MintAgentAPIToken(ctx, tokens, name, abilities, expiresAt, agentIdentity{}, "")
}

// MintAgentAPIToken is MintAPIToken with an agent label and an owning user (empty for system tokens).
func MintAgentAPIToken(ctx context.Context, tokens TokenStore, name string, abilities []string, expiresAt *time.Time, agent agentIdentity, ownerUserID string) (string, store.APIToken, error) {
	plaintext, err := randomToken()
	if err != nil {
		return "", store.APIToken{}, fmt.Errorf("api: mint token: generate token: %w", err)
	}
	id, err := randomTokenID()
	if err != nil {
		return "", store.APIToken{}, fmt.Errorf("api: mint token: generate id: %w", err)
	}
	rec := store.APIToken{
		ID:        id,
		Name:      name,
		TokenHash: hashToken(plaintext),
		Abilities: abilities,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,

		AgentName: agent.Name, AgentDescription: agent.Description,
		OwnerUserID: ownerUserID,
	}
	if err := tokens.SaveAPIToken(ctx, rec); err != nil {
		return "", store.APIToken{}, fmt.Errorf("api: mint token: save: %w", err)
	}
	return plaintext, rec, nil
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
