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

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func serveMobile(rt *Router, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	return rec
}

func mobileStepUp(t *testing.T, rt *Router, cookie *http.Cookie) {
	t.Helper()
	body := `{"password":"` + testAdminPassword + `","action":"manage","app":"` + mobilePairingStepUpApp + `"}`
	if rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/step-up", body)); rec.Code != http.StatusOK {
		t.Fatalf("step-up = %d %s", rec.Code, rec.Body.String())
	}
}

func newPairing(t *testing.T, rt *Router, cookie *http.Cookie) pairingCodeResponse {
	t.Helper()
	rec := serveMobile(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/mobile/pairing-codes", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create pairing = %d %s", rec.Code, rec.Body.String())
	}
	var out pairingCodeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func pairRequest(code, ip string) *http.Request {
	body := `{"code":"` + code + `","device_name":"Pixel 9","platform":"android"}`
	r := httptest.NewRequest(http.MethodPost, "/api/v1/mobile/pair", strings.NewReader(body))
	r.RemoteAddr = ip + ":1234"
	return r
}

func TestMobilePairing_StepUpRequired(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	rec := serveMobile(rt, authedRequest(t, admin, http.MethodPost, "/api/v1/mobile/pairing-codes", ""))
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), errCodeStepUpRequired) {
		t.Fatalf("no step-up = %d %s", rec.Code, rec.Body.String())
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	if rec := serveMobile(rt, authedRequest(t, sessionCookieForTest(t, rt, reader.ID), http.MethodPost, "/api/v1/mobile/pairing-codes", "")); rec.Code != http.StatusForbidden {
		t.Fatalf("reader = %d", rec.Code)
	}
}

func TestMobilePairing_CodeLifecycle(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mobileStepUp(t, rt, admin)
	p := newPairing(t, rt, admin)
	if len(p.Code) != 40 || !strings.Contains(p.QRPayload, p.Code) || !strings.Contains(p.PairURL, "code=") {
		t.Fatalf("pairing response = %+v", p)
	}
	expired := newPairing(t, rt, admin)
	if _, err := db.ExecContext(context.Background(), `UPDATE mobile_pairing_codes SET expires_at = ? WHERE lookup = ?`,
		time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), expired.Code[:mobileLookupLen]); err != nil {
		t.Fatal(err)
	}
	wrong := p.Code[:mobileLookupLen] + strings.Repeat("A", len(p.Code)-mobileLookupLen)

	tests := []struct {
		name string
		code string
		ip   string
		want int
	}{
		{"unknown code", strings.Repeat("B", 40), "10.0.0.1", http.StatusBadRequest},
		{"wrong secret", wrong, "10.0.0.2", http.StatusBadRequest},
		{"expired", expired.Code, "10.0.0.3", http.StatusBadRequest},
		{"valid", p.Code, "10.0.0.4", http.StatusCreated},
		{"reuse", p.Code, "10.0.0.5", http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveMobile(rt, pairRequest(tc.code, tc.ip))
			if rec.Code != tc.want {
				t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
			}
			if tc.want == http.StatusBadRequest && !strings.Contains(rec.Body.String(), errCodeMobilePairing) {
				t.Fatalf("want generic error, got %s", rec.Body.String())
			}
		})
	}
}

