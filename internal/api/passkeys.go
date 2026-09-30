package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/passkey"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// passkeyCeremonyTTL bounds how long a registration or login challenge
// stays valid: short enough that an abandoned ceremony can't be
// replayed hours later, long enough for a real authenticator prompt
// (a hardware key, a phone hand-off) to complete.
const passkeyCeremonyTTL = 5 * time.Minute

// passkeyCeremonyEntry is one in-flight WebAuthn challenge.
type passkeyCeremonyEntry struct {
	session   webauthn.SessionData
	expiresAt time.Time
}

// passkeyCeremonyStore is a short-TTL, single-use sibling of
// mfaPendingStore: unlike a mistyped TOTP code, a WebAuthn response is
// only ever generated once per challenge, so consume pops the entry on
// first lookup regardless of whether the ceremony that follows
// succeeds. That pop is this package's replay guard, on top of the
// library's own challenge and (with Timeouts.Enforce, see
// internal/passkey.NewRelyingParty) expiry checks.
type passkeyCeremonyStore struct {
	mu      sync.Mutex
	entries map[string]passkeyCeremonyEntry
}

func newPasskeyCeremonyStore() *passkeyCeremonyStore {
	return &passkeyCeremonyStore{entries: make(map[string]passkeyCeremonyEntry)}
}

func (s *passkeyCeremonyStore) create(session webauthn.SessionData) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate passkey ceremony token: %w", err)
	}
	s.mu.Lock()
	s.entries[token] = passkeyCeremonyEntry{session: session, expiresAt: time.Now().Add(passkeyCeremonyTTL)}
	s.mu.Unlock()
	return token, nil
}

// consume pops token's session data, if any, and reports whether it was
// found and still live. A missing or expired token both report false;
// callers give the same generic error either way.
func (s *passkeyCeremonyStore) consume(token string) (webauthn.SessionData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[token]
	if !ok {
		return webauthn.SessionData{}, false
	}
	delete(s.entries, token)
	if time.Now().After(e.expiresAt) {
		return webauthn.SessionData{}, false
	}
	return e.session, true
}

// passkeyRelyingParty builds a *webauthn.WebAuthn bound to r's own host
// and scheme. There is no single fixed domain to configure at startup
// on a self-hosted platform, so the relying party ID is whatever domain
// the browser actually reached this control plane on, derived fresh per
// request the same way oauthRedirectURL derives an OAuth callback URL.
// Deliberately independent of rt.publicHost (advertisedHost's own
// source of truth for a DNS pointer hint): a WebAuthn ceremony must be
// bound to the origin the browser itself resolved, which an
// administratively configured value could drift from.
func (rt *Router) passkeyRelyingParty(r *http.Request) (*webauthn.WebAuthn, error) {
	rpID := r.Host
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		rpID = h
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return passkey.NewRelyingParty(rpID, rt.brand.Name, scheme+"://"+r.Host)
}

// passkeyWebAuthnUser loads user's stored credentials and adapts both
// into the shape internal/passkey's ceremonies need. A row that fails
// to decode (should never happen; only this package ever writes one) is
// skipped rather than failing the whole ceremony, logged so it's not
// silently lost.
func (rt *Router) passkeyWebAuthnUser(r *http.Request, user store.User) (passkey.User, []store.PasskeyCredential, error) {
	rows, err := rt.passkeys.ListPasskeyCredentialsForUser(r.Context(), user.ID)
	if err != nil {
		return passkey.User{}, nil, fmt.Errorf("list passkey credentials: %w", err)
	}
	creds := make([]passkey.Credential, 0, len(rows))
	for _, row := range rows {
		c, derr := fromStorePasskeyCredential(row)
		if derr != nil {
			rt.logger.Warn("api: passkey: decode stored credential failed", slog.String("error", derr.Error()), slog.String("credential_row_id", row.ID))
			continue
		}
		creds = append(creds, c)
	}
	return passkey.User{ID: user.ID, Name: user.Email, DisplayName: user.DisplayName, Credentials: creds}, rows, nil
}

