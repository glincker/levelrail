package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newCLIPairing(t *testing.T, rt *Router, cookie *http.Cookie) cliPairingCodeResponse {
	t.Helper()
	rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/cli-pairing", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cli pairing = %d %s", rec.Code, rec.Body.String())
	}
	var out cliPairingCodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func cliRedeemRequest(code, ip string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli-pairing/redeem", strings.NewReader(`{"code":"`+code+`","client_name":"laptop"}`))
	r.RemoteAddr = ip + ":1234"
	return r
}

func cliAuditActions(t *testing.T, db *store.DB) []string {
	t.Helper()
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Action)
		if strings.Contains(e.Path, strings.ReplaceAll(e.Detail, "-", "")) && e.Detail != "" {
			t.Errorf("audit row leaks detail into path: %q", e.Path)
		}
	}
	return out
}

func TestCLIPairing_RedeemOutcomes(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	good := newCLIPairing(t, rt, admin)
	if len(good.Code) != 9 || good.Code[4] != '-' || !time.Now().Before(good.ExpiresAt) || time.Until(good.ExpiresAt) > maxCLIPairingTTL {
		t.Fatalf("pairing response = %+v", good)
	}
	plain := normalizeCLIPairingCode(good.Code)
	var storedHash string
	if err := db.QueryRowContext(context.Background(), `SELECT code_hash FROM mobile_pairing_codes WHERE lookup = ?`, cliPairingLookup(plain)).Scan(&storedHash); err != nil || storedHash == plain || storedHash != hashPairingCode(plain) {
		t.Fatalf("stored code is not a hash: %q (%v)", storedHash, err)
	}
	wrong := plain[:cliPairingGroupLen] + strings.Repeat("A", cliPairingGroupLen)
	if wrong == plain {
		wrong = plain[:cliPairingGroupLen] + strings.Repeat("B", cliPairingGroupLen)
	}
	tests := []struct {
		name string
		code string
		ip   string
		want int
	}{
		{"unknown code", "ZZZZ-ZZZZ", "10.0.0.1", http.StatusBadRequest},
		{"wrong second half", wrong, "10.0.0.2", http.StatusBadRequest},
		{"malformed", "abc", "10.0.0.3", http.StatusBadRequest},
		{"valid, lower case and spaces", " " + strings.ToLower(good.Code) + " ", "10.0.0.4", http.StatusCreated},
		{"reuse after redeem", good.Code, "10.0.0.5", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveMobile(rt, cliRedeemRequest(tc.code, tc.ip))
			if rec.Code != tc.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
			}
			if tc.want == http.StatusBadRequest && !strings.Contains(rec.Body.String(), errCodeCLIPairing) {
				t.Fatalf("want the generic refusal, got %s", rec.Body.String())
			}
		})
	}
}

func TestCLIPairing_TokenIsScopedAndWorks(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	p := newCLIPairing(t, rt, admin)
	rec := serveMobile(rt, cliRedeemRequest(p.Code, "10.0.1.1"))
	var out cliPairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("redeem = %d %s", rec.Code, rec.Body.String())
	}
	if slices.Contains(out.Abilities, AbilityRoot) {
		t.Fatalf("paired CLI token is root: %v", out.Abilities)
	}
	if d := time.Until(out.ExpiresAt); d > maxDeviceTokenDays*24*time.Hour {
		t.Fatalf("token lives %s", d)
	}
	if rec := serveMobile(rt, mobileBearer(http.MethodGet, "/api/v1/apps", out.Token)); rec.Code != http.StatusOK {
		t.Fatalf("paired token cannot read apps: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serveMobile(rt, mobileBearer(http.MethodPost, "/api/v1/auth/cli-pairing", out.Token)); rec.Code == http.StatusCreated {
		t.Fatal("a bearer token minted a pairing code")
	}
}

func TestCLIPairing_Expired(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	p := newCLIPairing(t, rt, admin)
	if _, err := db.ExecContext(context.Background(), `UPDATE mobile_pairing_codes SET expires_at = ? WHERE lookup = ?`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), cliPairingLookup(normalizeCLIPairingCode(p.Code))); err != nil {
		t.Fatal(err)
	}
	if rec := serveMobile(rt, cliRedeemRequest(p.Code, "10.0.2.1")); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired code = %d", rec.Code)
	}
}

