package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/store"
)

// liveCodeID requests a code for the harness user and returns its id.
func (h *codeHarness) liveCodeID() string {
	h.t.Helper()
	h.requestCode(codeTestEmail)
	codes, err := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(codes) == 0 {
		h.t.Fatalf("no live code: %v", err)
	}
	return codes[0].ID
}

// pendingApprovalID signs in once, then pauses a second password login.
func (h *codeHarness) pendingApprovalID() string {
	h.t.Helper()
	h.passwordLogin()
	id, _ := h.pendingApproval(h.passwordLogin())
	return id
}

func TestSignInApprove_TokenCannotEscalateToSession(t *testing.T) {
	tests := []struct {
		name      string
		abilities []string
		want      bool
	}{
		{"write:sensitive token", []string{AbilityWriteSensitive}, false},
		{"root token", []string{AbilityRoot}, false},
		{"device login token", authengine.EngineAbilities(), false},
		{"signin:approve token", []string{AbilitySignInApprove}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newCodeHarness(t, true)
			tok := &store.APIToken{ID: "tok_" + strconv.Itoa(len(tt.abilities)), OwnerUserID: h.user.ID, Abilities: tt.abilities}

			req := httptestRequestWithToken(http.MethodPost, "/api/v1/auth/sign-in-requests/codes/x/reveal", tok)
			req.SetPathValue("id", h.liveCodeID())
			rec := newRecorder()
			h.rt.handleRevealLoginCode(rec, req)
			if got := rec.Code == http.StatusOK; got != tt.want {
				t.Fatalf("reveal status = %d (%s), want allowed=%v", rec.Code, rec.Body.String(), tt.want)
			}

			id := h.pendingApprovalID()
			a, err := h.db.GetLoginApproval(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			match, _ := approvalMatch(a.BrowserHash)
			req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login-approvals/x/approve", strings.NewReader(`{"match":`+strconv.Itoa(match)+`}`))
			req = req.WithContext(withTokenIdentity(req.Context(), tok))
			req.SetPathValue("id", id)
			rec = newRecorder()
			h.rt.handleApproveLoginApproval(rec, req)
			if got := rec.Code == http.StatusNoContent; got != tt.want {
				t.Fatalf("approve status = %d (%s), want allowed=%v", rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestSignInApprove_MintedOnlyFromASession(t *testing.T) {
	h := newCodeHarness(t, false)
	session := h.userSession()
	rec := h.send(http.MethodPost, "/api/v1/auth/tokens", `{"name":"approver","abilities":["`+AbilitySignInApprove+`"]}`, "", session)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint from session = %d %s", rec.Code, rec.Body.String())
	}
	var minted createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &minted); err != nil {
		t.Fatal(err)
	}
	id := h.liveCodeID()
	bearer := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+minted.Token)
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(out, req)
		return out
	}
	if got := bearer(http.MethodPost, "/api/v1/auth/sign-in-requests/codes/"+id+"/reveal", ""); got.Code != http.StatusOK {
		t.Fatalf("reveal with a signin:approve token = %d %s", got.Code, got.Body.String())
	}
	if got := bearer(http.MethodPost, "/api/v1/auth/tokens", `{"name":"again","abilities":["`+AbilitySignInApprove+`"]}`); got.Code == http.StatusCreated {
		t.Fatal("a token must not mint another signin:approve token")
	}
	userRec := h.send(http.MethodPut, "/api/v1/users/"+h.user.ID+"/abilities", `{"abilities":["`+AbilitySignInApprove+`"]}`, "",
		loginTestSession(t, h.rt, h.db))
	if userRec.Code != http.StatusBadRequest {
		t.Fatalf("a user must never hold signin:approve: %d %s", userRec.Code, userRec.Body.String())
	}
}

func TestLoginCodeRequest_RejectionsDoNotDrainOtherBudgets(t *testing.T) {
	tests := []struct {
		name  string
		env   map[string]string
		drain func(h *codeHarness)
		final func(h *codeHarness) int
	}{
		{"per-IP rejections leave the global budget", map[string]string{envLoginCodeGlobalRate: "6"}, func(h *codeHarness) {
			for i := range 15 {
				h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"ip`+strconv.Itoa(i)+`@example.com"}`, codeTestIP)
			}
		}, func(h *codeHarness) int {
			return h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"other@example.com"}`, "203.0.113.9:1").Code
		}},
		{"per-account rejections leave the per-IP budget", nil, func(h *codeHarness) {
			for range 5 {
				h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"same@example.com"}`, codeTestIP)
			}
		}, func(h *codeHarness) int {
			return h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"fresh@example.com"}`, codeTestIP).Code
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			h := newCodeHarness(t, false)
			tt.drain(h)
			if got := tt.final(h); got != http.StatusAccepted {
				t.Fatalf("status = %d, want 202", got)
			}
		})
	}
}

func (h *codeHarness) mailCount(settle time.Duration) int {
	h.t.Helper()
	time.Sleep(settle)
	h.mail.mu.Lock()
	defer h.mail.mu.Unlock()
	return len(h.mail.bodies)
}

