package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/totp"
)

// mfaLoginSeam is everything the library MFA handlers need from the sign-in
// path, so the session area can swap its own implementation in.
type mfaLoginSeam interface {
	PendingUser(token string) (userID string, ok bool)
	RevokePending(token string)
	EstablishSession(w http.ResponseWriter, r *http.Request, user store.User) error
}

type legacyLoginSeam struct{ rt *Router }

func (s legacyLoginSeam) PendingUser(token string) (string, bool) {
	return s.rt.mfaPending.lookup(token)
}
func (s legacyLoginSeam) RevokePending(token string) { s.rt.mfaPending.revoke(token) }
func (s legacyLoginSeam) EstablishSession(w http.ResponseWriter, r *http.Request, user store.User) error {
	return s.rt.establishSession(w, r, user)
}

// authLibMFA serves the legacy TOTP and passkey routes through the library.
type authLibMFA struct {
	rt   *Router
	mfa  *authengine.MFA
	seam mfaLoginSeam
}

// WithAuthEngineMFA serves TOTP and passkeys through the library on the existing
// routes. A nil service is a no-op, which keeps the built-in handlers.
func WithAuthEngineMFA(m *authengine.MFA) Option {
	return func(rt *Router) {
		if m == nil {
			return
		}
		rt.mfaLib = &authLibMFA{rt: rt, mfa: m, seam: legacyLoginSeam{rt: rt}}
	}
}

const (
	msgAuthRequired    = "authentication required"
	msgInternalError   = "internal error"
	msgInvalidBody     = "invalid request body"
	msgInvalidCode     = "invalid code"
	msgTwoFactorOff    = "two-factor authentication requires a master key to be configured on this control plane"
	msgPasskeysOff     = "passkeys are not available: set the dashboard URL or the WebAuthn relying party override"
	msgAccountNotReady = "this account is not synced to the auth engine yet, run the backfill"
)

// fail writes the legacy-shaped response for a service error.
func (l *authLibMFA) fail(w http.ResponseWriter, ctx string, userID string, err error) {
	var locked *authengine.LockedError
	switch {
	case errors.As(err, &locked):
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", locked.RetryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
	case errors.Is(err, authengine.ErrNotMapped):
		writeError(w, http.StatusConflict, msgAccountNotReady)
	default:
		l.rt.internalError(w, ctx, err, slog.String("user_id", userID))
	}
}

func (l *authLibMFA) sessionUser(w http.ResponseWriter, r *http.Request, ctx string) (*store.User, bool) {
	userID, ok := l.rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, msgAuthRequired)
		return nil, false
	}
	user, err := l.rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		l.rt.internalError(w, ctx+": load user failed", err, slog.String("user_id", userID))
		return nil, false
	}
	return user, true
}

