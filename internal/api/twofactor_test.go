package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/totp"
)

// newTestRouterWithTwoFactorSecrets mirrors newTestRouterWithGitHubApp:
// fakeGitHubAppSecrets satisfies TwoFactorSecrets structurally too (both
// interfaces are the identical SetValue/Resolve/DeleteAll shape), so
// this reuses it rather than hand-writing a second fake for the same
// contract.
func newTestRouterWithTwoFactorSecrets(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := discardLogger()
	rt := NewRouter(logger, testBrand(), db, WithTwoFactorSecrets(newFakeGitHubAppSecrets()))
	return rt, db
}

func doJSON(t *testing.T, rt *Router, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// setUpConfirmedTwoFactor drives the real setup+confirm handlers for the
// admin session cookie, returning the secret (so the caller can compute
// further live codes) and the recovery codes handleConfirmTwoFactor
// returned.
func setUpConfirmedTwoFactor(t *testing.T, rt *Router, cookie *http.Cookie) (secret string, recoveryCodes []string) {
	t.Helper()
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/setup", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var setupResp twoFactorSetupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &setupResp); err != nil {
		t.Fatalf("unmarshal setup response: %v", err)
	}

	code, err := totp.GenerateCode(setupResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/confirm", `{"code":"`+code+`"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var confirmResp twoFactorRecoveryCodesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &confirmResp); err != nil {
		t.Fatalf("unmarshal confirm response: %v", err)
	}
	return setupResp.Secret, confirmResp.RecoveryCodes
}

func TestTwoFactorSetupConfirmStatus_FullFlow(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)

	rec := doJSON(t, rt, http.MethodGet, "/api/v1/auth/2fa", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: code = %d, body = %s", rec.Code, rec.Body.String())
	}
	var status twoFactorStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.Enabled {
		t.Fatal("Enabled = true before setup, want false")
	}

	_, codes := setUpConfirmedTwoFactor(t, rt, cookie)
	if len(codes) != recoveryCodeCount {
		t.Fatalf("len(recovery codes) = %d, want %d", len(codes), recoveryCodeCount)
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/auth/2fa", "", cookie)
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !status.Enabled {
		t.Fatal("Enabled = false after confirm, want true")
	}
	if status.RecoveryCodesRemaining != recoveryCodeCount {
		t.Errorf("RecoveryCodesRemaining = %d, want %d", status.RecoveryCodesRemaining, recoveryCodeCount)
	}
}

func TestTwoFactorConfirm_WrongCodeRejected(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/setup", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("setup: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/confirm", `{"code":"000000"}`, cookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("confirm with wrong code: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/auth/2fa", "", cookie)
	var status twoFactorStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.Enabled {
		t.Error("Enabled = true after a rejected confirm, want false")
	}
}

func TestTwoFactorSetup_AlreadyEnabled_Conflict(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	setUpConfirmedTwoFactor(t, rt, cookie)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/setup", "", cookie)
	if rec.Code != http.StatusConflict {
		t.Fatalf("setup while already enabled: status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestTwoFactorSetup_NoSecretsConfigured_NotImplemented(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/setup", "", cookie)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("setup with no secrets manager: status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestLogin_WithTwoFactorEnabled_RequiresVerify(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	secret, _ := setUpConfirmedTwoFactor(t, rt, cookie)

	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName {
			t.Fatal("login set a session cookie before the second factor was verified")
		}
	}
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if !loginResp.MFARequired || loginResp.MFAToken == "" {
		t.Fatalf("loginResp = %+v, want MFARequired=true and a non-empty MFAToken", loginResp)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	verifyBody := `{"mfa_token":"` + loginResp.MFAToken + `","code":"` + code + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", verifyBody, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var found bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookieName && c.Value != "" {
			found = true
		}
	}
	if !found {
		t.Error("verify with a correct code did not set a session cookie")
	}
}

func TestVerifyTwoFactor_WrongCodeRejected_TokenStaysValidForRetry(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	secret, _ := setUpConfirmedTwoFactor(t, rt, cookie)

	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", `{"mfa_token":"`+loginResp.MFAToken+`","code":"000000"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify wrong code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", `{"mfa_token":"`+loginResp.MFAToken+`","code":"`+code+`"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify correct code after one wrong attempt: status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestVerifyTwoFactor_UnknownToken_Unauthorized(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	setUpConfirmedTwoFactor(t, rt, cookie)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", `{"mfa_token":"does-not-exist","code":"123456"}`, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify with unknown token: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestVerifyTwoFactor_RecoveryCode_SingleUse(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	_, codes := setUpConfirmedTwoFactor(t, rt, cookie)
	recoveryCode := codes[0]

	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	verifyBody := `{"mfa_token":"` + loginResp.MFAToken + `","recovery_code":"` + recoveryCode + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", verifyBody, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify with recovery code: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	// Log in again and try to reuse the same recovery code: must fail,
	// single-use.
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal second login response: %v", err)
	}
	reusedBody := `{"mfa_token":"` + loginResp.MFAToken + `","recovery_code":"` + recoveryCode + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", reusedBody, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestTwoFactorDisable_RequiresValidCode(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	secret, _ := setUpConfirmedTwoFactor(t, rt, cookie)

	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/disable", `{"code":"000000"}`, cookie)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("disable with wrong code: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/disable", `{"code":"`+code+`"}`, cookie)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("disable with correct code: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, rt, http.MethodGet, "/api/v1/auth/2fa", "", cookie)
	var status twoFactorStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if status.Enabled {
		t.Error("Enabled = true after disable, want false")
	}

	// Logging in again must no longer require a second factor.
	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("login after disable: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if loginResp.MFARequired {
		t.Error("login after disable still required MFA")
	}
}

func TestTwoFactorRegenerateRecoveryCodes_InvalidatesOldSet(t *testing.T) {
	rt, db := newTestRouterWithTwoFactorSecrets(t)
	cookie := loginTestSession(t, rt, db)
	secret, oldCodes := setUpConfirmedTwoFactor(t, rt, cookie)

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	rec := doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/recovery-codes/regenerate", `{"code":"`+code+`"}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("regenerate: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp twoFactorRecoveryCodesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.RecoveryCodes) != recoveryCodeCount {
		t.Fatalf("len(new recovery codes) = %d, want %d", len(resp.RecoveryCodes), recoveryCodeCount)
	}

	loginBody := `{"username":"` + testAdminUsername + `","password":"` + testAdminPassword + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/login", loginBody, nil)
	var loginResp loginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	oldBody := `{"mfa_token":"` + loginResp.MFAToken + `","recovery_code":"` + oldCodes[0] + `"}`
	rec = doJSON(t, rt, http.MethodPost, "/api/v1/auth/2fa/verify", oldBody, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("verify with a code from the replaced set: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
