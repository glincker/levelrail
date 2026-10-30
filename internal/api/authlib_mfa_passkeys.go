package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-webauthn/webauthn/protocol"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const passkeyBodyLimit = 1 << 20

func passkeyResourceFromLib(p authengine.Passkey) passkeyResource {
	prov := authengine.ProviderForAAGUID(p.AAGUID)
	return passkeyResource{
		ID: p.ID, Label: p.Label, Transports: p.Transports, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt,
		ProviderKey: prov.Key, ProviderName: prov.Name, Synced: p.Synced, BackedUp: p.BackedUp, SignCount: p.SignCount,
	}
}

const codeLastSignInMethod = "last_sign_in_method"

// removalLocksOut reports whether deleting a passkey would leave the account with no way to sign in.
func removalLocksOut(passkeys int, hasPassword, hasTOTP bool) bool {
	return passkeys <= 1 && !hasPassword && !hasTOTP
}

// wouldLockOut checks the last-passkey guard for one user; an unreadable account fails open to the library delete.
func (l *authLibMFA) wouldLockOut(r *http.Request, userID, id string) bool {
	rows, err := l.mfa.PasskeyList(r.Context(), userID)
	if err != nil || len(rows) != 1 || rows[0].ID != id {
		return false
	}
	user, err := l.rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		return false
	}
	hasPassword := user.PasswordHash != nil && *user.PasswordHash != ""
	status, err := l.mfa.TOTPStatus(r.Context(), userID)
	return removalLocksOut(len(rows), hasPassword, err == nil && status.Enabled)
}

func (l *authLibMFA) passkeysReady(w http.ResponseWriter) bool {
	if l.mfa.PasskeysAvailable() {
		return true
	}
	writeError(w, http.StatusNotImplemented, msgPasskeysOff)
	return false
}

func (l *authLibMFA) beginRegistration(w http.ResponseWriter, r *http.Request) {
	if !l.passkeysReady(w) {
		return
	}
	user, ok := l.sessionUser(w, r, "api: passkey register begin")
	if !ok {
		return
	}
	options, challenge, err := l.mfa.PasskeyBeginRegistration(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: passkey register begin: library begin failed", user.ID, err)
		return
	}
	preferResidentKey(options)
	l.rt.recordAuthAudit(r, user.ID, accountEventPasskeyRegBegun, http.StatusOK, nil)
	writeJSON(w, http.StatusOK, passkeyRegistrationBeginResponse{SessionID: challenge, Options: options})
}

// preferResidentKey asks the authenticator for a discoverable credential when it can make one,
// without refusing devices that cannot (the library leaves the choice unset).
func preferResidentKey(c *protocol.CredentialCreation) {
	if c == nil {
		return
	}
	notRequired := false
	c.Response.AuthenticatorSelection.ResidentKey = protocol.ResidentKeyRequirementPreferred
	c.Response.AuthenticatorSelection.RequireResidentKey = &notRequired
}

func (l *authLibMFA) finishRegistration(w http.ResponseWriter, r *http.Request) {
	if !l.passkeysReady(w) {
		return
	}
	user, ok := l.sessionUser(w, r, "api: passkey register finish")
	if !ok {
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
	created, err := l.mfa.PasskeyFinishRegistration(r.Context(), user.ID, sessionID, label, http.MaxBytesReader(w, r.Body, passkeyBodyLimit))
	if errors.Is(err, authengine.ErrPasskeyRejected) {
		l.rt.logger.Warn("api: passkey registration finish failed", slog.String("user_id", user.ID), slog.String("origin", r.Header.Get("Origin")), slog.String("reason", err.Error()))
		writeError(w, http.StatusBadRequest, "could not verify passkey")
		return
	}
	if err != nil {
		l.fail(w, "api: passkey register finish: library finish failed", user.ID, err)
		return
	}
	writeJSON(w, http.StatusCreated, passkeyResourceFromLib(created))
}

func (l *authLibMFA) listPasskeys(w http.ResponseWriter, r *http.Request) {
	userID, ok := l.rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	rows, err := l.mfa.PasskeyList(r.Context(), userID)
	if errors.Is(err, authengine.ErrNotMapped) {
		rows, err = nil, nil
	}
	if err != nil {
		l.fail(w, "api: list passkeys failed", userID, err)
		return
	}
	out := make([]passkeyResource, 0, len(rows))
	for _, row := range rows {
		out = append(out, passkeyResourceFromLib(row))
	}
	writeJSON(w, http.StatusOK, pageQuery(w, r, out))
}

func (l *authLibMFA) deletePasskey(w http.ResponseWriter, r *http.Request) {
	userID, ok := l.rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	if l.wouldLockOut(r, userID, r.PathValue("id")) {
		writeCodedError(w, http.StatusConflict, codeLastSignInMethod, "this is your only way to sign in: add a password, two-factor or another passkey first")
		return
	}
	err := l.mfa.PasskeyDelete(r.Context(), userID, r.PathValue("id"))
	if errors.Is(err, authengine.ErrPasskeyNotFound) {
		writeError(w, http.StatusNotFound, "passkey not found")
		return
	}
	if err != nil {
		l.fail(w, "api: delete passkey failed", userID, err)
		return
	}
	l.rt.retireSignInRequests(r.Context(), r, userID, "passkey_removed")
	w.WriteHeader(http.StatusNoContent)
}

type passkeyRenameRequest struct {
	Label string `json:"label"`
}

// renamePasskey handles PATCH /api/v1/auth/passkeys/{id}, a route the built-in engine never had.
func (l *authLibMFA) renamePasskey(w http.ResponseWriter, r *http.Request) {
	userID, ok := l.rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return
	}
	var req passkeyRenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	id := r.PathValue("id")
	err := l.mfa.PasskeyRename(r.Context(), userID, id, req.Label)
	switch {
	case errors.Is(err, authengine.ErrPasskeyNotFound):
		writeError(w, http.StatusNotFound, "passkey not found")
		return
	case errors.Is(err, authengine.ErrBadPasskeyName):
		writeError(w, http.StatusBadRequest, "label must be 1 to 64 characters")
		return
	case err != nil:
		l.fail(w, "api: rename passkey failed", userID, err)
		return
	}
	rows, err := l.mfa.PasskeyList(r.Context(), userID)
	if err != nil {
		l.fail(w, "api: rename passkey: reload failed", userID, err)
		return
	}
	for _, row := range rows {
		if row.ID == id {
			writeJSON(w, http.StatusOK, passkeyResourceFromLib(row))
			return
		}
	}
	writeError(w, http.StatusNotFound, "passkey not found")
}

