package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

const passkeyBodyLimit = 1 << 20

func passkeyResourceFromLib(p authengine.Passkey) passkeyResource {
	return passkeyResource{ID: p.ID, Label: p.Label, Transports: p.Transports, CreatedAt: p.CreatedAt, LastUsedAt: p.LastUsedAt}
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
	writeJSON(w, http.StatusOK, passkeyRegistrationBeginResponse{SessionID: challenge, Options: options})
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
		l.rt.logger.Warn("api: passkey registration finish failed", slog.String("user_id", user.ID))
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
	writeJSON(w, http.StatusOK, out)
}

func (l *authLibMFA) deletePasskey(w http.ResponseWriter, r *http.Request) {
	userID, ok := l.rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
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
		rt.passkeyLogin.recordFailure(key)
		writeError(w, http.StatusUnauthorized, "could not verify passkey")
		return
	}
	if err != nil {
		rt.internalError(w, "api: passkey login finish: library finish failed", err)
		return
	}
	rt.passkeyLogin.recordSuccess(key)
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusUnauthorized, "invalid passkey login session")
			return
		}
		rt.internalError(w, "api: passkey login finish: load user failed", err)
		return
	}
	if err := l.seam.EstablishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: passkey login finish: establish session failed", err, slog.String("user_id", user.ID))
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}
