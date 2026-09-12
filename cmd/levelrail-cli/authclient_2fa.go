package main

import (
	"context"
	"net/http"
)

// twoFactorStatusResponse mirrors internal/api's twoFactorStatusResponse
// (internal/api/twofactor.go).
type twoFactorStatusResponse struct {
	Enabled                bool `json:"enabled"`
	RecoveryCodesRemaining int  `json:"recovery_codes_remaining"`
}

// TwoFactorStatus calls GET /api/v1/auth/2fa: session-only, same gate as
// CreateToken/ListTokens/RevokeToken above, since it reads the caller's
// own account security state.
func (c *authSessionClient) TwoFactorStatus(ctx context.Context) (twoFactorStatusResponse, error) {
	var out twoFactorStatusResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/2fa", nil, &out)
	return out, err
}

// twoFactorSetupResponse mirrors internal/api's twoFactorSetupResponse.
type twoFactorSetupResponse struct {
	Secret          string `json:"secret"`
	ProvisioningURI string `json:"provisioning_uri"`
}

// SetupTwoFactor calls POST /api/v1/auth/2fa/setup: mints a fresh,
// unconfirmed TOTP secret. Calling it again before EnableTwoFactor
// confirms just overwrites the pending secret server-side.
func (c *authSessionClient) SetupTwoFactor(ctx context.Context) (twoFactorSetupResponse, error) {
	var out twoFactorSetupResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/2fa/setup", nil, &out)
	return out, err
}

// twoFactorCodeRequest mirrors internal/api's twoFactorCodeRequest.
type twoFactorCodeRequest struct {
	Code string `json:"code"`
}

// twoFactorRecoveryCodesResponse mirrors internal/api's
// twoFactorRecoveryCodesResponse: plaintext recovery codes, shown exactly
// once per call, never recoverable from the server again afterward.
type twoFactorRecoveryCodesResponse struct {
	RecoveryCodes []string `json:"recovery_codes"`
}

// EnableTwoFactor calls POST /api/v1/auth/2fa/confirm: the second half
// of setup, proving code came from the secret SetupTwoFactor stored.
// Returns the account's recovery codes, printed exactly once.
func (c *authSessionClient) EnableTwoFactor(ctx context.Context, code string) (twoFactorRecoveryCodesResponse, error) {
	var out twoFactorRecoveryCodesResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/2fa/confirm", twoFactorCodeRequest{Code: code}, &out)
	return out, err
}

// twoFactorDisableRequest mirrors internal/api's twoFactorDisableRequest:
// exactly one of Code or RecoveryCode is expected.
type twoFactorDisableRequest struct {
	Code         string `json:"code,omitempty"`
	RecoveryCode string `json:"recovery_code,omitempty"`
}

// DisableTwoFactor calls POST /api/v1/auth/2fa/disable, re-verifying the
// second factor itself (a live code or a recovery code) rather than the
// account password, same reasoning handleDisableTwoFactor's own doc
// comment gives.
func (c *authSessionClient) DisableTwoFactor(ctx context.Context, code, recoveryCode string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/auth/2fa/disable", twoFactorDisableRequest{Code: code, RecoveryCode: recoveryCode}, nil)
}

// RegenerateRecoveryCodes calls
// POST /api/v1/auth/2fa/recovery-codes/regenerate: replaces the whole
// set, invalidating every previously issued code. Gated on a live TOTP
// code specifically, never a recovery code, same as the server route.
func (c *authSessionClient) RegenerateRecoveryCodes(ctx context.Context, code string) (twoFactorRecoveryCodesResponse, error) {
	var out twoFactorRecoveryCodesResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/2fa/recovery-codes/regenerate", twoFactorCodeRequest{Code: code}, &out)
	return out, err
}