func TestLoginCodeRequest_FloodIsBounded(t *testing.T) {
	t.Run("live codes are capped per account and only one email goes out", func(t *testing.T) {
		t.Setenv(envLoginCodeAccountRate, "50")
		h := newCodeHarness(t, false)
		var first string
		for i := range 6 {
			rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "198.51.100."+strconv.Itoa(i+1)+":1")
			if i == 0 {
				first = rec.Body.String()
			}
			if rec.Code != http.StatusAccepted || rec.Body.String() != first {
				t.Fatalf("request %d = %d %s, want the same 202", i, rec.Code, rec.Body.String())
			}
		}
		live, err := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
		if err != nil || len(live) != defaultLoginCodeMaxLive {
			t.Fatalf("live codes = %d (%v), want %d", len(live), err, defaultLoginCodeMaxLive)
		}
		if n := h.mailCount(200 * time.Millisecond); n != 1 {
			t.Fatalf("emails = %d, want 1 while a code is live", n)
		}
		var feed attentionFeedResponse
		if err := json.Unmarshal(h.send(http.MethodGet, "/api/v1/attention/feed", "", "", h.userSession()).Body.Bytes(), &feed); err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, it := range feed.Items {
			if it.Kind == "login_code" {
				n++
				if it.Params["count"] != strconv.Itoa(defaultLoginCodeMaxLive) {
					t.Fatalf("collapsed item count = %q", it.Params["count"])
				}
			}
		}
		if n != 1 {
			t.Fatalf("login_code attention items = %d, want 1", n)
		}
	})
	t.Run("emails per account per hour are throttled", func(t *testing.T) {
		t.Setenv(envLoginCodeAccountRate, "50")
		t.Setenv(envLoginCodeEmailRate, "2")
		h := newCodeHarness(t, false)
		for i := range 4 {
			h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "198.51.100."+strconv.Itoa(i+1)+":1")
			if _, err := h.db.ExecContext(context.Background(), `UPDATE login_code_challenges SET expires_at = ?`, store.FormatAuditTime(time.Now().Add(-time.Second))); err != nil {
				t.Fatal(err)
			}
		}
		if n := h.mailCount(200 * time.Millisecond); n != 2 {
			t.Fatalf("emails = %d, want 2", n)
		}
	})
	t.Run("a stranger draining the account bucket cannot block the owner's own browser", func(t *testing.T) {
		h := newCodeHarness(t, false)
		for i := range defaultLoginCodeAcctRate + 2 {
			h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "198.51.100."+strconv.Itoa(i+1)+":1")
		}
		rec := h.send(http.MethodPost, "/api/v1/auth/login-code/request", `{"username":"`+codeTestEmail+`"}`, "203.0.113.50:1", h.userSession())
		if rec.Code != http.StatusAccepted {
			t.Fatalf("owner request = %d %s, want 202", rec.Code, rec.Body.String())
		}
	})
}

func TestNewDeviceApproval_PromptFatigue(t *testing.T) {
	t.Run("a new request supersedes the old one", func(t *testing.T) {
		h := newCodeHarness(t, true)
		h.passwordLogin()
		_, firstBinding := h.pendingApproval(h.passwordLogin())
		h.pendingApproval(h.passwordLogin())
		pending, err := h.db.ListPendingLoginApprovalsForUser(context.Background(), h.user.ID, time.Now())
		if err != nil || len(pending) != 1 {
			t.Fatalf("pending approvals = %d (%v), want 1", len(pending), err)
		}
		if out, _ := h.poll(firstBinding); out.Status != approvalPollExpired {
			t.Fatalf("superseded browser polls %q, want expired", out.Status)
		}
		if !containsAction(h.auditActions(), store.AuditActionNewDeviceReplace) {
			t.Fatalf("audit %v lacks %s", h.auditActions(), store.AuditActionNewDeviceReplace)
		}
	})
	t.Run("approval creation is rate limited per account", func(t *testing.T) {
		t.Setenv(envDeviceApprovalRate, "2")
		h := newCodeHarness(t, true)
		h.passwordLogin()
		h.pendingApproval(h.passwordLogin())
		h.pendingApproval(h.passwordLogin())
		if rec := h.passwordLogin(); rec.Code != http.StatusTooManyRequests {
			t.Fatalf("third approval = %d %s, want 429", rec.Code, rec.Body.String())
		}
	})
	t.Run("the waiting browser shows one of the approver's three numbers", func(t *testing.T) {
		h := newCodeHarness(t, true)
		existing := responseCookie(h.passwordLogin(), sessionCookieName)
		rec := h.passwordLogin()
		var resp loginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.ApprovalMatch < 10 || resp.ApprovalMatch > 99 {
			t.Fatalf("approval_match = %d (%s)", resp.ApprovalMatch, rec.Body.String())
		}
		var list signInRequestsResponse
		if err := json.Unmarshal(h.send(http.MethodGet, "/api/v1/auth/sign-in-requests", "", "", existing).Body.Bytes(), &list); err != nil || len(list.Approvals) != 1 {
			t.Fatalf("list: %v %+v", err, list)
		}
		opts := list.Approvals[0].MatchOptions
		if len(opts) != 3 || opts[0] == opts[1] || opts[1] == opts[2] || opts[0] == opts[2] {
			t.Fatalf("options = %v, want 3 distinct", opts)
		}
		found := false
		for _, o := range opts {
			found = found || o == resp.ApprovalMatch
		}
		if !found {
			t.Fatalf("options %v lack the shown number %d", opts, resp.ApprovalMatch)
		}
	})
}

