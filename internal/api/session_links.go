package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// sessionLinkTokenTTL bounds how long a minted session link can be
// redeemed: short, since possessing it is root-equivalent (see
// handleMintSessionLink's own doc comment). Deliberately not the
// resulting session's own lifetime, which still uses rt.sessions.ttl
// like any ordinary login.
const sessionLinkTokenTTL = 2 * time.Minute

const sessionLinkTokenIDPrefix = "sl_"

func randomSessionLinkTokenID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate session link token id: %w", err)
	}
	return sessionLinkTokenIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

type mintSessionLinkResponse struct {
	Token string `json:"token"`
	URL   string `json:"url"`
}

// resolveMintingPrincipal resolves the caller a session link is minted
// on behalf of: the same session-or-token resolution callerPrincipal
// uses, but also returning a human-readable display name, which
// callerPrincipal's own callers never needed. Only ever called from
// behind requireAbility(AbilityRoot, ...), so a resolution failure here
// means a lookup race (e.g. the user/token was deleted between the
// ability check and this handler running), not an auth gap.
func (rt *Router) resolveMintingPrincipal(r *http.Request) (principalType, principalID, displayName string, abilities []string, err error) {
	if userID, ok := rt.currentSessionUserID(r); ok {
		user, err := rt.auth.GetUserByID(r.Context(), userID)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("api: load minting user %q: %w", userID, err)
		}
		return store.PrincipalTypeUser, userID, user.DisplayName, user.Abilities, nil
	}
	if token, ok := bearerToken(r); ok {
		rec, err := rt.tokens.GetAPITokenByHash(r.Context(), hashToken(token))
		if err != nil {
			return "", "", "", nil, fmt.Errorf("api: load minting token: %w", err)
		}
		name := rec.Name
		if name == "" {
			name = rec.ID
		}
		return store.PrincipalTypeToken, rec.ID, name, rec.Abilities, nil
	}
	return "", "", "", nil, errors.New("api: no authenticated principal on request")
}

// handleMintSessionLink handles POST /api/v1/auth/session-links: mints a
// short-lived, single-use token that GET
// .../session-links/{token}/consume exchanges for a real session
// carrying the minting caller's own abilities, no password involved.
// Gated AbilityRoot (router.go's route registration): minting one is
// root-equivalent, since redeeming it establishes a session with
// whatever abilities the minting caller itself held.
func (rt *Router) handleMintSessionLink(w http.ResponseWriter, r *http.Request) {
	principalType, principalID, displayName, abilities, err := rt.resolveMintingPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: mint session link: resolve principal failed", err)
		return
	}

	if rt.libSessions != nil && principalType == store.PrincipalTypeUser {
		rt.mintLibSessionLink(w, r, principalID)
		return
	}

	plaintext, err := randomToken()
	if err != nil {
		rt.internalError(w, "api: mint session link: generate token failed", err)
		return
	}
	id, err := randomSessionLinkTokenID()
	if err != nil {
		rt.internalError(w, "api: mint session link: generate id failed", err)
		return
	}

	now := time.Now().UTC()
	rec := store.SessionLinkToken{
		ID:            id,
		PrincipalType: principalType,
		PrincipalID:   principalID,
		Abilities:     abilities,
		DisplayName:   displayName,
		TokenHash:     hashToken(plaintext),
		CreatedAt:     now,
		ExpiresAt:     now.Add(sessionLinkTokenTTL),
	}
	if err := rt.sessionLinkTokens.SaveSessionLinkToken(r.Context(), rec); err != nil {
		rt.internalError(w, "api: mint session link: save failed", err)
		return
	}

	writeJSON(w, http.StatusCreated, mintSessionLinkResponse{
		Token: plaintext,
		URL:   rt.sessionLinkURL(r, plaintext),
	})
}

// sessionLinkURL builds the link handleMintSessionLink hands back:
// absolute against the primary domain when controlPlaneBaseURL can
// resolve one, the same bare-path fallback passwordResetURL uses
// otherwise (e.g. local/dev use, exactly where this feature's own
// browser-automation use case is most likely to run with no domain
// configured at all).
func (rt *Router) sessionLinkURL(r *http.Request, token string) string {
	const path = "/login?session_link="
	base, err := rt.controlPlaneBaseURL(r.Context())
	if err != nil {
		return path + token
	}
	return base + path + token
}

// errInvalidOrExpiredSessionLink covers every way a session-link token
// can fail (not found, expired, used): distinguishing them isn't worth
// the complexity, matching errInvalidOrExpiredResetToken's own reasoning.
var errInvalidOrExpiredSessionLink = errors.New("invalid or expired session link")

// handleConsumeSessionLink handles GET
// /api/v1/auth/session-links/{token}/consume: unauthenticated by
// necessity, gated by possession of the token itself, the same shape
// handleResetPassword uses. A user-minted link establishes an ordinary
// session for that user; a token-minted link establishes a pinned
// session carrying the mint-time snapshot (see sessionStore.createPinned),
// since an API token has no user row to attach a normal session to.
func (rt *Router) handleConsumeSessionLink(w http.ResponseWriter, r *http.Request) {
	if !rt.allowTokenRedeem(w, r, "session-link-consume") {
		return
	}
	token := r.PathValue("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	}

	rec, err := rt.sessionLinkTokens.GetSessionLinkTokenByHash(r.Context(), hashToken(token))
	if errors.Is(err, store.ErrSessionLinkTokenNotFound) {
		if rt.libSessions != nil {
			rt.consumeLibSessionLink(w, r, token)
			return
		}
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: consume session link: load token failed", err)
		return
	}
	if rec.UsedAt != nil || time.Now().After(rec.ExpiresAt) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	}

	// Claimed before a session is established, not after: the single
	// atomic point two concurrent requests holding the same token race
	// on, so at most one can proceed past it (see ClaimSessionLinkToken's
	// own doc comment).
	if err := rt.sessionLinkTokens.ClaimSessionLinkToken(r.Context(), rec.ID); errors.Is(err, store.ErrSessionLinkTokenAlreadyUsed) {
		writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
		return
	} else if err != nil {
		rt.internalError(w, "api: consume session link: claim failed", err)
		return
	}

	if rec.PrincipalType == store.PrincipalTypeUser {
		user, err := rt.auth.GetUserByID(r.Context(), rec.PrincipalID)
		if err != nil {
			rt.logger.Warn("api: consume session link: minting user no longer exists", slog.String("error", err.Error()), slog.String("user_id", rec.PrincipalID))
			writeError(w, http.StatusBadRequest, errInvalidOrExpiredSessionLink.Error())
			return
		}
		if err := rt.establishSession(w, r, *user); err != nil {
			rt.internalError(w, "api: consume session link: establish session failed", err)
			return
		}
		writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
		return
	}

	sessionToken, err := rt.sessions.createPinned(rec.PrincipalID, rec.Abilities, rec.DisplayName)
	if err != nil {
		rt.internalError(w, "api: consume session link: create pinned session failed", err)
		return
	}
	setSessionCookie(w, r, sessionToken, time.Now().Add(rt.sessions.ttl))
	// Email has no real meaning for a token principal (there is no user
	// row), but the frontend's login response always reads this field as
	// "username" to display and locally record who's signed in, so it's
	// set to the same display name rather than left blank.
	writeJSON(w, http.StatusOK, loginResponse{Email: rec.DisplayName, DisplayName: rec.DisplayName})
}
