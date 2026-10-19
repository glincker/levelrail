package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type oauthUnlinker interface {
	UnlinkOAuthIdentity(ctx context.Context, legacyUserID, provider string) error
}

// signInMethodsLeft counts the ways this account could still sign in once
// except is removed: a password, passkeys, and the other linked providers.
func (rt *Router) signInMethodsLeft(ctx context.Context, userID, except string) (int, error) {
	n := 0
	if rt.userHasPassword(ctx, userID) {
		n++
	}
	if rt.userHasPasskeys(ctx, userID) {
		n++
	}
	if rt.oauthIdentities == nil {
		return n, nil
	}
	ids, err := rt.oauthIdentities.ListOAuthIdentitiesForUser(ctx, userID)
	if err != nil {
		return 0, err
	}
	for _, i := range ids {
		if i.Provider != except {
			n++
		}
	}
	return n, nil
}

// handleUnlinkProvider handles DELETE /api/v1/auth/link/{provider}: the caller
// drops one linked sign-in provider, never its last way in.
func (rt *Router) handleUnlinkProvider(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWriteSensitive)
	if !ok {
		return
	}
	provider := r.PathValue("provider")
	if !isValidOAuthProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown oauth provider")
		return
	}
	db := rt.accountDB()
	if db == nil || rt.oauthIdentities == nil {
		writeError(w, http.StatusNotImplemented, "linked providers are not available on this server")
		return
	}
	accountMutationMu.Lock()
	defer accountMutationMu.Unlock()
	left, err := rt.signInMethodsLeft(r.Context(), userID, provider)
	if err != nil {
		rt.internalError(w, "api: unlink provider: count sign-in methods failed", err)
		return
	}
	if left == 0 {
		writeCodedError(w, http.StatusConflict, "last_sign_in_method", "this is your only way to sign in: set a password or add a passkey first")
		return
	}
	if u, ok := rt.authLibOAuth.(oauthUnlinker); ok && rt.authLibOAuthActive() {
		if err := u.UnlinkOAuthIdentity(r.Context(), userID, provider); err != nil {
			rt.internalError(w, "api: unlink provider in auth library failed", err, slog.String("provider", provider))
			return
		}
	}
	if err := db.DeleteOAuthIdentityForUser(r.Context(), userID, provider); err != nil {
		if errors.Is(err, store.ErrOAuthIdentityNotFound) {
			writeError(w, http.StatusNotFound, "this provider is not linked")
			return
		}
		rt.internalError(w, "api: unlink provider failed", err, slog.String("provider", provider))
		return
	}
	rt.recordAccountAction(r, userID, viaSession, accountEventProviderUnlinked, http.StatusNoContent)
	w.WriteHeader(http.StatusNoContent)
}