func TestSignInContext_NeverCarriesRawUserAgent(t *testing.T) {
	const hostile = "Mozilla/5.0 (Windows NT 10.0; Win64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36 visit http://evil.example/reset\r\nBcc: attacker@example.net"
	h := newCodeHarness(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login-code/request", strings.NewReader(`{"username":"`+codeTestEmail+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", hostile)
	req.RemoteAddr = codeTestIP
	h.rt.Handler().ServeHTTP(httptest.NewRecorder(), req)
	var mailed string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && mailed == ""; time.Sleep(10 * time.Millisecond) {
		mailed = h.mail.all()
	}
	feed := h.send(http.MethodGet, "/api/v1/attention/feed", "", "", h.userSession()).Body.String()
	list := h.send(http.MethodGet, "/api/v1/auth/sign-in-requests", "", "", h.userSession()).Body.String()
	for name, surface := range map[string]string{"email": mailed, "attention feed": feed, "sign-in requests": list} {
		for _, bad := range []string{"evil.example", "Bcc", "attacker@", "AppleWebKit"} {
			if strings.Contains(surface, bad) {
				t.Fatalf("%s carries %q: %s", name, bad, surface)
			}
		}
		if !strings.Contains(surface, "Chrome on Windows") {
			t.Fatalf("%s lacks the derived label: %s", name, surface)
		}
	}
}

func TestBrowserLabelAndSafeText(t *testing.T) {
	tests := []struct {
		name, in, want string
		fn             func(string) string
	}{
		{"firefox on linux", "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0", "Firefox on Linux", browserLabel},
		{"safari on ios", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 Version/17.0 Mobile/15E148 Safari/604.1", "Safari on iOS", browserLabel},
		{"edge on windows", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120 Safari/537.36 Edg/120", "Edge on Windows", browserLabel},
		{"free text", "click http://x.example now", unknownBrowserLabel, browserLabel},
		{"strips urls and newlines", "Chrome on\nWindows http://x.example/a", "Chrome on Windows", safeSignInText},
		{"caps length", strings.Repeat("a", 200), strings.Repeat("a", maxSignInTextLen), safeSignInText},
		{"bad ip", "1.2.3.4\nX: y", "unknown address", safeSignInIP},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fn(tt.in); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPublicSignInPOSTs_RefuseCrossSite(t *testing.T) {
	routes := []string{"/api/v1/auth/login-code/request", "/api/v1/auth/login-code/redeem", "/api/v1/auth/login-approval/poll"}
	cases := []struct {
		name    string
		ctype   string
		headers map[string]string
		want    int
	}{
		{"form post", "application/x-www-form-urlencoded", nil, http.StatusUnsupportedMediaType},
		{"text plain", "text/plain", nil, http.StatusUnsupportedMediaType},
		{"cross-site origin", "application/json", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"cross-site fetch metadata", "application/json", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
	}
	h := newCodeHarness(t, false)
	for _, path := range routes {
		for _, c := range cases {
			t.Run(path+" "+c.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"username":"x@example.com","code":"AAAA-AAAA"}`))
				req.Header.Set("Content-Type", c.ctype)
				for k, v := range c.headers {
					req.Header.Set(k, v)
				}
				rec := httptest.NewRecorder()
				h.rt.Handler().ServeHTTP(rec, req)
				if rec.Code != c.want {
					t.Fatalf("status = %d, want %d", rec.Code, c.want)
				}
			})
		}
	}
}

func TestLoginCode_PepperedHash(t *testing.T) {
	pepper := []byte("0123456789abcdef0123456789abcdef")
	if hashLoginCode(pepper, "salt", "ABCDEFGH") == hashLoginCode(nil, "salt", "ABCDEFGH") {
		t.Fatal("a peppered hash must differ from the salt-only hash")
	}
	h := newCodeHarness(t, false)
	WithLoginCodeKey(pepper)(h.rt)
	_, binding := h.requestCode(codeTestEmail)
	code := h.revealFirst(h.userSession())
	codes, err := h.db.ListLiveLoginCodesForUser(context.Background(), h.user.ID, time.Now())
	if err != nil || len(codes) != 1 {
		t.Fatal(err)
	}
	if codes[0].CodeHash == hashLoginCode(nil, codes[0].Salt, normalizeLoginCode(code)) {
		t.Fatal("stored hash is reproducible from the row alone")
	}
	if rec := h.redeem(code, binding); rec.Code != http.StatusOK {
		t.Fatalf("redeem with pepper = %d %s", rec.Code, rec.Body.String())
	}
}