func (l *authLibMFA) status(w http.ResponseWriter, r *http.Request) {
	user, ok := l.sessionUser(w, r, "api: 2fa status")
	if !ok {
		return
	}
	resp := twoFactorStatusResponse{}
	if l.mfa.TOTPAvailable() {
		st, err := l.mfa.TOTPStatus(r.Context(), user.ID)
		if err != nil && !errors.Is(err, authengine.ErrNotMapped) {
			l.fail(w, "api: 2fa status: library status failed", user.ID, err)
			return
		}
		resp = twoFactorStatusResponse{
			Enabled:                       st.Enabled,
			RecoveryCodesRemaining:        st.RecoveryCodesRemaining,
			RecoveryCodesNeedRegeneration: st.NeedsRegeneration,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (l *authLibMFA) setup(w http.ResponseWriter, r *http.Request) {
	if !l.mfa.TOTPAvailable() {
		writeError(w, http.StatusNotImplemented, msgTwoFactorOff)
		return
	}
	user, ok := l.sessionUser(w, r, "api: 2fa setup")
	if !ok {
		return
	}
	st, err := l.mfa.TOTPStatus(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: 2fa setup: library status failed", user.ID, err)
		return
	}
	if st.Enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
		return
	}
	enroll, err := l.mfa.TOTPBegin(r.Context(), user.ID, user.Email)
	if err != nil {
		l.fail(w, "api: 2fa setup: begin enrollment failed", user.ID, err)
		return
	}
	if l.rt.twoFactorSecrets != nil {
		if err := l.rt.twoFactorSecrets.SetValue(r.Context(), store.UserTOTPSecretsKey(user.ID), totpSecretEnvKey, enroll.Secret); err != nil {
			l.rt.logger.Warn("api: 2fa setup: mirror secret to built-in store failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
		}
	}
	writeJSON(w, http.StatusOK, twoFactorSetupResponse{Secret: enroll.Secret, ProvisioningURI: enroll.ProvisioningURI})
}

func (l *authLibMFA) confirm(w http.ResponseWriter, r *http.Request) {
	if !l.mfa.TOTPAvailable() {
		writeError(w, http.StatusNotImplemented, msgTwoFactorOff)
		return
	}
	user, ok := l.sessionUser(w, r, "api: 2fa confirm")
	if !ok {
		return
	}
	st, err := l.mfa.TOTPStatus(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: 2fa confirm: library status failed", user.ID, err)
		return
	}
	if st.Enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
		return
	}
	var req twoFactorCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	codes, err := l.mfa.TOTPFinish(r.Context(), user.ID, req.Code)
	switch {
	case errors.Is(err, authengine.ErrNoEnrollment):
		writeError(w, http.StatusBadRequest, "call setup before confirm")
		return
	case errors.Is(err, authengine.ErrInvalidCode):
		writeError(w, http.StatusBadRequest, msgInvalidCode)
		return
	case errors.Is(err, authengine.ErrAlreadyEnrolled):
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
		return
	case err != nil:
		l.fail(w, "api: 2fa confirm: finish enrollment failed", user.ID, err)
		return
	}
	if err := l.rt.auth.EnableUserTOTP(r.Context(), user.ID, time.Now()); err != nil {
		l.rt.internalError(w, "api: 2fa confirm: enable failed", err, slog.String("user_id", user.ID))
		return
	}
	l.mirrorRecoveryCodes(r, user.ID, codes)
	writeJSON(w, http.StatusOK, twoFactorRecoveryCodesResponse{RecoveryCodes: codes})
}

// mirrorRecoveryCodes keeps the built-in recovery-code table in step so
// switching the engine back to legacy leaves enrolled users able to sign in.
func (l *authLibMFA) mirrorRecoveryCodes(r *http.Request, userID string, codes []string) {
	hashes := make([]string, 0, len(codes))
	for _, c := range codes {
		hashes = append(hashes, hashToken(totp.NormalizeRecoveryCode(c)))
	}
	if err := l.rt.recoveryCodes.ReplaceUserRecoveryCodes(r.Context(), userID, hashes); err != nil {
		l.rt.logger.Warn("api: mirror recovery codes to built-in store failed", slog.String("error", err.Error()), slog.String("user_id", userID))
	}
}

// checkCode verifies a TOTP or recovery code and writes the failure response itself.
func (l *authLibMFA) checkCode(w http.ResponseWriter, r *http.Request, userID, code, recovery string) bool {
	err := l.mfa.CheckSecondFactor(r.Context(), userID, code, recovery, r.UserAgent(), clientIP(r))
	switch {
	case err == nil:
		return true
	case errors.Is(err, authengine.ErrInvalidCode):
		writeError(w, http.StatusBadRequest, msgInvalidCode)
	default:
		l.fail(w, "api: 2fa: verify code failed", userID, err)
	}
	return false
}

func (l *authLibMFA) disable(w http.ResponseWriter, r *http.Request) {
	user, ok := l.sessionUser(w, r, "api: 2fa disable")
	if !ok {
		return
	}
	st, err := l.mfa.TOTPStatus(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: 2fa disable: library status failed", user.ID, err)
		return
	}
	if !st.Enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is not enabled")
		return
	}
	var req twoFactorDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.Code == "" && req.RecoveryCode == "" {
		writeError(w, http.StatusBadRequest, msgInvalidCode)
		return
	}
	if !l.checkCode(w, r, user.ID, req.Code, req.RecoveryCode) {
		return
	}
	if err := l.mfa.TOTPDisable(r.Context(), user.ID); err != nil {
		l.fail(w, "api: 2fa disable: library delete failed", user.ID, err)
		return
	}
	if err := l.rt.auth.DisableUserTOTP(r.Context(), user.ID); err != nil {
		l.rt.internalError(w, "api: 2fa disable: save failed", err, slog.String("user_id", user.ID))
		return
	}
	if err := l.rt.recoveryCodes.DeleteUserRecoveryCodes(r.Context(), user.ID); err != nil {
		l.rt.logger.Warn("api: 2fa disable: delete built-in recovery codes failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
	}
	if l.rt.twoFactorSecrets != nil {
		if err := l.rt.twoFactorSecrets.DeleteAll(r.Context(), store.UserTOTPSecretsKey(user.ID)); err != nil {
			l.rt.logger.Warn("api: 2fa disable: delete built-in secret failed", slog.String("error", err.Error()), slog.String("user_id", user.ID))
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (l *authLibMFA) regenerate(w http.ResponseWriter, r *http.Request) {
	if !l.mfa.TOTPAvailable() {
		writeError(w, http.StatusNotImplemented, msgTwoFactorOff)
		return
	}
	user, ok := l.sessionUser(w, r, "api: 2fa regenerate")
	if !ok {
		return
	}
	st, err := l.mfa.TOTPStatus(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: 2fa regenerate: library status failed", user.ID, err)
		return
	}
	if !st.Enabled {
		writeError(w, http.StatusConflict, "two-factor authentication is not enabled")
		return
	}
	var req twoFactorCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, msgInvalidCode)
		return
	}
	if !l.checkCode(w, r, user.ID, req.Code, "") {
		return
	}
	codes, err := l.mfa.RegenerateRecoveryCodes(r.Context(), user.ID)
	if err != nil {
		l.fail(w, "api: 2fa regenerate: library regenerate failed", user.ID, err)
		return
	}
	l.mirrorRecoveryCodes(r, user.ID, codes)
	writeJSON(w, http.StatusOK, twoFactorRecoveryCodesResponse{RecoveryCodes: codes})
}

func (l *authLibMFA) verify(w http.ResponseWriter, r *http.Request) {
	rt := l.rt
	if rt.refuseInsecureLogin(w, r) {
		return
	}
	var req twoFactorVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	if req.MFAToken == "" || (req.Code == "" && req.RecoveryCode == "") {
		writeError(w, http.StatusBadRequest, "mfa_token and code or recovery_code are required")
		return
	}
	userID, ok := l.seam.PendingUser(req.MFAToken)
	if !ok {
		writeError(w, http.StatusUnauthorized, "login session expired, sign in again")
		return
	}
	limiterKey := loginLimiterKey(r, req.MFAToken)
	if allowed, retryAfter := rt.mfaVerify.allow(limiterKey); !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%.0f", retryAfter.Seconds()))
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	if !l.mfa.TOTPAvailable() {
		writeError(w, http.StatusNotImplemented, "two-factor authentication is not available")
		return
	}
	err := l.mfa.CheckSecondFactor(r.Context(), userID, req.Code, req.RecoveryCode, r.UserAgent(), clientIP(r))
	switch {
	case errors.Is(err, authengine.ErrInvalidCode):
		rt.mfaVerify.recordFailure(limiterKey)
		writeError(w, http.StatusUnauthorized, msgInvalidCode)
		return
	case err != nil:
		l.fail(w, "api: 2fa verify: check code failed", userID, err)
		return
	}
	rt.mfaVerify.recordSuccess(limiterKey)
	l.seam.RevokePending(req.MFAToken)
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: 2fa verify: load user failed", err, slog.String("user_id", userID))
		return
	}
	if err := l.seam.EstablishSession(w, r, *user); err != nil {
		rt.internalError(w, "api: 2fa verify: establish session failed", err, slog.String("user_id", userID))
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}
