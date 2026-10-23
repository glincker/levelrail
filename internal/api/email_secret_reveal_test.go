package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type emailRevealHarness struct {
	*mfaHarness
	logs     *bytes.Buffer
	provider string
	recovery []string
	step     int
}

// newEmailRevealHarness boots a router with email secrets and one resend provider holding provSecretA.
func newEmailRevealHarness(t *testing.T, enrollTOTP bool) *emailRevealHarness {
	t.Helper()
	h := &emailRevealHarness{logs: &bytes.Buffer{}}
	h.mfaHarness = newMFAHarness(t, WithEmailSecrets(newFakeEmailSecretsStore()))
	h.rt.logger = slog.New(slog.NewTextHandler(h.logs, nil))
	rec := h.do(http.MethodPost, "/api/v1/settings/email/providers", resendBody)
	var p emailProviderResource
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &p) != nil {
		t.Fatalf("create provider = %d %s", rec.Code, rec.Body.String())
	}
	h.provider = p.ID
	if enrollTOTP {
		rec = h.do(http.MethodPost, "/api/v1/auth/2fa/setup", "")
		var setup twoFactorSetupResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &setup) != nil {
			t.Fatalf("setup = %d %s", rec.Code, rec.Body.String())
		}
		h.secret = setup.Secret
		rec = h.do(http.MethodPost, "/api/v1/auth/2fa/confirm", `{"code":"`+h.code(0)+`"}`)
		var codes twoFactorRecoveryCodesResponse
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &codes) != nil || len(codes.RecoveryCodes) == 0 {
			t.Fatalf("confirm = %d %s", rec.Code, rec.Body.String())
		}
		h.recovery = codes.RecoveryCodes
	}
	return h
}

// nextCode returns a TOTP code from a fresh time step, because the library rejects a replayed code.
func (h *emailRevealHarness) nextCode() string {
	h.step++
	return h.code(time.Duration(h.step) * 30 * time.Second)
}

func (h *emailRevealHarness) revealPath(field string) string {
	return "/api/v1/settings/email/providers/" + h.provider + "/secrets/" + field + "/reveal"
}

func proofBody(password, code, recovery string) string {
	b, _ := json.Marshal(map[string]any{"step_up": map[string]string{"password": password, "code": code, "recovery_code": recovery}})
	return string(b)
}

func (h *emailRevealHarness) auditEntries() []store.AuditEntry {
	h.t.Helper()
	entries, err := h.db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{Action: store.AuditActionEmailSecretRevealed})
	if err != nil {
		h.t.Fatal(err)
	}
	return entries
}

func TestRevealEmailSecret_RequiresProof(t *testing.T) {
	h := newEmailRevealHarness(t, true)
	tests := []struct {
		name, body, code string
	}{
		{"no proof", `{}`, "step_up_required"},
		{"wrong password", proofBody("not-the-password", h.nextCode(), ""), "step_up_invalid"},
		{"wrong totp", proofBody(testAdminPassword, "000000", ""), "step_up_invalid"},
		{"password only", proofBody(testAdminPassword, "", ""), "step_up_code_required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.do(http.MethodPost, h.revealPath("resend_api_key"), tt.body)
			if rec.Code != http.StatusForbidden || errorCode(t, rec) != tt.code {
				t.Fatalf("status = %d code = %q body = %s", rec.Code, errorCode(t, rec), rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), provSecretA) {
				t.Fatal("refusal leaked the value")
			}
		})
	}
	if n := len(h.auditEntries()); n < len(tests) {
		t.Fatalf("failed attempts audited = %d, want at least %d", n, len(tests))
	}
	for _, e := range h.auditEntries() {
		if e.StatusCode == http.StatusOK || strings.Contains(e.Detail+e.Path, "not-the-password") || strings.Contains(e.Detail, provSecretA) {
			t.Fatalf("failed attempt audit row is wrong: %+v", e)
		}
	}
}