func (l *authLibMFA) beginLogin(w http.ResponseWriter, r *http.Request) {
	rt := l.rt
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	if !l.passkeysReady(w) {
		return
	}
	var req passkeyLoginBeginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Discoverable && req.Username == "" {
		l.beginDiscoverableLogin(w, r)
		return
	}
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
	existing, err := l.mfa.PasskeyList(r.Context(), user.ID)
	if err != nil && !errors.Is(err, authengine.ErrNotMapped) {
		l.fail(w, "api: passkey login begin: list credentials failed", user.ID, err)
		return
	}
	if len(existing) == 0 {
		rt.passkeyLogin.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "no passkey available for this account")
		return
	}
	options, challenge, err := l.mfa.PasskeyBeginLogin(r.Context())
	if err != nil {
		l.fail(w, "api: passkey login begin: library begin failed", user.ID, err)
		return
	}
	writeJSON(w, http.StatusOK, passkeyLoginBeginResponse{SessionID: challenge, Options: options})
}

// beginDiscoverableLogin issues a challenge without naming an account; the authenticator
// picks the credential, so nothing here depends on which users exist.
func (l *authLibMFA) beginDiscoverableLogin(w http.ResponseWriter, r *http.Request) {
	rt := l.rt
	key := clientIP(r) + "|passkey-discover"
	if allowed, retryAfter := rt.passkeyLogin.allow(key); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	options, challenge, err := l.mfa.PasskeyBeginLogin(r.Context())
	if err != nil {
		rt.internalError(w, "api: passkey discoverable login begin: library begin failed", err)
		return
	}
	writeJSON(w, http.StatusOK, passkeyLoginBeginResponse{SessionID: challenge, Options: options})
}

const msgPasskeyUnverified = "could not verify passkey"

// denyPasskeyLogin is the single failure answer of the finish step, so a rejected assertion,
// an unknown credential owner and a disabled account are indistinguishable to the caller.
func (l *authLibMFA) denyPasskeyLogin(w http.ResponseWriter, key string) {
	l.rt.passkeyLogin.recordFailure(key)
	writeError(w, http.StatusUnauthorized, msgPasskeyUnverified)
}

func (l *authLibMFA) finishLogin(w http.ResponseWriter, r *http.Request) {
	rt := l.rt
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	if !l.passkeysReady(w) {
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "session_id is required")
		return
	}
	key := clientIP(r) + "|passkey-finish"
	if allowed, retryAfter := rt.passkeyLogin.allow(key); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many attempts, try again later")
		return
	}
	userID, err := l.mfa.PasskeyFinishLogin(r.Context(), sessionID, http.MaxBytesReader(w, r.Body, passkeyBodyLimit), r.UserAgent(), clientIP(r))
	if errors.Is(err, authengine.ErrPasskeyRejected) || errors.Is(err, authengine.ErrNotMapped) {
		rt.logger.Warn("api: passkey login finish failed", slog.String("remote", clientIP(r)))
		l.denyPasskeyLogin(w, key)
		return
	}
	if err != nil {
		rt.internalError(w, "api: passkey login finish: library finish failed", err)
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			l.denyPasskeyLogin(w, key)
			return
		}
		rt.internalError(w, "api: passkey login finish: load user failed", err)
		return
	}
	if rt.accountDisabled(r.Context(), user.ID) {
		l.denyPasskeyLogin(w, key)
		return
	}
	rt.passkeyLogin.recordSuccess(key)
	if rt.ssoBlocksLocalSignIn(r.Context(), user.ID) {
		writeError(w, http.StatusForbidden, ssoRequiredMessage)
		return
	}
	if rt.pauseForDeviceApproval(w, r, *user, "", signInMethodPasskey) {
		return
	}
	if err := l.seam.EstablishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: passkey login finish: establish session failed", err, slog.String("user_id", user.ID))
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}