func TestMobilePairing_WrongGuessesBurnCode(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mobileStepUp(t, rt, admin)
	p := newPairing(t, rt, admin)
	wrong := p.Code[:mobileLookupLen] + strings.Repeat("A", len(p.Code)-mobileLookupLen)
	for i := 0; i < mobilePairingMaxTries; i++ {
		serveMobile(rt, pairRequest(wrong, "10.1.0."+string(rune('1'+i))))
	}
	if _, err := db.GetMobilePairingCodeByLookup(context.Background(), p.Code[:mobileLookupLen], time.Now(), mobilePairingMaxTries); err == nil {
		t.Fatal("code still usable after max wrong guesses")
	}
	rec := serveMobile(rt, pairRequest(p.Code, "10.1.1.1"))
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusTooManyRequests {
		t.Fatalf("burned code still pairs: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMobilePairing_RateLimitPerIP(t *testing.T) {
	t.Setenv(envMobilePairPerMinute, "3")
	rt, _ := newTestRouter(t)
	last := 0
	for i := 0; i < 5; i++ {
		last = serveMobile(rt, pairRequest(strings.Repeat(string(rune('C'+i)), 40), "10.2.0.1")).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("5th try from one IP = %d, want 429", last)
	}
}

func pairDevice(t *testing.T, rt *Router, admin *http.Cookie) mobilePairResponse {
	t.Helper()
	mobileStepUp(t, rt, admin)
	p := newPairing(t, rt, admin)
	rec := serveMobile(rt, pairRequest(p.Code, "10.3.0.1"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("pair = %d %s", rec.Code, rec.Body.String())
	}
	var out mobilePairResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mobileBearer(method, target, token string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func TestMobileToken_AbilitiesAndListing(t *testing.T) {
	t.Setenv(envMobileTokenTTL, "7d")
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	dev := pairDevice(t, rt, admin)
	for _, banned := range []string{AbilityRoot, AbilitySecretsReveal, AbilitySignInApprove, AbilityWriteSensitive, AbilityReadSensitive} {
		if slices.Contains(dev.Abilities, banned) {
			t.Fatalf("mobile token carries %s: %v", banned, dev.Abilities)
		}
	}
	if !slices.Equal(dev.Abilities, []string{AbilityRead, AbilityDeploy}) {
		t.Fatalf("abilities = %v", dev.Abilities)
	}
	if d := time.Until(dev.ExpiresAt); d < 6*24*time.Hour || d > 8*24*time.Hour {
		t.Fatalf("expiry %s, want about 7d", d)
	}
	if rec := serveMobile(rt, mobileBearer(http.MethodPost, "/api/v1/mobile/pairing-codes", dev.Token)); rec.Code != http.StatusForbidden {
		t.Fatalf("mobile token minting a pairing code = %d", rec.Code)
	}
	rec := serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/auth/tokens", ""))
	if !strings.Contains(rec.Body.String(), `"Mobile: Pixel 9"`) {
		t.Fatalf("token list lacks mobile row: %s", rec.Body.String())
	}
}

func TestMobileDevices_ListAndRevoke(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	dev := pairDevice(t, rt, admin)
	if rec := serveMobile(rt, mobileBearer(http.MethodGet, "/api/v1/mobile/overview", dev.Token)); rec.Code != http.StatusOK {
		t.Fatalf("overview with device token = %d %s", rec.Code, rec.Body.String())
	}
	rec := serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/mobile/devices", ""))
	var list []mobileDeviceResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Name != "Pixel 9" || list[0].LastSeenAt == nil {
		t.Fatalf("devices = %s", rec.Body.String())
	}
	other := storeUserWithAbilitiesForTest(t, db, "other@example.com", []string{AbilityRead})
	if rec := serveMobile(rt, authedRequest(t, sessionCookieForTest(t, rt, other.ID), http.MethodDelete, "/api/v1/mobile/devices/"+dev.DeviceID, "")); rec.Code != http.StatusNotFound {
		t.Fatalf("stranger revoke = %d", rec.Code)
	}
	if rec := serveMobile(rt, authedRequest(t, admin, http.MethodDelete, "/api/v1/mobile/devices/"+dev.DeviceID, "")); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", rec.Code, rec.Body.String())
	}
	if rec := serveMobile(rt, mobileBearer(http.MethodGet, "/api/v1/mobile/overview", dev.Token)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked token still works: %d", rec.Code)
	}
	rec = serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/mobile/devices", ""))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("revoked device still listed: %s", rec.Body.String())
	}
}

func TestMobileOverview_ShapeAndETag(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	rec := serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/mobile/overview", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("overview = %d %s", rec.Code, rec.Body.String())
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"server", "attention", "apps", "apps_total", "nodes", "deploys_24h", "open_incidents"} {
		if _, ok := got[k]; !ok {
			t.Fatalf("overview missing %q: %s", k, rec.Body.String())
		}
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	r := authedRequest(t, admin, http.MethodGet, "/api/v1/mobile/overview", "")
	r.Header.Set("If-None-Match", etag)
	if rec := serveMobile(rt, r); rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("If-None-Match = %d", rec.Code)
	}
}

func TestMobileAPI_FlagOff(t *testing.T) {
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	tests := []struct {
		name  string
		setup func()
	}{
		{"flag off", func() { experimental.Set() }},
		{"kill switch", func() { experimental.Set(experimental.MobileAPI); t.Setenv(envMobileAPI, "off") }},
	}
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/v1/mobile/pairing-codes"},
		{http.MethodPost, "/api/v1/mobile/pair"},
		{http.MethodGet, "/api/v1/mobile/devices"},
		{http.MethodDelete, "/api/v1/mobile/devices/mdv_x"},
		{http.MethodGet, "/api/v1/mobile/overview"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			for _, rc := range routes {
				rec := serveMobile(rt, authedRequest(t, admin, rc.method, rc.path, ""))
				if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), ExperimentalDisabledCode) {
					t.Fatalf("%s %s = %d %s", rc.method, rc.path, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

func TestEtagMatches(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{`W/"abc"`, true},
		{`"abc"`, true},
		{`"x", W/"abc"`, true},
		{`*`, true},
		{`"zzz"`, false},
		{``, false},
	}
	for _, tc := range tests {
		if got := etagMatches(tc.header, `W/"abc"`); got != tc.want {
			t.Errorf("etagMatches(%q) = %v", tc.header, got)
		}
	}
}
