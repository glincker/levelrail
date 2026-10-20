package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// trustThisBrowser signs in past an approval so the harness user ends up
// with one trusted browser, and returns its cookie plus a live session.
func (h *codeHarness) trustThisBrowser() (trusted, session *http.Cookie) {
	h.t.Helper()
	session = responseCookie(h.passwordLogin(), sessionCookieName)
	id, binding := h.pendingApproval(h.passwordLogin())
	if rec := h.approve(id, session); rec.Code != http.StatusNoContent {
		h.t.Fatalf("approve = %d", rec.Code)
	}
	_, rec := h.poll(binding)
	trusted = responseCookie(rec, trustedCookie)
	if trusted == nil {
		h.t.Fatal("no trusted cookie")
	}
	return trusted, session
}

func (h *codeHarness) trustedRows() int {
	h.t.Helper()
	var n int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM trusted_devices WHERE user_id = ? AND revoked_at = ''`, h.user.ID).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func TestTrustedDevices_RevokedOnCredentialAndSessionResets(t *testing.T) {
	tests := []struct {
		name string
		run  func(h *codeHarness, session *http.Cookie) int
		want int
	}{
		{"password change", func(h *codeHarness, s *http.Cookie) int {
			return h.send(http.MethodPut, "/api/v1/auth/password", `{"current_password":"`+codeTestPassword+`","new_password":"another-long-password"}`, "", s).Code
		}, http.StatusNoContent},
		{"revoke other sessions", func(h *codeHarness, s *http.Cookie) int {
			return h.send(http.MethodPost, "/api/v1/auth/sessions/revoke-others", "", "", s).Code
		}, http.StatusNoContent},
		{"user deletion", func(h *codeHarness, _ *http.Cookie) int {
			return h.send(http.MethodDelete, "/api/v1/users/"+h.user.ID, "", "", loginTestSession(h.t, h.rt, h.db)).Code
		}, http.StatusNoContent},
		{"password reset", func(h *codeHarness, _ *http.Cookie) int {
			h.send(http.MethodPost, "/api/v1/auth/forgot-password", `{"email":"`+codeTestEmail+`"}`, "")
			var body string
			for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && body == ""; time.Sleep(10 * time.Millisecond) {
				body = h.mail.all()
			}
			token := tokenFromResetEmail(h.t, body)
			return h.send(http.MethodPost, "/api/v1/auth/reset-password", `{"token":"`+token+`","new_password":"another-long-password"}`, "").Code
		}, http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			_, session := h.trustThisBrowser()
			if h.trustedRows() != 1 {
				t.Fatalf("trusted rows = %d before the reset", h.trustedRows())
			}
			if got := tt.run(h, session); got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
			if n := h.trustedRows(); n != 0 {
				t.Fatalf("trusted rows = %d after the reset, want 0", n)
			}
		})
	}
}

func TestTrustedDevices_ReuseAndPrune(t *testing.T) {
	h := newCodeHarness(t, true)
	trusted, _ := h.trustThisBrowser()
	for range 3 {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.AddCookie(trusted)
		h.rt.trustDevice(httptest.NewRecorder(), req, h.user.ID)
	}
	if n := h.trustedRows(); n != 1 {
		t.Fatalf("trusted rows = %d after repeat sign-ins from one browser, want 1", n)
	}
	past := store.FormatAuditTime(time.Now().Add(-time.Hour))
	if _, err := h.db.ExecContext(context.Background(), `UPDATE trusted_devices SET expires_at = ?`, past); err != nil {
		t.Fatal(err)
	}
	if err := h.rt.SweepSignInExpiry(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := h.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM trusted_devices`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("rows after sweep = %d (%v), want 0", n, err)
	}
}

type failingRevoke struct{ approvalSessions }

func (failingRevoke) RevokeChecked(context.Context, string) error { return errors.New("store down") }

func TestNewDeviceApproval_RevokeFailureFailsClosed(t *testing.T) {
	h := newCodeHarness(t, true)
	h.passwordLogin()
	h.rt.approvalSessionsOverride = failingRevoke{h.rt.libSessions}
	rec := h.passwordLogin()
	if rec.Code != http.StatusInternalServerError || responseCookie(rec, sessionCookieName) != nil {
		t.Fatalf("login = %d %s, want 500 with no session", rec.Code, rec.Body.String())
	}
	pending, err := h.db.ListPendingLoginApprovalsForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending approvals = %d (%v), want none", len(pending), err)
	}
}

