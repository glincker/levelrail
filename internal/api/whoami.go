package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type tokenIdentityKey struct{}

// withTokenIdentity lets handleWhoami read the already-resolved token
// record instead of repeating requireAbilityDecided's lookup.
func withTokenIdentity(ctx context.Context, rec *store.APIToken) context.Context {
	return context.WithValue(ctx, tokenIdentityKey{}, rec)
}

// allowAnyAuthenticated passes every caller requireAbilityDecided already
// authenticated: whoami needs a valid principal, not a specific ability.
func allowAnyAuthenticated(context.Context, string, string, []string) (bool, error) {
	return true, nil
}

type whoamiResponse struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Abilities []string `json:"abilities"`
	ExpiresAt string   `json:"expires_at"`
}

// handleWhoami handles GET /api/v1/auth/whoami: the caller's own identity
// for a session, a pinned session link, or a bearer API token.
func (rt *Router) handleWhoami(w http.ResponseWriter, r *http.Request) {
	if pinned, ok := rt.currentPinnedSession(r); ok {
		writeJSON(w, http.StatusOK, whoamiResponse{
			Kind: "session", Name: pinned.pinnedDisplayName, Abilities: nonNil(pinned.pinnedAbilities),
			ExpiresAt: pinned.expiresAt.UTC().Format(time.RFC3339),
		})
		return
	}
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			rt.logger.Error("api: whoami: load user failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		expires := ""
		if c, err := r.Cookie(sessionCookieName); err == nil {
			if sess, ok := rt.sessions.get(c.Value); ok {
				expires = sess.expiresAt.UTC().Format(time.RFC3339)
			}
		}
		writeJSON(w, http.StatusOK, whoamiResponse{Kind: "session", Name: user.Email, Abilities: nonNil(user.Abilities), ExpiresAt: expires})
		return
	}
	rec, ok := r.Context().Value(tokenIdentityKey{}).(*store.APIToken)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	expires := ""
	if rec.ExpiresAt != nil {
		expires = rec.ExpiresAt.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, whoamiResponse{Kind: "token", Name: rec.Name, Abilities: nonNil(rec.Abilities), ExpiresAt: expires})
}
