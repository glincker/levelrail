package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/totp"
)

// TwoFactorSecrets is the surface these handlers need from
// internal/secrets.Manager: SetValue at setup time, Resolve to check a
// submitted code against the stored secret (confirm, disable,
// regenerate, and the login-time verify handler all need this), and
// DeleteAll when 2FA is turned off. *secrets.Manager satisfies this
// structurally, the same set-then-resolve shape GitHubAppSecrets
// already documents for the identical need.
type TwoFactorSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

const (
	totpSecretEnvKey  = "secret"
	recoveryCodeCount = 10
	mfaPendingTTL     = 5 * time.Minute
)

// mfaPending identifies who is mid-login after a correct password but
// before a correct second factor: a session cookie is not issued until
// handleVerifyTwoFactor consumes one of these successfully.
type mfaPending struct {
	userID    string
	expiresAt time.Time
}

// mfaPendingStore is a short-TTL, in-memory sibling of sessionStore,
// same shape, much narrower purpose: it identifies a mid-login user,
// nothing else, and expires in minutes rather than hours.
type mfaPendingStore struct {
	mu      sync.Mutex
	pending map[string]mfaPending
}

func newMFAPendingStore() *mfaPendingStore {
	return &mfaPendingStore{pending: make(map[string]mfaPending)}
}

func (s *mfaPendingStore) create(userID string) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", fmt.Errorf("api: generate mfa pending token: %w", err)
	}
	s.mu.Lock()
	s.pending[token] = mfaPending{userID: userID, expiresAt: time.Now().Add(mfaPendingTTL)}
	s.mu.Unlock()
	return token, nil
}

// lookup does not delete on read: a mistyped code must not burn the
// pending token, the caller gets to keep retrying (subject to
// rt.mfaVerify's rate limit) until it expires or a correct one lands.
func (s *mfaPendingStore) lookup(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[token]
	if !ok {
		return "", false
	}
	if time.Now().After(p.expiresAt) {
		delete(s.pending, token)
		return "", false
	}
	return p.userID, true
}

func (s *mfaPendingStore) revoke(token string) {
	s.mu.Lock()
	delete(s.pending, token)
	s.mu.Unlock()
}