func toStorePasskeyCredential(c passkey.Credential) store.PasskeyCredential {
	return store.PasskeyCredential{
		ID:           c.ID,
		UserID:       c.UserID,
		CredentialID: base64.RawURLEncoding.EncodeToString(c.CredentialID),
		PublicKey:    c.PublicKey,
		SignCount:    c.SignCount,
		AAGUID:       base64.RawURLEncoding.EncodeToString(c.AAGUID),
		Transports:   c.Transports,
		Label:        c.Label,
		CreatedAt:    c.CreatedAt,
		LastUsedAt:   c.LastUsedAt,
	}
}

func fromStorePasskeyCredential(c store.PasskeyCredential) (passkey.Credential, error) {
	credID, err := base64.RawURLEncoding.DecodeString(c.CredentialID)
	if err != nil {
		return passkey.Credential{}, fmt.Errorf("decode credential_id: %w", err)
	}
	var aaguid []byte
	if c.AAGUID != "" {
		aaguid, err = base64.RawURLEncoding.DecodeString(c.AAGUID)
		if err != nil {
			return passkey.Credential{}, fmt.Errorf("decode aaguid: %w", err)
		}
	}
	return passkey.Credential{
		ID:           c.ID,
		UserID:       c.UserID,
		CredentialID: credID,
		PublicKey:    c.PublicKey,
		SignCount:    c.SignCount,
		AAGUID:       aaguid,
		Transports:   c.Transports,
		Label:        c.Label,
		CreatedAt:    c.CreatedAt,
		LastUsedAt:   c.LastUsedAt,
	}, nil
}

type passkeyRegistrationBeginResponse struct {
	SessionID string                       `json:"session_id"`
	Options   *protocol.CredentialCreation `json:"options"`
}

// handleBeginPasskeyRegistration handles
// POST /api/v1/auth/passkeys/register/begin: requireAuth-gated, since
// adding a passkey to an account only ever happens from inside an
// already-authenticated session.
func (rt *Router) handleBeginPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: passkey register begin: load user failed", err, slog.String("user_id", userID))
		return
	}

	wu, existing, err := rt.passkeyWebAuthnUser(r, *user)
	if err != nil {
		rt.internalError(w, "api: passkey register begin: load credentials failed", err, slog.String("user_id", userID))
		return
	}

	rp, err := rt.passkeyRelyingParty(r)
	if err != nil {
		rt.internalError(w, "api: passkey register begin: build relying party failed", err)
		return
	}

	exclude := make([]protocol.CredentialDescriptor, 0, len(existing))
	for _, c := range wu.Credentials {
		exclude = append(exclude, protocol.CredentialDescriptor{Type: protocol.PublicKeyCredentialType, CredentialID: c.CredentialID})
	}

	creation, session, err := rp.BeginRegistration(wu, webauthn.WithExclusions(exclude))
	if err != nil {
		rt.internalError(w, "api: passkey register begin: begin registration failed", err, slog.String("user_id", userID))
		return
	}
	token, err := rt.passkeyRegSessions.create(*session)
	if err != nil {
		rt.internalError(w, "api: passkey register begin: create ceremony session failed", err)
		return
	}
	writeJSON(w, http.StatusOK, passkeyRegistrationBeginResponse{SessionID: token, Options: creation})
}

