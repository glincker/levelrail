package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func stepUpBody(password, action string) string {
	return `{"method":"password","password":"` + password + `","action":"` + action + `","app":"web"}`
}

func revealValue(t *testing.T, f envFixture, cookie *http.Cookie, path, body string) revealResponse {
	t.Helper()
	var out revealResponse
	rec := f.do(t, cookie, http.MethodPost, path, body, http.StatusOK)
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("reveal response is cacheable: %q", rec.Header().Get("Cache-Control"))
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReveal_RequiresStepUpAndIsAudited(t *testing.T) {
	f := newEnvFixture(t)

	rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`, http.StatusForbidden)
	if errorCode(t, rec) != errCodeStepUpRequired {
		t.Fatalf("code = %q", errorCode(t, rec))
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody("wrong-password", stepUpActionReveal), http.StatusForbidden)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)

	if got := revealValue(t, f, f.cookie, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`); got.Value != plainSecretValue || got.HideAfterSeconds != 30 {
		t.Fatalf("reveal = %+v", got)
	}
	if got := revealValue(t, f, f.cookie, "/api/v1/apps/web/secrets/DB_PASSWORD/reveal", `{}`); got.Value != storedSecretValue || got.Type != envTypeSecret {
		t.Fatalf("secret reveal = %+v", got)
	}

	entries, err := f.db.ListAuditEntries(context.Background(), 500, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var revealed, failed bool
	for _, e := range entries {
		raw, _ := json.Marshal(e)
		assertNoSecretValues(t, "audit row", string(raw))
		if strings.Contains(string(raw), testAdminPassword) {
			t.Fatal("audit row holds the password")
		}
		revealed = revealed || (e.Action == auditActionEnvRevealed && strings.Contains(e.Path, "API_SECRET") && strings.Contains(e.Path, stepUpMethodPassword))
		failed = failed || e.Action == auditActionStepUpFailed
	}
	if !revealed || !failed {
		t.Fatalf("missing audit rows: revealed=%v failed=%v", revealed, failed)
	}
}

func TestStepUpGrant_ScopeSessionAndExpiry(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)

	tests := []struct {
		name   string
		cookie *http.Cookie
		method string
		path   string
		body   string
		want   int
	}{
		{"other app", f.cookie, http.MethodPost, "/api/v1/apps/other/env/reveal", `{"key":"API_SECRET"}`, http.StatusForbidden},
		{"export needs its own scope", f.cookie, http.MethodPost, "/api/v1/apps/web/env/export", `{"format":"dotenv"}`, http.StatusForbidden},
		{"another session of the same user", loginTestSession(t, f.rt, f.db), http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(t, tt.cookie, tt.method, tt.path, tt.body, tt.want)
			assertNoSecretValues(t, tt.name, rec.Body.String())
		})
	}

	t.Run("expired grant", func(t *testing.T) {
		g := newStepUpGrants()
		g.issue("tok", "u1", stepUpActionReveal, "web", stepUpMethodPassword, time.Minute, time.Now().Add(-2*time.Minute))
		if _, ok := g.check("tok", "u1", stepUpActionReveal, "web", time.Now()); ok {
			t.Fatal("expired grant accepted")
		}
		g.issue("tok", "u1", stepUpActionReveal, "web", stepUpMethodPassword, time.Minute, time.Now())
		if _, ok := g.check("tok", "u2", stepUpActionReveal, "web", time.Now()); ok {
			t.Fatal("grant accepted for a different user")
		}
		if _, ok := g.check("tok2", "u1", stepUpActionReveal, "web", time.Now()); ok {
			t.Fatal("grant replayed from another session token")
		}
	})

	t.Run("export after export step-up", func(t *testing.T) {
		f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionExport), http.StatusOK)
		rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/export", `{"format":"dotenv"}`, http.StatusOK)
		body := rec.Body.String()
		if !strings.Contains(body, "API_SECRET="+plainSecretValue) || !strings.Contains(body, "DB_PASSWORD="+storedSecretValue) {
			t.Fatalf("full export = %s", body)
		}
		if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
			t.Fatalf("export headers = %v", rec.Header())
		}
	})
}

func TestStepUp_RateLimited(t *testing.T) {
	f := newEnvFixture(t)
	var last int
	for range 6 {
		last = f.serve(authedRequest(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody("nope", stepUpActionReveal))).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("after repeated failures status = %d, want 429", last)
	}
	rec := f.serve(authedRequest(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal)))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked out user got %d with the right password", rec.Code)
	}
}

