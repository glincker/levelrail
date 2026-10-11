package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"github.com/GLINCKER/levelrail/internal/store"
)

func seedRevealApp(t *testing.T, db *store.DB) {
	t.Helper()
	err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 3000, Env: map[string]string{"API_SECRET": plainSecretValue}})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStepUp_TOTPAccountNeedsPasswordAndCode(t *testing.T) {
	h := newMFAHarness(t)
	seedRevealApp(t, h.db)
	h.enrollTOTP()

	rec := h.do(http.MethodGet, "/api/v1/auth/step-up", "")
	var opts stepUpOptionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil || !opts.CodeRequired {
		t.Fatalf("options = %s", rec.Body.String())
	}

	tests := []struct {
		name, body, code string
		want             int
	}{
		{"password only", stepUpBody(testAdminPassword, stepUpActionReveal), "step_up_code_required", http.StatusForbidden},
		{"wrong code", `{"password":"` + testAdminPassword + `","code":"000000","action":"reveal","app":"web"}`, "step_up_invalid", http.StatusForbidden},
		{"code without password", `{"password":"","code":"` + h.code(0) + `","action":"reveal","app":"web"}`, "step_up_invalid", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.do(http.MethodPost, "/api/v1/auth/step-up", tt.body)
			if rec.Code != tt.want || errorCode(t, rec) != tt.code {
				t.Fatalf("status = %d code = %q", rec.Code, errorCode(t, rec))
			}
		})
	}
	// A fresh time step: the library rejects a code it has already seen.
	rec = h.do(http.MethodPost, "/api/v1/auth/step-up", `{"password":"`+testAdminPassword+`","code":"`+h.code(30*time.Second)+`","action":"reveal","app":"web"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("password plus code = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reveal after step-up = %d %s", rec.Code, rec.Body.String())
	}
}

func TestStepUp_Passkey(t *testing.T) {
	h := newMFAHarness(t)
	seedRevealApp(t, h.db)
	key := h.registerPasskey("laptop")

	assert := func() (string, string) {
		rec := h.do(http.MethodPost, "/api/v1/auth/step-up/passkey/begin", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("begin = %d %s", rec.Code, rec.Body.String())
		}
		var resp passkeyLoginBeginResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(resp.Options)
		opts, err := virtualwebauthn.ParseAssertionOptions(string(raw))
		if err != nil {
			t.Fatal(err)
		}
		return resp.SessionID, virtualwebauthn.CreateAssertionResponse(h.rp(), key.auth, key.cred, *opts)
	}
	session, body := assert()
	rec := h.do(http.MethodPost, "/api/v1/auth/step-up/passkey/finish?session_id="+url.QueryEscape(session)+"&action=reveal&app=web", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("passkey finish = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("reveal after passkey = %d %s", rec.Code, rec.Body.String())
	}
	rec = h.do(http.MethodPost, "/api/v1/auth/step-up/passkey/finish?session_id="+url.QueryEscape(session)+"&action=reveal&app=web", body)
	if rec.Code == http.StatusOK {
		t.Fatal("replayed passkey assertion was accepted")
	}
}