type twoFactorStatusResponse struct {
	Enabled                bool `json:"enabled"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

// handleGetTwoFactorStatus handles GET /api/v1/auth/2fa: whether the
// caller's own account has TOTP enabled, and if so how many recovery
// codes are left unused. Works with no master key configured, unlike
// every other route in this file: it only ever reads store.User and
// recovery-code rows, neither of which needs internal/secrets.
func (rt *Router) handleGetTwoFactorStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa status: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	resp := twoFactorStatusResponse{Enabled: user.TOTPEnabled}
	if user.TOTPEnabled {
		n, err := rt.recoveryCodes.CountUnusedUserRecoveryCodes(r.Context(), userID)
		if err != nil {
			rt.logger.Error("api: 2fa status: count recovery codes failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resp.RecoveryCodesRemaining = n
	}
	writeJSON(w, http.StatusOK, resp)
}

type twoFactorSetupResponse struct {
	Secret          string `json:"secret"`
	ProvisioningURI string `json:"provisioning_uri"`
}

// handleSetupTwoFactor handles POST /api/v1/auth/2fa/setup: mints a
// fresh secret and stores it unconfirmed (store.User.TOTPEnabled stays
// false until handleConfirmTwoFactor validates a real code against it).
// Calling this again before confirming just overwrites the pending
// secret; nothing reads the old one once a new one lands.
func (rt *Router) handleSetupTwoFactor(w http.ResponseWriter, r *http.Request) {
	if rt.twoFactorSecrets == nil {
		writeError(w, http.StatusNotImplemented, "two-factor authentication requires a master key to be configured on this control plane")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa setup: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.TOTPEnabled {
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
		return
	}

	secret, err := totp.GenerateSecret()
	if err != nil {
		rt.logger.Error("api: 2fa setup: generate secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.twoFactorSecrets.SetValue(r.Context(), store.UserTOTPSecretsKey(userID), totpSecretEnvKey, secret); err != nil {
		rt.logger.Error("api: 2fa setup: store secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, twoFactorSetupResponse{
		Secret:          secret,
		ProvisioningURI: totp.ProvisioningURI(secret, user.Email, rt.brand.Name),
	})
}

type twoFactorCodeRequest struct {
	Code string `json:"code"`
}

type twoFactorRecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// handleConfirmTwoFactor handles POST /api/v1/auth/2fa/confirm: the
// second half of setup, proving the caller's authenticator app actually
// has the secret handleSetupTwoFactor stored by validating a live code
// against it. On success this is the one moment recovery codes are ever
// shown in plaintext.
func (rt *Router) handleConfirmTwoFactor(w http.ResponseWriter, r *http.Request) {
	if rt.twoFactorSecrets == nil {
		writeError(w, http.StatusNotImplemented, "two-factor authentication requires a master key to be configured on this control plane")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa confirm: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if user.TOTPEnabled {
		writeError(w, http.StatusConflict, "two-factor authentication is already enabled")
		return
	}

	var req twoFactorCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	secret, err := rt.twoFactorSecrets.Resolve(r.Context(), store.UserTOTPSecretsKey(userID), totpSecretEnvKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "call setup before confirm")
		return
	}
	if !totp.Validate(secret, req.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}

	if err := rt.auth.EnableUserTOTP(r.Context(), userID, time.Now()); err != nil {
		rt.logger.Error("api: 2fa confirm: enable failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		rt.logger.Error("api: 2fa confirm: generate recovery codes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.recoveryCodes.ReplaceUserRecoveryCodes(r.Context(), userID, hashes); err != nil {
		rt.logger.Error("api: 2fa confirm: save recovery codes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, twoFactorRecoveryCodesResponse{RecoveryCodes: codes})
}

type twoFactorDisableRequest struct {
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

// handleDisableTwoFactor handles POST /api/v1/auth/2fa/disable.
// Deliberately re-verifies the second factor itself rather than the
// account password: the whole point of 2FA is to stop someone who only
// has the password, so letting a password re-check turn it off would
// undermine the exact property it exists to provide.
func (rt *Router) handleDisableTwoFactor(w http.ResponseWriter, r *http.Request) {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa disable: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !user.TOTPEnabled {
		writeError(w, http.StatusConflict, "two-factor authentication is not enabled")
		return
	}

	var req twoFactorDisableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	valid, err := rt.verifyTwoFactorCode(r.Context(), userID, req.Code, req.RecoveryCode)
	if err != nil {
		rt.logger.Error("api: 2fa disable: verify code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}

	if err := rt.auth.DisableUserTOTP(r.Context(), userID); err != nil {
		rt.logger.Error("api: 2fa disable: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Best-effort cleanup below: store.User.TOTPEnabled=false (just set
	// above) is what every other code path actually checks, so a
	// leftover secret or recovery-code row here is inert, not a security
	// hole, if either delete fails.
	if err := rt.recoveryCodes.DeleteUserRecoveryCodes(r.Context(), userID); err != nil {
		rt.logger.Warn("api: 2fa disable: delete recovery codes failed", slog.String("error", err.Error()), slog.String("user_id", userID))
	}
	if rt.twoFactorSecrets != nil {
		if err := rt.twoFactorSecrets.DeleteAll(r.Context(), store.UserTOTPSecretsKey(userID)); err != nil {
			rt.logger.Warn("api: 2fa disable: delete secret failed", slog.String("error", err.Error()), slog.String("user_id", userID))
		}
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleRegenerateRecoveryCodes handles
// POST /api/v1/auth/2fa/recovery-codes/regenerate: replaces the whole
// set, invalidating every previously issued code, gated on a live TOTP
// code specifically (not a recovery code), so regenerating never
// consumes one of the codes it's about to throw away.
func (rt *Router) handleRegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	if rt.twoFactorSecrets == nil {
		writeError(w, http.StatusNotImplemented, "two-factor authentication requires a master key to be configured on this control plane")
		return
	}
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "authentication required")
		return
	}
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa regenerate: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !user.TOTPEnabled {
		writeError(w, http.StatusConflict, "two-factor authentication is not enabled")
		return
	}

	var req twoFactorCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	secret, err := rt.twoFactorSecrets.Resolve(r.Context(), store.UserTOTPSecretsKey(userID), totpSecretEnvKey)
	if err != nil {
		rt.logger.Error("api: 2fa regenerate: resolve secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !totp.Validate(secret, req.Code, time.Now()) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}

	codes, hashes, err := generateRecoveryCodes()
	if err != nil {
		rt.logger.Error("api: 2fa regenerate: generate recovery codes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.recoveryCodes.ReplaceUserRecoveryCodes(r.Context(), userID, hashes); err != nil {
		rt.logger.Error("api: 2fa regenerate: save recovery codes failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, twoFactorRecoveryCodesResponse{RecoveryCodes: codes})
}

type twoFactorVerifyRequest struct {
	MFAToken     string `json:"mfa_token"`
	Code         string `json:"code"`
	RecoveryCode string `json:"recovery_code"`
}

// handleVerifyTwoFactor handles POST /api/v1/auth/2fa/verify: step two
// of login for an account with TOTP enabled, exchanging the mfa_token
// handleLogin returned plus a valid code (or recovery code) for a real
// session, the same establishSession every other sign-in path funnels
// through. Necessarily public, the caller has no session yet; guarded
// instead by rt.mfaVerify, a rate limiter of its own separate from
// rt.logins, since brute-forcing a 6-digit code after a correct
// password is a distinct attack surface from password guessing.
func (rt *Router) handleVerifyTwoFactor(w http.ResponseWriter, r *http.Request) {
	var req twoFactorVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MFAToken == "" || (req.Code == "" && req.RecoveryCode == "") {
		writeError(w, http.StatusBadRequest, "mfa_token and code or recovery_code are required")
		return
	}

	userID, ok := rt.mfaPending.lookup(req.MFAToken)
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

	if rt.twoFactorSecrets == nil {
		// TOTPEnabled can only be true if setup/confirm succeeded, both
		// of which require rt.twoFactorSecrets, so reaching here means a
		// control plane lost its master key configuration between then
		// and now; the same "not configured" 501 shape every other route
		// in this file uses.
		writeError(w, http.StatusNotImplemented, "two-factor authentication is not available")
		return
	}

	valid, err := rt.verifyTwoFactorCode(r.Context(), userID, req.Code, req.RecoveryCode)
	if err != nil {
		rt.logger.Error("api: 2fa verify: check code failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !valid {
		rt.mfaVerify.recordFailure(limiterKey)
		writeError(w, http.StatusUnauthorized, "invalid code")
		return
	}
	rt.mfaVerify.recordSuccess(limiterKey)
	rt.mfaPending.revoke(req.MFAToken)

	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		rt.logger.Error("api: 2fa verify: load user failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.establishSession(r.Context(), w, *user); err != nil {
		rt.logger.Error("api: 2fa verify: establish session failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Email: user.Email, DisplayName: user.DisplayName})
}

// verifyTwoFactorCode checks exactly one of code (a live TOTP) or
// recoveryCode (a single-use fallback, consumed on match) against
// userID's stored second factor, trying code first if both are somehow
// set. valid=false, err=nil means "wrong code," the ordinary failure
// case every caller turns into a generic 400/401; err != nil means a
// real backend failure the caller should log and turn into a 500
// instead of a user-facing "invalid code."
func (rt *Router) verifyTwoFactorCode(ctx context.Context, userID, code, recoveryCode string) (valid bool, err error) {
	switch {
	case code != "":
		secret, resolveErr := rt.twoFactorSecrets.Resolve(ctx, store.UserTOTPSecretsKey(userID), totpSecretEnvKey)
		if resolveErr != nil {
			return false, nil
		}
		return totp.Validate(secret, code, time.Now()), nil
	case recoveryCode != "":
		hash := hashToken(totp.NormalizeRecoveryCode(recoveryCode))
		return rt.recoveryCodes.ConsumeUserRecoveryCode(ctx, userID, hash)
	default:
		return false, nil
	}
}

// generateRecoveryCodes mints recoveryCodeCount fresh single-use
// fallback codes, returning both the plaintext (shown to the caller
// exactly once, by whichever handler called this) and their SHA-256
// hashes, the only form ReplaceUserRecoveryCodes ever persists.
func generateRecoveryCodes() (codes, hashes []string, err error) {
	codes = make([]string, 0, recoveryCodeCount)
	hashes = make([]string, 0, recoveryCodeCount)
	for range recoveryCodeCount {
		code, genErr := totp.GenerateRecoveryCode()
		if genErr != nil {
			return nil, nil, fmt.Errorf("generate recovery code: %w", genErr)
		}
		codes = append(codes, code)
		hashes = append(hashes, hashToken(totp.NormalizeRecoveryCode(code)))
	}
	return codes, hashes, nil
}