func TestTOTPRememberBrowser_OptIn(t *testing.T) {
	tests := []struct {
		name     string
		remember bool
	}{
		{"unchecked never trusts the browser", false},
		{"checked trusts the browser", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newMFAHarness(t)
			h.rt.newDeviceApproval = true
			h.enrollTOTP()
			token := h.mfaToken(h.passwordLogin())
			body := `{"mfa_token":"` + token + `","code":"` + h.code(0) + `","remember_device":` + strconv.FormatBool(tt.remember) + `}`
			rec := doJSON(t, h.rt, http.MethodPost, "/api/v1/auth/2fa/verify", body, nil)
			if rec.Code != http.StatusOK || !hasSessionCookie(rec) {
				t.Fatalf("verify = %d %s", rec.Code, rec.Body.String())
			}
			if got := responseCookie(rec, trustedCookie) != nil; got != tt.remember {
				t.Fatalf("trusted cookie set = %v, want %v", got, tt.remember)
			}
		})
	}
}

func TestAPIRateLimiter_KeysDoNotGrowWithoutBound(t *testing.T) {
	t.Run("idle buckets are swept", func(t *testing.T) {
		now := time.Now()
		l := newAPIRateLimiter(5)
		l.now = func() time.Time { return now }
		for i := range 1000 {
			l.allow("k" + strconv.Itoa(i))
		}
		now = now.Add(2 * time.Minute)
		l.allow("fresh")
		if n := l.keyCount(); n != 1 {
			t.Fatalf("keys = %d after a window idle, want 1", n)
		}
	})
	t.Run("a burst of distinct keys is capped", func(t *testing.T) {
		l := newAPIRateLimiter(5)
		l.maxKeys = 10
		for i := range 500 {
			l.allow("k" + strconv.Itoa(i))
		}
		if n := l.keyCount(); n > 10 {
			t.Fatalf("keys = %d, want at most 10", n)
		}
	})
}

func TestCodeLoginConfig_NonPositiveFallsBackToDefault(t *testing.T) {
	envs := []string{envLoginCodeTTL, envLoginCodeMaxAttempts, envLoginCodeIPRate, envLoginCodeAccountRate, envLoginCodeGlobalRate,
		envLoginCodeRedeemRate, envLoginCodeMinResponse, envDeviceApprovalTTL, envTrustedDeviceTTL, envLoginCodeMaxLive,
		envLoginCodeEmailRate, envDeviceApprovalRate}
	for _, v := range []string{"0", "-5", "0s", "-1m"} {
		t.Run(v, func(t *testing.T) {
			for _, e := range envs {
				t.Setenv(e, v)
			}
			s := newCodeLoginState()
			checks := []struct {
				name     string
				got, def float64
			}{
				{"ttl", s.ttl.Seconds(), defaultLoginCodeTTL.Seconds()},
				{"attempts", float64(s.maxAttempts), defaultLoginCodeAttempts},
				{"max live", float64(s.maxLive), defaultLoginCodeMaxLive},
				{"min response", s.minResponse.Seconds(), defaultLoginCodeMinResp.Seconds()},
				{"approval ttl", s.approvalTTL.Seconds(), defaultDeviceApprovalTTL.Seconds()},
				{"trust ttl", s.trustTTL.Seconds(), defaultTrustedDeviceTTL.Seconds()},
				{"ip rate", s.byIP.ratePerMinute, defaultLoginCodeIPRate},
				{"account rate", s.byAccount.ratePerMinute, defaultLoginCodeAcctRate},
				{"global rate", s.global.ratePerMinute, defaultLoginCodeGlobal},
				{"redeem rate", s.redeemByIP.ratePerMinute, defaultLoginCodeRedeem},
				{"email rate", s.emailByUser.ratePerMinute, defaultLoginCodeEmails},
				{"approval rate", s.approvals.ratePerMinute, defaultApprovalRate},
			}
			for _, c := range checks {
				if c.got != c.def {
					t.Errorf("%s = %v, want default %v", c.name, c.got, c.def)
				}
			}
		})
	}
}
