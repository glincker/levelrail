package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func (h *codeHarness) pendingApproval(rec *httptest.ResponseRecorder) (string, *http.Cookie) {
	h.t.Helper()
	var resp loginResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &resp) != nil || !resp.ApprovalRequired {
		h.t.Fatalf("login must pause for approval: %d %s", rec.Code, rec.Body.String())
	}
	if responseCookie(rec, sessionCookieName) != nil {
		h.t.Fatal("a paused login must not set a session cookie")
	}
	return resp.ApprovalID, responseCookie(rec, approvalCookie)
}

func (h *codeHarness) poll(binding *http.Cookie) (loginApprovalPollResponse, *httptest.ResponseRecorder) {
	h.t.Helper()
	rec := h.send(http.MethodPost, "/api/v1/auth/login-approval/poll", "", codeTestIP, binding)
	var out loginApprovalPollResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, rec
}

func TestNewDeviceApproval(t *testing.T) {
	tests := []struct {
		name       string
		decide     func(h *codeHarness, id string, existing *http.Cookie) int
		wantDecide int
		wantPoll   string
		wantAudit  string
	}{
		{"existing session approves with the matching number", func(h *codeHarness, id string, s *http.Cookie) int {
			return h.approve(id, s).Code
		}, http.StatusNoContent, approvalPollApproved, store.AuditActionNewDeviceApprove},
		{"existing session denies", func(h *codeHarness, id string, s *http.Cookie) int {
			return h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/deny", "", "", s).Code
		}, http.StatusNoContent, approvalPollDenied, store.AuditActionNewDeviceDeny},
		{"another account cannot approve", func(h *codeHarness, id string, _ *http.Cookie) int {
			admin := loginTestSession(h.t, h.rt, h.db)
			return h.approve(id, admin).Code
		}, http.StatusNotFound, approvalPollPending, ""},
		{"approve without a number is refused and leaves it pending", func(h *codeHarness, id string, s *http.Cookie) int {
			return h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/approve", "{}", "", s).Code
		}, http.StatusBadRequest, approvalPollPending, ""},
		{"a number that is not two digits is refused and leaves it pending", func(h *codeHarness, id string, s *http.Cookie) int {
			return h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/approve", `{"match":7}`, "", s).Code
		}, http.StatusBadRequest, approvalPollPending, ""},
		{"a wrong number denies the sign-in", func(h *codeHarness, id string, s *http.Cookie) int {
			a, err := h.db.GetLoginApproval(context.Background(), id)
			if err != nil {
				h.t.Fatal(err)
			}
			wrong := approvalMatch(a.BrowserHash) + 1
			if wrong > 99 {
				wrong = 10
			}
			return h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/approve", `{"match":`+strconv.Itoa(wrong)+`}`, "", s).Code
		}, http.StatusConflict, approvalPollDenied, store.AuditActionNewDeviceDeny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			first := h.passwordLogin()
			existing := responseCookie(first, sessionCookieName)
			if first.Code != http.StatusOK || existing == nil {
				t.Fatalf("first login with no other session must go straight through: %d %s", first.Code, first.Body.String())
			}
			id, binding := h.pendingApproval(h.passwordLogin())
			if got := tt.decide(h, id, existing); got != tt.wantDecide {
				t.Fatalf("decide status = %d, want %d", got, tt.wantDecide)
			}
			out, rec := h.poll(binding)
			if out.Status != tt.wantPoll {
				t.Fatalf("poll = %q (%s), want %q", out.Status, rec.Body.String(), tt.wantPoll)
			}
			gotSession := responseCookie(rec, sessionCookieName) != nil
			if gotSession != (tt.wantPoll == approvalPollApproved) {
				t.Fatalf("session cookie issued = %v for poll status %q", gotSession, out.Status)
			}
			if tt.wantPoll == approvalPollApproved {
				if again, _ := h.poll(binding); again.Status == approvalPollApproved {
					t.Fatal("an approval must issue a session only once")
				}
			}
			if tt.wantAudit != "" && !containsAction(h.auditActions(), tt.wantAudit) {
				t.Fatalf("audit actions %v lack %s", h.auditActions(), tt.wantAudit)
			}
		})
	}
}

func TestTrustedDeviceSkipsApprovalUntilRevoked(t *testing.T) {
	h := newCodeHarness(t, true)
	existing := responseCookie(h.passwordLogin(), sessionCookieName)
	id, binding := h.pendingApproval(h.passwordLogin())
	if rec := h.approve(id, existing); rec.Code != http.StatusNoContent {
		t.Fatalf("approve = %d", rec.Code)
	}
	_, rec := h.poll(binding)
	trusted := responseCookie(rec, trustedCookie)
	if trusted == nil || !trusted.HttpOnly || trusted.SameSite != http.SameSiteStrictMode {
		t.Fatalf("trusted cookie = %+v", trusted)
	}

	steps := []struct {
		name         string
		revokeFirst  bool
		wantApproval bool
	}{
		{"trusted browser signs in directly", false, false},
		{"revoked browser must be approved again", true, true},
	}
	for _, s := range steps {
		t.Run(s.name, func(t *testing.T) {
			if s.revokeFirst {
				var list trustedDevicesResponse
				lr := h.send(http.MethodGet, "/api/v1/auth/trusted-devices", "", "", existing, trusted)
				if err := json.Unmarshal(lr.Body.Bytes(), &list); err != nil || len(list.Devices) != 1 || !list.Devices[0].Current {
					t.Fatalf("list = %s", lr.Body.String())
				}
				if rr := h.send(http.MethodDelete, "/api/v1/auth/trusted-devices/"+list.Devices[0].ID, "", "", existing); rr.Code != http.StatusNoContent {
					t.Fatalf("revoke = %d %s", rr.Code, rr.Body.String())
				}
			}
			login := h.passwordLogin(trusted)
			var resp loginResponse
			if err := json.Unmarshal(login.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.ApprovalRequired != s.wantApproval {
				t.Fatalf("approval required = %v, want %v (%s)", resp.ApprovalRequired, s.wantApproval, login.Body.String())
			}
		})
	}
	if !containsAction(h.auditActions(), store.AuditActionTrustedDeviceDrop) {
		t.Fatalf("audit actions %v lack the revoke", h.auditActions())
	}
}

func TestNewDeviceApproval_Off(t *testing.T) {
	h := newCodeHarness(t, false)
	h.passwordLogin()
	if rec := h.passwordLogin(); responseCookie(rec, sessionCookieName) == nil {
		t.Fatalf("with approval off a second login must sign in: %d %s", rec.Code, rec.Body.String())
	}
}
