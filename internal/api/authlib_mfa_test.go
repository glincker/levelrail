package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
)

type mfaStep struct {
	name string
	run  func(h *mfaHarness) *httptest.ResponseRecorder
}

type mfaGoldenRow struct {
	Step   string
	Status int
	Body   any
}

// runGolden replays steps against a fresh router and compares each status and
// normalized body with testdata/mfa_golden_<name>.json (APP_UPDATE_GOLDEN=1 rewrites it).
func runGolden(t *testing.T, name string, steps []mfaStep) {
	t.Helper()
	h := newMFAHarness(t)
	got := make([]mfaGoldenRow, 0, len(steps))
	for _, s := range steps {
		rec := s.run(h)
		got = append(got, mfaGoldenRow{Step: s.name, Status: rec.Code, Body: normalizeBody(t, rec.Body.String())})
	}
	path := "testdata/mfa_golden_" + name + ".json"
	if os.Getenv("APP_UPDATE_GOLDEN") == "1" {
		raw, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want []mfaGoldenRow
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if len(want) != len(got) {
		t.Fatalf("steps = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Status != want[i].Status || !reflect.DeepEqual(got[i].Body, want[i].Body) {
			t.Fatalf("%s differs from the contract\n got  %d %v\n want %d %v", want[i].Step, got[i].Status, got[i].Body, want[i].Status, want[i].Body)
		}
	}
}

func TestMFAGoldenTOTPContract(t *testing.T) {
	post := func(path, body string) func(h *mfaHarness) *httptest.ResponseRecorder {
		return func(h *mfaHarness) *httptest.ResponseRecorder { return h.do(http.MethodPost, path, body) }
	}
	withCode := func(path string, offset time.Duration, tpl string) func(h *mfaHarness) *httptest.ResponseRecorder {
		return func(h *mfaHarness) *httptest.ResponseRecorder {
			return h.do(http.MethodPost, path, `{"`+tpl+`":"`+h.code(offset)+`"}`)
		}
	}
	setup := func(h *mfaHarness) *httptest.ResponseRecorder {
		rec := h.do(http.MethodPost, "/api/v1/auth/2fa/setup", "")
		var resp twoFactorSetupResponse
		if json.Unmarshal(rec.Body.Bytes(), &resp) == nil {
			h.secret = resp.Secret
		}
		return rec
	}
	steps := []mfaStep{
		{"status before enrollment", func(h *mfaHarness) *httptest.ResponseRecorder { return h.do(http.MethodGet, "/api/v1/auth/2fa", "") }},
		{"status without a session", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodGet, "/api/v1/auth/2fa", "", nil)
		}},
		{"confirm before setup", post("/api/v1/auth/2fa/confirm", `{"code":"123456"}`)},
		{"setup", setup},
		{"setup again before confirm", setup},
		{"confirm with a bad code", post("/api/v1/auth/2fa/confirm", `{"code":"000000"}`)},
		{"confirm with a bad body", post("/api/v1/auth/2fa/confirm", `not json`)},
		{"confirm", withCode("/api/v1/auth/2fa/confirm", 0, "code")},
		{"status after enrollment", func(h *mfaHarness) *httptest.ResponseRecorder { return h.do(http.MethodGet, "/api/v1/auth/2fa", "") }},
		{"setup when enabled", post("/api/v1/auth/2fa/setup", "")},
		{"confirm when enabled", post("/api/v1/auth/2fa/confirm", `{"code":"000000"}`)},
		{"regenerate with a bad code", post("/api/v1/auth/2fa/recovery-codes/regenerate", `{"code":"000000"}`)},
		{"regenerate with no code", post("/api/v1/auth/2fa/recovery-codes/regenerate", `{}`)},
		{"regenerate", withCode("/api/v1/auth/2fa/recovery-codes/regenerate", -30*time.Second, "code")},
		{"status after regeneration", func(h *mfaHarness) *httptest.ResponseRecorder { return h.do(http.MethodGet, "/api/v1/auth/2fa", "") }},
		{"disable with a bad code", post("/api/v1/auth/2fa/disable", `{"code":"000000"}`)},
		{"disable with no code", post("/api/v1/auth/2fa/disable", `{}`)},
		{"disable with a bad body", post("/api/v1/auth/2fa/disable", `not json`)},
		{"disable", withCode("/api/v1/auth/2fa/disable", 0, "code")},
		{"status after disable", func(h *mfaHarness) *httptest.ResponseRecorder { return h.do(http.MethodGet, "/api/v1/auth/2fa", "") }},
		{"disable when not enabled", post("/api/v1/auth/2fa/disable", `{"code":"000000"}`)},
		{"regenerate when not enabled", post("/api/v1/auth/2fa/recovery-codes/regenerate", `{"code":"000000"}`)},
	}
	runGolden(t, "totp", steps)
}

