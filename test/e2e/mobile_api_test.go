package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
)

// TestMobileAPI_Live_PairAndRevoke drives pairing and revoke over real HTTP
// against a real router and store. It proves the wire flow only: a real phone
// run (app, camera, push) stays a separate needs-real-host item.
func TestMobileAPI_Live_PairAndRevoke(t *testing.T) {
	t.Setenv(experimental.EnvVar, string(experimental.MobileAPI))
	db := openLiveStore(t)
	dataDir := t.TempDir()
	setupToken, _, err := api.EnsureSetupToken(context.Background(), db, dataDir)
	if err != nil {
		t.Fatalf("EnsureSetupToken() error = %v", err)
	}
	router := api.NewRouter(discardTestLogger(), &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}, db, api.WithDataDir(dataDir))
	server := httptest.NewTLSServer(router.Handler())
	t.Cleanup(server.Close)

	const password = "first-real-password"
	admin := newAuthClient(t, server)
	call := func(c *http.Client, method, path, body string) (int, []byte) {
		return requestJSON(t, c, method, server.URL+path, body)
	}
	if status, body := call(admin, http.MethodPost, "/api/v1/auth/register",
		`{"username":"admin","password":"`+password+`","setup_token":"`+setupToken+`"}`); status != http.StatusCreated {
		t.Fatalf("register = %d %s", status, body)
	}

	// Minting a pairing code needs a fresh step-up on top of the session.
	if status, body := call(admin, http.MethodPost, "/api/v1/mobile/pairing-codes", ""); status != http.StatusForbidden {
		t.Fatalf("pairing code without step-up = %d %s", status, body)
	}
	if status, body := call(admin, http.MethodPost, "/api/v1/auth/step-up",
		`{"password":"`+password+`","action":"manage","app":"mobile-pairing"}`); status != http.StatusOK {
		t.Fatalf("step-up = %d %s", status, body)
	}
	status, body := call(admin, http.MethodPost, "/api/v1/mobile/pairing-codes", "")
	if status != http.StatusCreated {
		t.Fatalf("pairing code = %d %s", status, body)
	}
	var code struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(body, &code); err != nil || code.Code == "" {
		t.Fatalf("decode pairing code %s: %v", body, err)
	}

	// The phone has no session: the one-time code is its only credential.
	phone := newAuthClient(t, server)
	pairBody := `{"code":"` + code.Code + `","device_name":"Pixel 9","platform":"android"}`
	status, body = call(phone, http.MethodPost, "/api/v1/mobile/pair", pairBody)
	if status != http.StatusCreated {
		t.Fatalf("pair = %d %s", status, body)
	}
	var dev struct {
		Token    string `json:"token"`
		DeviceID string `json:"device_id"`
	}
	if err := json.Unmarshal(body, &dev); err != nil || dev.Token == "" || dev.DeviceID == "" {
		t.Fatalf("decode pair response: %v", err)
	}
	if status, _ := call(phone, http.MethodPost, "/api/v1/mobile/pair", pairBody); status != http.StatusBadRequest {
		t.Fatalf("code reuse = %d, want 400", status)
	}

	bearer := func(method, path string) int {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, nil) //nolint:noctx // loopback test helper
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+dev.Token)
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := bearer(http.MethodGet, "/api/v1/mobile/overview"); got != http.StatusOK {
		t.Fatalf("overview with device token = %d", got)
	}

	if status, body := call(admin, http.MethodDelete, "/api/v1/mobile/devices/"+dev.DeviceID, ""); status != http.StatusNoContent {
		t.Fatalf("revoke = %d %s", status, body)
	}
	if got := bearer(http.MethodGet, "/api/v1/mobile/overview"); got != http.StatusUnauthorized {
		t.Fatalf("revoked token = %d, want 401", got)
	}

	// Pairing and revoke must both leave an audit trail.
	status, body = call(admin, http.MethodGet, "/api/v1/audit-log?q=mobile", "")
	if status != http.StatusOK {
		t.Fatalf("audit log = %d %s", status, body)
	}
	for _, action := range []string{"mobile.pairing_created", "mobile.paired", "mobile.revoked"} {
		if !strings.Contains(string(body), `"`+action+`"`) {
			t.Errorf("audit log lacks %s: %s", action, body)
		}
	}
}
