package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/descope/virtualwebauthn"
)

const (
	signInBeginRoute  = "/api/v1/auth/passkey-login/begin"
	signInFinishRoute = "/api/v1/auth/passkey-login/finish"
)

// discoverableAssertion starts a usernameless login and signs the challenge with k.
func (h *mfaHarness) discoverableAssertion(k softKey) (target, body string) {
	h.t.Helper()
	rec := doJSON(h.t, h.rt, http.MethodPost, signInBeginRoute, `{"discoverable":true}`, nil)
	if rec.Code != http.StatusOK {
		h.t.Fatalf("discoverable begin: %d %s", rec.Code, rec.Body.String())
	}
	var resp passkeyLoginBeginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		h.t.Fatal(err)
	}
	raw, _ := json.Marshal(resp.Options)
	opts, err := virtualwebauthn.ParseAssertionOptions(string(raw))
	if err != nil {
		h.t.Fatalf("parse assertion options: %v", err)
	}
	return signInFinishRoute + "?session_id=" + url.QueryEscape(resp.SessionID),
		virtualwebauthn.CreateAssertionResponse(h.rp(), k.auth, k.cred, *opts)
}

func TestDiscoverablePasskeyLogin(t *testing.T) {
	tests := []struct {
		name       string
		wantStatus int
		wantCookie bool
		run        func(t *testing.T, h *mfaHarness, k softKey) *httptest.ResponseRecorder
	}{
		{"success", http.StatusOK, true, func(_ *testing.T, h *mfaHarness, k softKey) *httptest.ResponseRecorder {
			k.cred.Counter = 1
			target, body := h.discoverableAssertion(k)
			return h.finishLogin(target, body)
		}},
		{"replayed challenge", http.StatusUnauthorized, false, func(t *testing.T, h *mfaHarness, k softKey) *httptest.ResponseRecorder {
			k.cred.Counter = 1
			target, body := h.discoverableAssertion(k)
			if rec := h.finishLogin(target, body); rec.Code != http.StatusOK {
				t.Fatalf("first use: %d", rec.Code)
			}
			return h.finishLogin(target, body)
		}},
		{"unknown credential", http.StatusUnauthorized, false, func(_ *testing.T, h *mfaHarness, _ softKey) *httptest.ResponseRecorder {
			stranger := softKey{cred: virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2), auth: virtualwebauthn.NewAuthenticator()}
			stranger.auth.Options.UserHandle = []byte("someone-else")
			stranger.auth.AddCredential(stranger.cred)
			target, body := h.discoverableAssertion(stranger)
			return h.finishLogin(target, body)
		}},
		{"disabled user", http.StatusUnauthorized, false, func(t *testing.T, h *mfaHarness, k softKey) *httptest.ResponseRecorder {
			if err := h.rt.authEngine.DisableUser(context.Background(), h.userID); err != nil {
				t.Fatalf("disable: %v", err)
			}
			k.cred.Counter = 1
			target, body := h.discoverableAssertion(k)
			return h.finishLogin(target, body)
		}},
	}
	var failureBody string
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newMFAHarness(t)
			k := h.registerPasskey("Laptop")
			rec := tc.run(t, h, k)
			if rec.Code != tc.wantStatus || hasSessionCookie(rec) != tc.wantCookie {
				t.Fatalf("status = %d cookie = %v, want %d %v: %s", rec.Code, hasSessionCookie(rec), tc.wantStatus, tc.wantCookie, rec.Body.String())
			}
			if tc.wantStatus != http.StatusUnauthorized {
				return
			}
			if failureBody == "" {
				failureBody = withoutRequestID(rec.Body.String())
			} else if withoutRequestID(rec.Body.String()) != failureBody {
				t.Fatalf("failure body %q differs from %q, which leaks the reason", rec.Body.String(), failureBody)
			}
		})
	}
}

func TestDiscoverableBeginRateLimitAndInputs(t *testing.T) {
	h := newMFAHarness(t)

	t.Run("an empty body without the flag is still refused", func(t *testing.T) {
		if rec := doJSON(t, h.rt, http.MethodPost, signInBeginRoute, `{}`, nil); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("repeated failed finishes lock the client out", func(t *testing.T) {
		var last *httptest.ResponseRecorder
		for i := 0; i < 8; i++ {
			last = doJSON(t, h.rt, http.MethodPost, signInFinishRoute+"?session_id=nope", `{}`, nil)
		}
		if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
			t.Fatalf("status = %d retry-after = %q, want 429 with Retry-After", last.Code, last.Header().Get("Retry-After"))
		}
	})
}

func TestPasskeyRegistrationPrefersResidentKey(t *testing.T) {
	h := newMFAHarness(t)
	rec := h.do(http.MethodPost, "/api/v1/auth/passkeys/register/begin", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("register begin: %d %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"residentKey":"preferred"`) || !strings.Contains(body, `"requireResidentKey":false`) {
		t.Fatalf("options do not ask for a preferred resident key: %s", body)
	}
}