func TestCLIPairing_OnlyFromAuthenticatedSession(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	anon := httptest.NewRequest(http.MethodPost, "/api/v1/auth/cli-pairing", nil)
	if rec := serveMobile(rt, anon); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous create = %d", rec.Code)
	}
	p := newCLIPairing(t, rt, admin)
	tok := serveMobile(rt, cliRedeemRequest(p.Code, "10.0.3.1"))
	var out cliPairResponse
	_ = json.Unmarshal(tok.Body.Bytes(), &out)
	if rec := serveMobile(rt, mobileBearer(http.MethodPost, "/api/v1/auth/cli-pairing", out.Token)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bearer create = %d, want 401", rec.Code)
	}
}

func TestCLIPairing_NewCodeRevokesTheOldOne(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	first := newCLIPairing(t, rt, admin)
	second := newCLIPairing(t, rt, admin)
	if rec := serveMobile(rt, cliRedeemRequest(first.Code, "10.0.4.1")); rec.Code != http.StatusBadRequest {
		t.Fatalf("superseded code = %d", rec.Code)
	}
	if rec := serveMobile(rt, cliRedeemRequest(second.Code, "10.0.4.2")); rec.Code != http.StatusCreated {
		t.Fatalf("latest code = %d", rec.Code)
	}
}

func TestCLIPairing_RevokedByCredentialReset(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	p := newCLIPairing(t, rt, admin)
	userID, ok := rt.sessions.lookup(admin.Value)
	if !ok {
		t.Fatal("no session user")
	}
	rt.resetSignInTrust(context.Background(), httptest.NewRequest(http.MethodPost, "/", nil), userID, "password_changed")
	if rec := serveMobile(rt, cliRedeemRequest(p.Code, "10.0.5.1")); rec.Code != http.StatusBadRequest {
		t.Fatalf("code survived a password change: %d", rec.Code)
	}
}

func TestCLIPairing_WrongGuessesBurnCode(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	p := newCLIPairing(t, rt, admin)
	plain := normalizeCLIPairingCode(p.Code)
	wrong := plain[:cliPairingGroupLen] + "AAAA"
	if wrong == plain {
		wrong = plain[:cliPairingGroupLen] + "BBBB"
	}
	for i := range cliPairingMaxTries {
		serveMobile(rt, cliRedeemRequest(wrong, "10.1.0."+string(rune('1'+i))))
	}
	if rec := serveMobile(rt, cliRedeemRequest(p.Code, "10.1.1.1")); rec.Code == http.StatusCreated {
		t.Fatal("code still pairs after the wrong-guess budget")
	}
}

func TestCLIPairing_RateLimits(t *testing.T) {
	t.Setenv(envCLIPairPerMinute, "3")
	t.Setenv(envCLIPairCreatePerMin, "2")
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	last := 0
	for i := range 5 {
		last = serveMobile(rt, cliRedeemRequest("AAAA-BBB"+string(rune('C'+i)), "10.2.0.1")).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("5th redeem from one IP = %d, want 429", last)
	}
	newCLIPairing(t, rt, admin)
	newCLIPairing(t, rt, admin)
	if rec := serveMobile(rt, authedRequest(t, admin, http.MethodPost, "/api/v1/auth/cli-pairing", "")); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd create = %d, want 429", rec.Code)
	}
}

func TestCLIPairing_AuditTrail(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	p := newCLIPairing(t, rt, admin)
	serveMobile(rt, cliRedeemRequest("ZZZZ-ZZZZ", "10.3.0.1"))
	serveMobile(rt, cliRedeemRequest(p.Code, "10.3.0.2"))
	actions := cliAuditActions(t, db)
	for _, want := range []string{store.AuditActionCLIPairCreated, store.AuditActionCLIPairFailed, store.AuditActionCLIPaired} {
		if !slices.Contains(actions, want) {
			t.Errorf("audit actions %v lack %s", actions, want)
		}
	}
	entries, _ := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	plain := normalizeCLIPairingCode(p.Code)
	for _, e := range entries {
		if strings.Contains(strings.ToUpper(e.Path+e.Detail), plain) {
			t.Fatalf("audit row carries the code: %+v", e)
		}
	}
}
