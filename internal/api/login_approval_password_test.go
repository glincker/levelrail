package api

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

const loopbackTestIP = "127.0.0.1:4000"

func (h *codeHarness) loginFrom(remote string) (string, *http.Cookie) {
	h.t.Helper()
	rec := h.send(http.MethodPost, "/api/v1/auth/login",
		`{"username":"`+codeTestEmail+`","password":"`+codeTestPassword+`"}`, remote)
	return h.pendingApproval(rec)
}

func TestApproveLoginWithPassword(t *testing.T) {
	tests := []struct {
		name       string
		requester  string
		body       func(h *codeHarness, id string) string
		wantDecide int
		wantPoll   string
	}{
		{"password approves a sign-in from this computer", loopbackTestIP, func(*codeHarness, string) string {
			return `{"step_up":{"password":"` + codeTestPassword + `"}}`
		}, http.StatusNoContent, approvalPollApproved},
		{"a wrong password is refused and leaves it pending", loopbackTestIP, func(*codeHarness, string) string {
			return `{"step_up":{"password":"not-the-password"}}`
		}, http.StatusForbidden, approvalPollPending},
		{"the password is refused for a sign-in from elsewhere", codeTestIP, func(*codeHarness, string) string {
			return `{"step_up":{"password":"` + codeTestPassword + `"}}`
		}, http.StatusForbidden, approvalPollPending},
		{"a wrong number still denies a sign-in from this computer", loopbackTestIP, func(h *codeHarness, id string) string {
			a, err := h.db.GetLoginApproval(t.Context(), id)
			if err != nil {
				h.t.Fatal(err)
			}
			wrong := approvalMatch(a.BrowserHash) + 1
			if wrong > 99 {
				wrong = 10
			}
			return `{"match":` + strconv.Itoa(wrong) + `}`
		}, http.StatusConflict, approvalPollDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			first := h.passwordLogin()
			existing := responseCookie(first, sessionCookieName)
			if first.Code != http.StatusOK || existing == nil {
				t.Fatalf("first login must go straight through: %d %s", first.Code, first.Body.String())
			}
			id, binding := h.loginFrom(tt.requester)
			rec := h.send(http.MethodPost, "/api/v1/auth/login-approvals/"+id+"/approve", tt.body(h, id), "", existing)
			if rec.Code != tt.wantDecide {
				t.Fatalf("approve = %d (%s), want %d", rec.Code, rec.Body.String(), tt.wantDecide)
			}
			out, pollRec := h.poll(binding)
			if out.Status != tt.wantPoll {
				t.Fatalf("poll = %q (%s), want %q", out.Status, pollRec.Body.String(), tt.wantPoll)
			}
			if tt.wantPoll == approvalPollApproved && !containsAction(h.auditActions(), store.AuditActionNewDeviceApprove) {
				t.Fatalf("audit actions %v lack %s", h.auditActions(), store.AuditActionNewDeviceApprove)
			}
		})
	}
}
