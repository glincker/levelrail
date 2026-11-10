package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func mobileDenyFeature() authzDenyFeature {
	return authzDenyFeature{
		name:  "mobile",
		match: func(_, p string) bool { return strings.HasPrefix(p, "/api/v1/mobile/") },
		routes: []authzDenyRoute{
			{http.MethodPost, "/api/v1/mobile/pairing-codes", "", AbilityRoot, true},
			{http.MethodPost, "/api/v1/mobile/pair", `{"code":"AAAAAAAAAAAAAAAA","device_name":"x","platform":"android"}`, "", false},
			{http.MethodGet, "/api/v1/mobile/devices", "", AbilityRead, false},
			{http.MethodDelete, "/api/v1/mobile/devices/{id}", "", AbilityRead, false},
			{http.MethodGet, "/api/v1/mobile/overview", "", AbilityRead, false},
		},
		stepUp: mobileStepUp,
	}
}

func TestAuthzDeny_Mobile(t *testing.T) {
	runAuthzDeny(t, mobileDenyFeature())
}

// A paired phone holds read and deploy only: it cannot mint pairing codes, and revoking it ends every route at once.
func TestAuthzDeny_MobileDeviceToken(t *testing.T) {
	a := newAuthzDenyActors(t)
	mobileStepUp(t, a.rt, a.admin)
	dev := pairDevice(t, a.rt, a.admin)
	a.secrets = append(a.secrets, dev.Token)

	if rec := a.do(http.MethodPost, "/api/v1/mobile/pairing-codes", "", nil, dev.Token, ""); rec.Code != http.StatusForbidden {
		t.Errorf("device token minting a pairing code = %d, want 403", rec.Code)
	}
	for _, p := range []string{"/api/v1/settings/ai-control", "/api/v1/roles/role_x"} {
		method := http.MethodPut
		if p == "/api/v1/roles/role_x" {
			method = http.MethodDelete
		}
		if rec := a.do(method, p, `{"mode":"off"}`, nil, dev.Token, ""); rec.Code != http.StatusForbidden {
			t.Errorf("device token %s %s = %d, want 403", method, p, rec.Code)
		}
	}
	for _, p := range []string{"/api/v1/system/cli-console/commands", "/api/v1/system/cli-console/session"} {
		if rec := a.do(http.MethodGet, p, "", nil, dev.Token, "http://example.com"); rec.Code != http.StatusForbidden {
			t.Errorf("device token GET %s = %d, want 403", p, rec.Code)
		}
	}
	// A different read-tier user must not revoke a device it does not own.
	if rec := a.do(http.MethodDelete, "/api/v1/mobile/devices/"+dev.DeviceID, "", a.reader, "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("stranger revoke = %d, want 404", rec.Code)
	}
	if rec := a.do(http.MethodGet, "/api/v1/mobile/overview", "", nil, dev.Token, ""); rec.Code != http.StatusOK {
		t.Fatalf("device token must still work after a refused revoke, got %d", rec.Code)
	}
	if rec := a.do(http.MethodDelete, "/api/v1/mobile/devices/"+dev.DeviceID, "", a.admin, "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("owner revoke = %d", rec.Code)
	}
	for _, r := range mobileDenyFeature().routes {
		if r.ability == "" {
			continue
		}
		if rec := a.do(r.method, r.path(), r.body, nil, dev.Token, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with a revoked device token = %d, want 401", r.key(), rec.Code)
		}
	}
	a.assertNoSecrets(t)
}

// Flag off and the kill switch answer 404 on every route, public pairing included, before any handler work.
func TestAuthzDeny_MobileGateOff(t *testing.T) {
	a := newAuthzDenyActors(t)
	for name, setup := range map[string]func(){
		"flag off":    func() { experimental.Set() },
		"kill switch": func() { experimental.Set(experimental.MobileAPI); t.Setenv(envMobileAPI, "off") },
	} {
		setup()
		for _, r := range mobileDenyFeature().routes {
			rec := a.do(r.method, r.path(), r.body, a.admin, "", "")
			if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), ExperimentalDisabledCode) {
				t.Errorf("%s %s as admin = %d %s, want 404 %s", name, r.key(), rec.Code, rec.Body.String(), ExperimentalDisabledCode)
			}
		}
		if rec := a.do(http.MethodPost, "/api/v1/mobile/pair", mobileDenyFeature().routes[1].body, nil, "", ""); rec.Code != http.StatusNotFound {
			t.Errorf("%s anonymous pair = %d, want 404", name, rec.Code)
		}
	}
	a.assertNoSecrets(t)
}