func TestRevealEmailSecret_TOTPAndRecoveryCode(t *testing.T) {
	h := newEmailRevealHarness(t, true)
	rec := h.do(http.MethodPost, h.revealPath("resend_api_key"), proofBody(testAdminPassword, h.nextCode(), ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("reveal = %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || len(body) != 1 || body["value"] != provSecretA {
		t.Fatalf("body = %s, want only the value", rec.Body.String())
	}
	rec = h.do(http.MethodPost, h.revealPath("resend_api_key"), proofBody(testAdminPassword, "", h.recovery[0]))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), provSecretA) {
		t.Fatalf("recovery code reveal = %d %s", rec.Code, rec.Body.String())
	}

	var ok int
	for _, e := range h.auditEntries() {
		raw, _ := json.Marshal(e)
		if strings.Contains(string(raw), provSecretA) || strings.Contains(string(raw), testAdminPassword) {
			t.Fatalf("audit row carries a secret: %s", raw)
		}
		if e.StatusCode == http.StatusOK {
			ok++
			if !strings.Contains(e.Detail, h.provider) || !strings.Contains(e.Detail, "resend_api_key") {
				t.Errorf("audit detail = %q, want provider id and field", e.Detail)
			}
		}
	}
	if ok != 2 {
		t.Fatalf("successful reveal audit rows = %d, want 2", ok)
	}
	if strings.Contains(h.logs.String(), provSecretA) {
		t.Fatal("the value reached the log")
	}
}

func TestRevealEmailSecret_RefusedWithoutTOTP(t *testing.T) {
	h := newEmailRevealHarness(t, false)
	rec := h.do(http.MethodPost, h.revealPath("resend_api_key"), proofBody(testAdminPassword, "", ""))
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != errCodeTwoFactorRequired {
		t.Fatalf("no-TOTP reveal = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), provSecretA) {
		t.Fatal("refusal leaked the value")
	}
}

func TestRevealEmailSecret_PinnedSessionRefused(t *testing.T) {
	h := newEmailRevealHarness(t, true)
	plain := seedRootAPIToken(t, h.db, "tok_reveal", []string{AbilityRoot})
	link, minted := mintSessionLink(t, h.rt, nil, plain)
	if minted.Code != http.StatusCreated {
		t.Fatalf("mint = %d %s", minted.Code, minted.Body.String())
	}
	pinned := sessionCookieFromRecorder(t, consumeSessionLinkRequest(t, h.rt, link.Token))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, h.revealPath("resend_api_key"), strings.NewReader(proofBody(testAdminPassword, h.nextCode(), "")))
	req.AddCookie(pinned)
	h.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || errorCode(t, rec) != errCodeStepUpSessionOnly {
		t.Fatalf("pinned reveal = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRevealEmailSecret_RateLimitedPerUser(t *testing.T) {
	t.Setenv(envEmailRevealPerMin, "2")
	h := newEmailRevealHarness(t, true)
	for i := 0; i < 2; i++ {
		if rec := h.do(http.MethodPost, h.revealPath("resend_api_key"), proofBody(testAdminPassword, "", h.recovery[i])); rec.Code != http.StatusOK {
			t.Fatalf("reveal %d = %d %s", i, rec.Code, rec.Body.String())
		}
	}
	rec := h.do(http.MethodPost, h.revealPath("resend_api_key"), proofBody(testAdminPassword, "", h.recovery[2]))
	if rec.Code != http.StatusTooManyRequests || strings.Contains(rec.Body.String(), provSecretA) {
		t.Fatalf("third reveal = %d %s, want 429", rec.Code, rec.Body.String())
	}
}

func TestEmailRevealLimiterDefault(t *testing.T) {
	l := newEmailRevealLimiter()
	for i := 0; i < defaultEmailRevealRate; i++ {
		if ok, _ := l.allow("u"); !ok {
			t.Fatalf("attempt %d blocked early", i)
		}
	}
	if ok, _ := l.allow("u"); ok {
		t.Fatal("default limit not enforced")
	}
}

func TestRevealEmailSecret_NotFoundCases(t *testing.T) {
	h := newEmailRevealHarness(t, true)
	tests := []struct{ name, path string }{
		{"field not on allowlist", h.revealPath("smtp_host")},
		{"field typo", h.revealPath("RESEND_API_KEY")},
		{"unset secret", h.revealPath("smtp_password")},
		{"unknown provider", "/api/v1/settings/email/providers/prov_nope/secrets/resend_api_key/reveal"},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.do(http.MethodPost, tt.path, proofBody(testAdminPassword, "", h.recovery[i]))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d body = %s, want 404", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEmailProviderReadsNeverCarryValues(t *testing.T) {
	h := newEmailRevealHarness(t, true)
	for _, target := range []string{"/api/v1/settings/email/providers", "/api/v1/settings/email/providers/" + h.provider, "/api/v1/settings/email"} {
		rec := h.do(http.MethodGet, target, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d", target, rec.Code)
		}
		if strings.Contains(rec.Body.String(), provSecretA) {
			t.Fatalf("GET %s carries a credential value", target)
		}
	}
}