func TestMFAGoldenLoginVerifyContract(t *testing.T) {
	steps := []mfaStep{
		{"enroll", func(h *mfaHarness) *httptest.ResponseRecorder {
			h.enrollTOTP()
			return httptest.NewRecorder()
		}},
		{"password login asks for the second factor", func(h *mfaHarness) *httptest.ResponseRecorder {
			rec := h.passwordLogin()
			h.mfaToken(rec)
			rec.Body.Reset()
			return rec
		}},
		{"verify with an unknown token", func(h *mfaHarness) *httptest.ResponseRecorder { return h.verifyLogin("nope", "123456") }},
		{"verify with a bad code", func(h *mfaHarness) *httptest.ResponseRecorder {
			return h.verifyLogin(h.mfaToken(h.passwordLogin()), "000000")
		}},
		{"verify with a missing code", func(h *mfaHarness) *httptest.ResponseRecorder {
			return h.verifyLogin(h.mfaToken(h.passwordLogin()), "")
		}},
		{"verify", func(h *mfaHarness) *httptest.ResponseRecorder {
			rec := h.verifyLogin(h.mfaToken(h.passwordLogin()), h.code(30*time.Second))
			if !hasSessionCookie(rec) {
				h.t.Fatalf("verify must set a session cookie: %d %s", rec.Code, rec.Body.String())
			}
			return rec
		}},
	}
	runGolden(t, "login_verify", steps)
}

func TestMFAGoldenPasskeyContract(t *testing.T) {
	list := func(h *mfaHarness) *httptest.ResponseRecorder {
		return h.do(http.MethodGet, "/api/v1/auth/passkeys", "")
	}
	steps := []mfaStep{
		{"list when empty", list},
		{"register begin without a session", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkeys/register/begin", "", nil)
		}},
		{"register finish without session_id", func(h *mfaHarness) *httptest.ResponseRecorder {
			return h.do(http.MethodPost, "/api/v1/auth/passkeys/register/finish", "{}")
		}},
		{"register", func(h *mfaHarness) *httptest.ResponseRecorder {
			sessionID, opts := h.registerBegin()
			k := softKey{cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2), auth: virtualwebauthn.NewAuthenticator()}
			return h.registerFinish(sessionID, "Golden Key", virtualwebauthn.CreateAttestationResponse(h.rp(), k.auth, k.cred, *opts))
		}},
		{"list after register", list},
		{"delete an unknown passkey", func(h *mfaHarness) *httptest.ResponseRecorder {
			return h.do(http.MethodDelete, "/api/v1/auth/passkeys/pk_missing", "")
		}},
		{"delete", func(h *mfaHarness) *httptest.ResponseRecorder {
			var rows []passkeyResource
			if err := json.Unmarshal(list(h).Body.Bytes(), &rows); err != nil || len(rows) != 1 {
				h.t.Fatalf("want one passkey to delete, got %v (%v)", rows, err)
			}
			return h.do(http.MethodDelete, "/api/v1/auth/passkeys/"+rows[0].ID, "")
		}},
		{"list after delete", list},
		{"login begin for an unknown user", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"nobody@example.com"}`, nil)
		}},
		{"login begin with no passkey registered", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{"username":"`+testAdminUsername+`"}`, nil)
		}},
		{"login begin with no username", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkey-login/begin", `{}`, nil)
		}},
		{"login finish without session_id", func(h *mfaHarness) *httptest.ResponseRecorder {
			return doJSON(h.t, h.rt, http.MethodPost, "/api/v1/auth/passkey-login/finish", `{}`, nil)
		}},
	}
	runGolden(t, "passkey", steps)
}