type passkeyResource struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Transports []string   `json:"transports,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

func passkeyResourceFromStore(c store.PasskeyCredential) passkeyResource {
	return passkeyResource{
		ID:         c.ID,
		Label:      c.Label,
		Transports: c.Transports,
		CreatedAt:  c.CreatedAt,
		LastUsedAt: c.LastUsedAt,
	}
}

// handleFinishPasskeyRegistration handles
// POST /api/v1/auth/passkeys/register/finish?session_id=...&label=...:
// the request body is the raw navigator.credentials.create() response
// (PublicKeyCredential.toJSON()), consumed whole by the library's own
// parser, so session_id and label travel as query parameters instead of
// alongside it in the body.
func (rt *Router) handleFinishPasskeyRegistration(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	label := strings.TrimSpace(r.URL.Query().Get("label"))
	if label == "" {
		label = "Passkey"
	}

	session, ok := rt.passkeyRegSessions.consume(sessionID)
	if !ok {
		writeError(w, http.StatusUnauthorized, "passkey registration session expired or already used, start again")
		return
	}
	if string(session.UserID) != userID {
		writeError(w, http.StatusUnauthorized, "passkey registration session does not belong to this account")
		return
	}

	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: passkey register finish: load user failed", err, slog.String("user_id", userID))
		return
	}
	wu, _, err := rt.passkeyWebAuthnUser(r, *user)
	if err != nil {
		rt.internalError(w, "api: passkey register finish: load credentials failed", err, slog.String("user_id", userID))
		return
	}

	rp, err := rt.passkeyRelyingParty(r)
	if err != nil {
		rt.internalError(w, "api: passkey register finish: build relying party failed", err)
		return
	}

	cred, err := rp.FinishRegistration(wu, session, r)
	if err != nil {
		rt.logger.Warn("api: passkey registration finish failed", slog.String("error", err.Error()), slog.String("user_id", userID))
		writeError(w, http.StatusBadRequest, "could not verify passkey")
		return
	}

	id, err := randomOpaqueID("pk_")
	if err != nil {
		rt.internalError(w, "api: passkey register finish: generate id failed", err, slog.String("user_id", userID))
		return
	}
	row := toStorePasskeyCredential(passkey.FromWebAuthnCredential(id, userID, label, cred, time.Now()))
	if err := rt.passkeys.SavePasskeyCredential(r.Context(), row); err != nil {
		if errors.Is(err, store.ErrPasskeyCredentialAlreadyRegistered) {
			writeError(w, http.StatusConflict, "this passkey is already registered")
			return
		}
		rt.internalError(w, "api: passkey register finish: save credential failed", err, slog.String("user_id", userID))
		return
	}
	writeJSON(w, http.StatusCreated, passkeyResourceFromStore(row))
}

// handleListPasskeys handles GET /api/v1/auth/passkeys: the caller's
// own registered credentials, never the public key or credential ID
// bytes themselves, only what the settings page needs to label and
// revoke one.
func (rt *Router) handleListPasskeys(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	rows, err := rt.passkeys.ListPasskeyCredentialsForUser(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: list passkeys failed", err, slog.String("user_id", userID))
		return
	}
	out := make([]passkeyResource, 0, len(rows))
	for _, row := range rows {
		out = append(out, passkeyResourceFromStore(row))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeletePasskey handles DELETE /api/v1/auth/passkeys/{id}, scoped
// to the caller's own account: store.DeletePasskeyCredential's
// (id, userID) WHERE clause is what actually enforces that, this is
// just the 404 translation.
func (rt *Router) handleDeletePasskey(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	id := r.PathValue("id")
	if err := rt.passkeys.DeletePasskeyCredential(r.Context(), id, userID); err != nil {
		if errors.Is(err, store.ErrPasskeyCredentialNotFound) {
			writeError(w, http.StatusNotFound, "passkey not found")
			return
		}
		rt.internalError(w, "api: delete passkey failed", err, slog.String("user_id", userID), slog.String("passkey_id", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type passkeyLoginBeginRequest struct {
	Username string `json:"username"`
}

type passkeyLoginBeginResponse struct {
	SessionID string                        `json:"session_id"`
	Options   *protocol.CredentialAssertion `json:"options"`
}

// handleBeginPasskeyLogin handles POST /api/v1/auth/passkey-login/begin.
// Necessarily public and username-first, not usernameless/discoverable:
// this codebase's login form already collects a username before any
// second factor, and reusing that shape keeps the ceremony's identity
// binding explicit rather than relying on resident-key discovery. The
// tradeoff is that an unknown username and a known one with no passkey
// registered are distinguishable from one with a real passkey (the
// caller gets a genuine assertion challenge only in the latter case),
// the same "learn less than the whole picture" tradeoff most
// username-first passkey flows accept; a fully enumeration-resistant
// flow needs usernameless/discoverable credentials instead.
func (rt *Router) handleBeginPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	var req passkeyLoginBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}

	key := loginLimiterKey(r, req.Username)
	if allowed, retryAfter := rt.passkeyLogin.allow(key); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	user, err := rt.auth.GetUserByEmail(r.Context(), req.Username)
	if err != nil {
		if !errors.Is(err, store.ErrUserNotFound) {
			rt.internalError(w, "api: passkey login begin: load user failed", err)
			return
		}
		rt.passkeyLogin.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "no passkey available for this account")
		return
	}

	wu, existing, err := rt.passkeyWebAuthnUser(r, *user)
	if err != nil {
		rt.internalError(w, "api: passkey login begin: load credentials failed", err, slog.String("user_id", user.ID))
		return
	}
	if len(existing) == 0 {
		rt.passkeyLogin.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "no passkey available for this account")
		return
	}

	rp, err := rt.passkeyRelyingParty(r)
	if err != nil {
		rt.internalError(w, "api: passkey login begin: build relying party failed", err)
		return
	}

	assertion, session, err := rp.BeginLogin(wu)
	if err != nil {
		rt.internalError(w, "api: passkey login begin: begin login failed", err, slog.String("user_id", user.ID))
		return
	}
	token, err := rt.passkeyLoginSessions.create(*session)
	if err != nil {
		rt.internalError(w, "api: passkey login begin: create ceremony session failed", err)
		return
	}
	writeJSON(w, http.StatusOK, passkeyLoginBeginResponse{SessionID: token, Options: assertion})
}

// handleFinishPasskeyLogin handles
// POST /api/v1/auth/passkey-login/finish?session_id=...: the assertion
// counterpart of handleFinishPasskeyRegistration, same query-parameter
// shape for the same reason (the body is the raw assertion response).
// Necessarily public, like handleVerifyTwoFactor. Does not additionally
// require a TOTP code even when the account has 2FA enabled: this
// mirrors the OAuth sign-in callback (oauth.go), which also completes a
// session on its own external verification without a second factor, and
// a platform authenticator's own verification (biometric or PIN)
// already establishes both possession and, typically, user verification.
func (rt *Router) handleFinishPasskeyLogin(w http.ResponseWriter, r *http.Request) {
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}

	session, ok := rt.passkeyLoginSessions.consume(sessionID)
	if !ok {
		rt.passkeyLogin.recordFailure(clientIP(r) + "|passkey-finish")
		writeError(w, http.StatusUnauthorized, "passkey login session expired or already used, start again")
		return
	}

	user, err := rt.auth.GetUserByID(r.Context(), string(session.UserID))
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid passkey login session")
			return
		}
		rt.internalError(w, "api: passkey login finish: load user failed", err)
		return
	}
	key := loginLimiterKey(r, user.Email)
	if allowed, retryAfter := rt.passkeyLogin.allow(key); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}

	wu, storeCreds, err := rt.passkeyWebAuthnUser(r, *user)
	if err != nil {
		rt.internalError(w, "api: passkey login finish: load credentials failed", err, slog.String("user_id", user.ID))
		return
	}

	rp, err := rt.passkeyRelyingParty(r)
	if err != nil {
		rt.internalError(w, "api: passkey login finish: build relying party failed", err)
		return
	}

	cred, err := rp.FinishLogin(wu, session, r)
	if err != nil {
		rt.logger.Warn("api: passkey login finish failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
		rt.passkeyLogin.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "could not verify passkey")
		return
	}
	rt.passkeyLogin.recordSuccess(key)

	for _, row := range storeCreds {
		if row.CredentialID != base64.RawURLEncoding.EncodeToString(cred.ID) {
			continue
		}
		if uerr := rt.passkeys.UpdatePasskeySignCountByCredentialID(r.Context(), row.CredentialID, cred.Authenticator.SignCount, time.Now()); uerr != nil {
			// Best-effort: the login itself already succeeded against a
			// verified signature, a stale sign counter or last_used_at is
			// an observability nicety, not a reason to fail the sign-in.
			rt.logger.Warn("api: passkey login finish: update sign count failed", slog.String("error", uerr.Error()), slog.String("user_id", user.ID))
		}
		break
	}

	if err := rt.establishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: passkey login finish: establish session failed", err, slog.String("user_id", user.ID))
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}
