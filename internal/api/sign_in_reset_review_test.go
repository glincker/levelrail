package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func (h *codeHarness) mintToken(session *http.Cookie, body string) createTokenResponse {
	h.t.Helper()
	rec := h.send(http.MethodPost, "/api/v1/auth/tokens", body, "", session)
	var out createTokenResponse
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		h.t.Fatalf("mint %s = %d %s", body, rec.Code, rec.Body.String())
	}
	return out
}

func (h *codeHarness) tokenRevoked(id string) bool {
	h.t.Helper()
	rec, err := h.rt.authLib.tokens.GetToken(context.Background(), id)
	if err != nil {
		h.t.Fatal(err)
	}
	return rec.RevokedAt != nil
}

func (h *codeHarness) resetPassword() int {
	h.t.Helper()
	h.send(http.MethodPost, "/api/v1/auth/forgot-password", `{"email":"`+codeTestEmail+`"}`, "")
	var body string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && !strings.Contains(body, "token="); time.Sleep(10 * time.Millisecond) {
		body = h.mail.all()
	}
	token := tokenFromResetEmail(h.t, body)
	return h.send(http.MethodPost, "/api/v1/auth/reset-password", `{"token":"`+token+`","new_password":"another-long-password"}`, "").Code
}

func TestCredentialReset_RetiresSignInGrants(t *testing.T) {
	resets := []struct {
		name string
		run  func(h *codeHarness, session *http.Cookie) int
	}{
		{"password change", func(h *codeHarness, s *http.Cookie) int {
			return h.send(http.MethodPut, "/api/v1/auth/password", `{"current_password":"`+codeTestPassword+`","new_password":"another-long-password"}`, "", s).Code
		}},
		{"revoke other sessions", func(h *codeHarness, s *http.Cookie) int {
			return h.send(http.MethodPost, "/api/v1/auth/sessions/revoke-others", "", "", s).Code
		}},
		{"password reset", func(h *codeHarness, _ *http.Cookie) int { return h.resetPassword() }},
	}
	for _, reset := range resets {
		for _, approved := range []bool{false, true} {
			t.Run(reset.name+" approved="+strconv.FormatBool(approved), func(t *testing.T) {
				h := newCodeHarness(t, true)
				session := responseCookie(h.passwordLogin(), sessionCookieName)
				approver := h.mintToken(session, `{"name":"approver","abilities":["`+AbilitySignInApprove+`"],"expires_in_days":7}`)
				reader := h.mintToken(session, `{"name":"reader","abilities":["`+AbilityRead+`"]}`)
				id, approvalBinding := h.pendingApproval(h.passwordLogin())
				if approved {
					if rec := h.approve(id, session); rec.Code != http.StatusNoContent {
						t.Fatalf("approve = %d", rec.Code)
					}
				}
				_, codeBinding := h.requestCode(codeTestEmail)
				code := h.revealFirst(session)
				codes, err := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
				if err != nil || len(codes) != 1 {
					t.Fatalf("live codes = %d (%v)", len(codes), err)
				}

				if got := reset.run(h, session); got != http.StatusNoContent {
					t.Fatalf("reset = %d, want 204", got)
				}

				if out, rec := h.poll(approvalBinding); out.Status == approvalPollApproved || out.Status == approvalPollPending || responseCookie(rec, sessionCookieName) != nil {
					t.Fatalf("approval after reset = %q (%s), want retired", out.Status, rec.Body.String())
				}
				if rec := h.redeem(code, codeBinding); rec.Code != http.StatusUnauthorized {
					t.Fatalf("code redeem after reset = %d %s, want 401", rec.Code, rec.Body.String())
				}
				if _, ok := h.rt.codeLogin.lookup(codes[0].ID, h.user.ID, time.Now()); ok {
					t.Fatal("plaintext code still held after reset")
				}
				if !h.tokenRevoked(approver.ID) {
					t.Fatal("signin:approve token survived the reset")
				}
				if h.tokenRevoked(reader.ID) {
					t.Fatal("an unrelated token was revoked")
				}
			})
		}
	}
}