func TestMFALibraryPasskeyCeremonies(t *testing.T) {
	h := newMFAHarness(t)
	k := h.registerPasskey("Laptop")

	t.Run("discoverable login establishes a session and records the sign count", func(t *testing.T) {
		k.cred.Counter = 1
		target, body := h.passkeyAssertion(k, h.rp())
		rec := h.finishLogin(target, body)
		if rec.Code != http.StatusOK || !hasSessionCookie(rec) {
			t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
		}
		var resp loginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.Email != testAdminUsername {
			t.Fatalf("login body = %s", rec.Body.String())
		}
	})

	tests := []struct {
		name    string
		counter uint32
		want    int
	}{
		{"a counter that advances is accepted", 2, http.StatusOK},
		{"a regressed counter is rejected as a clone", 1, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			k.cred.Counter = tc.counter
			target, body := h.passkeyAssertion(k, h.rp())
			if rec := h.finishLogin(target, body); rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}

	t.Run("an assertion for the wrong origin is rejected", func(t *testing.T) {
		k.cred.Counter = 9
		rp := h.rp()
		rp.Origin = "https://evil.example.net"
		target, body := h.passkeyAssertion(k, rp)
		if rec := h.finishLogin(target, body); rec.Code != http.StatusUnauthorized || hasSessionCookie(rec) {
			t.Fatalf("status = %d, want 401 and no session: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("a registration for the wrong origin or rp id is rejected", func(t *testing.T) {
		for name, mutate := range map[string]func(*virtualwebauthn.RelyingParty){
			"origin": func(rp *virtualwebauthn.RelyingParty) { rp.Origin = "https://evil.example.net" },
			"rp id":  func(rp *virtualwebauthn.RelyingParty) { rp.ID = "evil.example.net" },
		} {
			sessionID, opts := h.registerBegin()
			rp := h.rp()
			mutate(&rp)
			cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
			body := virtualwebauthn.CreateAttestationResponse(rp, virtualwebauthn.NewAuthenticator(), cred, *opts)
			if rec := h.registerFinish(sessionID, name, body); rec.Code != http.StatusBadRequest {
				t.Fatalf("%s mismatch: status = %d, want 400: %s", name, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("rename", func(t *testing.T) {
		var rows []passkeyResource
		if err := json.Unmarshal(h.do(http.MethodGet, "/api/v1/auth/passkeys", "").Body.Bytes(), &rows); err != nil || len(rows) != 1 {
			t.Fatalf("list = %v, %v", rows, err)
		}
		path := "/api/v1/auth/passkeys/" + rows[0].ID
		if rec := h.do(http.MethodPatch, path, `{"label":"Work laptop"}`); rec.Code != http.StatusOK {
			t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
		}
		if rec := h.do(http.MethodPatch, path, `{"label":""}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("empty label: %d", rec.Code)
		}
		if rec := h.do(http.MethodPatch, "/api/v1/auth/passkeys/not-a-ulid", `{"label":"x"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown id: %d", rec.Code)
		}
		var after []passkeyResource
		_ = json.Unmarshal(h.do(http.MethodGet, "/api/v1/auth/passkeys", "").Body.Bytes(), &after)
		if len(after) != 1 || after[0].Label != "Work laptop" {
			t.Fatalf("label after rename = %+v", after)
		}
	})
}

func TestMFALibraryLockoutAfterRepeatedBadCodes(t *testing.T) {
	h := newMFAHarness(t)
	h.enrollTOTP()
	for i := 0; i < 5; i++ {
		if rec := h.do(http.MethodPost, "/api/v1/auth/2fa/disable", `{"code":"000000"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("bad attempt %d: %d", i, rec.Code)
		}
	}
	rec := h.do(http.MethodPost, "/api/v1/auth/2fa/disable", `{"code":"`+h.code(30*time.Second)+`"}`)
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("locked account: %d retry-after=%q", rec.Code, rec.Header().Get("Retry-After"))
	}
}

func TestMFARecoveryCodeFormat(t *testing.T) {
	format := regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){3}$`)
	{
		h := newMFAHarness(t)
		h.enrollTOTP()
		rec := h.do(http.MethodPost, "/api/v1/auth/2fa/recovery-codes/regenerate", `{"code":"`+h.code(30*time.Second)+`"}`)
		var resp twoFactorRecoveryCodesResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || len(resp.RecoveryCodes) != 10 {
			t.Fatalf("regenerate: %d %s", rec.Code, rec.Body.String())
		}
		for _, c := range resp.RecoveryCodes {
			if !format.MatchString(c) {
				t.Fatalf("code %q is not XXXX-XXXX-XXXX-XXXX", c)
			}
		}
		spellings := []string{resp.RecoveryCodes[0], strings.ToLower(strings.ReplaceAll(resp.RecoveryCodes[1], "-", ""))}
		for _, code := range spellings {
			body := fmt.Sprintf(`{"mfa_token":%q,"recovery_code":%q}`, h.mfaToken(h.passwordLogin()), code)
			if rec := doJSON(t, h.rt, http.MethodPost, "/api/v1/auth/2fa/verify", body, nil); rec.Code != http.StatusOK {
				t.Fatalf("recovery code %q: %d %s", code, rec.Code, rec.Body.String())
			}
		}
	}
}