func TestReveal_PolicyModes(t *testing.T) {
	tests := []struct {
		policy, code string
		want         int
	}{
		{"disabled", errCodeRevealDisabled, http.StatusForbidden},
		{"typo-allow", errCodeRevealDisabled, http.StatusForbidden},
		{"step_up", errCodeStepUpRequired, http.StatusForbidden},
		{"allow", "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.policy, func(t *testing.T) {
			t.Setenv(envSecretReveal, tt.policy)
			f := newEnvFixture(t)
			rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`, tt.want)
			if tt.code != "" && errorCode(t, rec) != tt.code {
				t.Fatalf("code = %q, want %q", errorCode(t, rec), tt.code)
			}
		})
	}
}

func TestReveal_PerAppWriteOnlyLock(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env/settings", `{"secrets_write_only":true}`, http.StatusOK)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)
	rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`, http.StatusForbidden)
	if errorCode(t, rec) != errCodeAppWriteOnly {
		t.Fatalf("code = %q", errorCode(t, rec))
	}
	// Unlocking loosens protection, so it needs its own step-up.
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env/settings", `{"secrets_write_only":false}`, http.StatusForbidden)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env/settings", `{"secrets_write_only":false,"step_up":{"password":"`+testAdminPassword+`"}}`, http.StatusOK)
	revealValue(t, f, f.cookie, "/api/v1/apps/web/env/reveal", `{"key":"API_SECRET"}`)
}

func TestEnvSettings_LooseningNeedsStepUp(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env/settings", `{"sensitive_overrides":{"PORT":true}}`, http.StatusOK)
	got := f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web", "", http.StatusOK).Body.String()
	if strings.Contains(got, `"PORT":"3000"`) {
		t.Fatalf("PORT flagged sensitive is still visible: %s", got)
	}
	f.do(t, f.cookie, http.MethodPut, "/api/v1/apps/web/env/settings", `{"sensitive_overrides":{"API_SECRET":false}}`, http.StatusForbidden)
	assertNoSecretValues(t, "app after refused loosening", f.do(t, f.cookie, http.MethodGet, "/api/v1/apps/web", "", http.StatusOK).Body.String())
}

func TestReveal_APITokens(t *testing.T) {
	f := newEnvFixture(t)
	mint := func(abilities string) string {
		rec := f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"t`+strings.ReplaceAll(abilities, `"`, "")+`","abilities":[`+abilities+`]}`, http.StatusCreated)
		var out createTokenResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out.Token
	}
	readTok := mint(`"read"`)
	sensTok := mint(`"read","read:sensitive"`)
	revealTok := mint(`"read","read:sensitive","secrets:reveal"`)
	proof := `{"key":"API_SECRET","step_up":{"password":"` + testAdminPassword + `"}}`

	for _, path := range []string{"/api/v1/apps/web", "/api/v1/apps", "/api/v1/apps/web/env/export"} {
		rec := f.serve(bearerRequest(http.MethodGet, path, "", readTok))
		if rec.Code != http.StatusOK {
			t.Fatalf("read token GET %s = %d", path, rec.Code)
		}
		assertNoSecretValues(t, "read token "+path, rec.Body.String())
	}
	tests := []struct {
		name, token, code string
		tokensOn          bool
		body              string
		want              int
	}{
		{"read token", readTok, "", true, proof, http.StatusForbidden},
		{"sensitive token without reveal ability", sensTok, errCodeTokenRevealAbility, true, proof, http.StatusForbidden},
		{"reveal token while policy off", revealTok, errCodeTokenRevealDisabled, false, proof, http.StatusForbidden},
		{"reveal token without proof", revealTok, errCodeStepUpRequired, true, `{"key":"API_SECRET"}`, http.StatusForbidden},
		{"reveal token with wrong proof", revealTok, "step_up_invalid", true, `{"key":"API_SECRET","step_up":{"password":"bad"}}`, http.StatusForbidden},
		{"reveal token with proof", revealTok, "", true, proof, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.tokensOn {
				t.Setenv(envSecretRevealTokens, "on")
			}
			rec := f.serve(bearerRequest(http.MethodPost, "/api/v1/apps/web/env/reveal", tt.body, tt.token))
			if rec.Code != tt.want || (tt.code != "" && errorCode(t, rec) != tt.code) {
				t.Fatalf("status = %d code = %q body = %s", rec.Code, errorCode(t, rec), rec.Body.String())
			}
			if tt.want != http.StatusOK {
				assertNoSecretValues(t, tt.name, rec.Body.String())
			}
		})
	}
	rec := f.serve(bearerRequest(http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), revealTok))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("token step-up = %d, want 403", rec.Code)
	}
}

func TestConvertEnvToSecret(t *testing.T) {
	f := newEnvFixture(t)
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/API_SECRET/convert-to-secret", "", http.StatusNoContent)
	svc, err := f.db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if _, still := svc.Env["API_SECRET"]; still || !svc.EnvDirty {
		t.Fatalf("plain copy kept or env not dirty: %v dirty=%v", svc.Env, svc.EnvDirty)
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/auth/step-up", stepUpBody(testAdminPassword, stepUpActionReveal), http.StatusOK)
	if got := revealValue(t, f, f.cookie, "/api/v1/apps/web/secrets/API_SECRET/reveal", `{}`); got.Value != plainSecretValue {
		t.Fatalf("converted secret = %+v", got)
	}
	f.do(t, f.cookie, http.MethodPost, "/api/v1/apps/web/env/API_SECRET/convert-to-secret", "", http.StatusNotFound)
}