func TestSecondFactorRemoval_RetiresSignInRequests(t *testing.T) {
	tests := []struct {
		name string
		run  func(h *mfaHarness) int
	}{
		{"TOTP disable", func(h *mfaHarness) int {
			h.enrollTOTP()
			return h.do(http.MethodPost, "/api/v1/auth/2fa/disable", `{"code":"`+h.code(30*time.Second)+`"}`).Code
		}},
		{"passkey removal", func(h *mfaHarness) int {
			h.registerPasskey("laptop")
			var rows []passkeyResource
			if err := json.Unmarshal(h.do(http.MethodGet, "/api/v1/auth/passkeys", "").Body.Bytes(), &rows); err != nil || len(rows) != 1 {
				h.t.Fatalf("passkeys = %v (%v)", rows, err)
			}
			return h.do(http.MethodDelete, "/api/v1/auth/passkeys/"+rows[0].ID, "").Code
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newMFAHarness(t)
			ctx := context.Background()
			now := time.Now()
			if err := h.db.CreateLoginApproval(ctx, store.LoginApproval{ID: "la_1", UserID: h.userID, BrowserHash: "b1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			if _, err := h.db.DecideLoginApproval(ctx, "la_1", h.userID, true, "session:x", now); err != nil {
				t.Fatal(err)
			}
			if err := h.db.CreateLoginCodeChallenge(ctx, store.LoginCodeChallenge{ID: "lc_1", UserID: h.userID, BrowserHash: "b2", CodeHash: "h", Salt: "s",
				CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			h.rt.codeLogin.remember("lc_1", pendingPlainCode{userID: h.userID, code: "ABCDEFGH", expires: now.Add(time.Hour)})

			if got := tt.run(h); got != http.StatusNoContent {
				t.Fatalf("status = %d, want 204", got)
			}
			if ok, err := h.db.ConsumeLoginApproval(ctx, "la_1", time.Now()); err != nil || ok {
				t.Fatalf("approved sign-in still collectable: %v %v", ok, err)
			}
			if live, _ := h.db.ListLiveLoginCodesForUser(ctx, h.userID, time.Now()); len(live) != 0 {
				t.Fatalf("live codes = %d, want 0", len(live))
			}
			if _, ok := h.rt.codeLogin.lookup("lc_1", h.userID, time.Now()); ok {
				t.Fatal("plaintext code still held")
			}
		})
	}
}

func TestSignInApproveToken_MintRules(t *testing.T) {
	tests := []struct {
		name string
		env  string
		body string
		want int
	}{
		{"no expiry is refused", "", `{"name":"a","abilities":["` + AbilitySignInApprove + `"]}`, http.StatusBadRequest},
		{"past the default 30 days is refused", "", `{"name":"a","abilities":["` + AbilitySignInApprove + `"],"expires_in_days":31}`, http.StatusBadRequest},
		{"30 days is allowed", "", `{"name":"a","abilities":["` + AbilitySignInApprove + `"],"expires_in_days":30}`, http.StatusCreated},
		{"the env cap applies", "7", `{"name":"a","abilities":["` + AbilitySignInApprove + `"],"expires_in_days":8}`, http.StatusBadRequest},
		{"an agent never gets it", "", `{"name":"a","abilities":["` + AbilitySignInApprove + `"],"expires_in_days":1,"agent":{"name":"bot"}}`, http.StatusBadRequest},
		{"other abilities may still never expire", "", `{"name":"a","abilities":["` + AbilityRead + `"]}`, http.StatusCreated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv(envSignInApproveTokenMaxDays, tt.env)
			}
			h := newCodeHarness(t, false)
			if rec := h.send(http.MethodPost, "/api/v1/auth/tokens", tt.body, "", h.userSession()); rec.Code != tt.want {
				t.Fatalf("status = %d %s, want %d", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestLoginCodeRequest_CapCannotBeSquatted(t *testing.T) {
	stranger := func(h *codeHarness, i int) {
		h.t.Helper()
		if rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "198.51.100."+strconv.Itoa(i+1)+":1"); rec.Code != http.StatusAccepted {
			h.t.Fatalf("stranger request %d = %d", i, rec.Code)
		}
	}
	t.Run("at the cap a stranger's request expires the oldest code, not the new one", func(t *testing.T) {
		t.Setenv(envLoginCodeAccountRate, "50")
		h := newCodeHarness(t, false)
		for i := range defaultLoginCodeMaxLive {
			stranger(h, i)
		}
		before, _ := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
		oldest := before[len(before)-1].ID
		stranger(h, defaultLoginCodeMaxLive)
		after, _ := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
		if len(after) != defaultLoginCodeMaxLive {
			t.Fatalf("live = %d, want %d", len(after), defaultLoginCodeMaxLive)
		}
		for _, c := range after {
			if c.ID == oldest {
				t.Fatal("the oldest code is still live")
			}
		}
		if after[0].ID == before[0].ID {
			t.Fatal("the newest request did not get a live code")
		}
	})
	t.Run("the proven owner skips the cap and the one-email rule", func(t *testing.T) {
		t.Setenv(envLoginCodeAccountRate, "50")
		h := newCodeHarness(t, false)
		for i := range defaultLoginCodeMaxLive + 2 {
			stranger(h, i)
		}
		session := h.userSession()
		for range 2 {
			if rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "203.0.113.50:1", session); rec.Code != http.StatusAccepted {
				t.Fatalf("owner request = %d", rec.Code)
			}
		}
		live, _ := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
		if len(live) != defaultLoginCodeMaxLive+2 {
			t.Fatalf("live = %d, want %d (strangers capped, owner's two kept)", len(live), defaultLoginCodeMaxLive+2)
		}
		for _, c := range live[:2] {
			if !h.rt.codeLogin.owned(c.ID) {
				t.Fatalf("newest code %s is not the owner's", c.ID)
			}
		}
		stranger(h, 20)
		if live, _ = h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now()); !h.rt.codeLogin.owned(live[1].ID) || !h.rt.codeLogin.owned(live[2].ID) {
			t.Fatal("a stranger at the cap expired an owner's code")
		}
		if n := h.mailCount(200 * time.Millisecond); n != 3 {
			t.Fatalf("emails = %d, want 3 (first stranger plus both owner requests)", n)
		}
	})
}

func TestNewDeviceApproval_ConcurrentLoginsLeaveOnePending(t *testing.T) {
	h := newCodeHarness(t, true)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
			h.rt.createLoginApproval(httptest.NewRecorder(), req, h.user)
		}()
	}
	close(start)
	wg.Wait()
	pending, err := h.db.ListPendingLoginApprovalsForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending approvals = %d (%v), want exactly 1", len(pending), err)
	}
}
