package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

const panelPluginBody = `contributes:
  panels:
    - id: deploys
      title: Recent deploys
      slot: app-detail
      source: { id: deploys, abilities: [read] }
      widgets:
        - { type: stat, field: total }
        - { type: button, action: refresh, label: Refresh }
`

func panelPluginManifest() string {
	return "api_version: 1\nname: deploy-panel\nversion: 1.0.0\nkind: panel_provider\npublisher: Example Org\n" +
		"image: registry.example.com/team/deploy-panel@" + pluginTestDigest + "\nabilities: [read]\n" + panelPluginBody
}

func TestPluginPanelRoutes(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	manifest := panelPluginManifest()
	req := func(cookie *http.Cookie, method, path, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, cookie, method, path, body))
		return rec.Code, rec.Body.String()
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if code, body := req(admin, http.MethodPost, "/api/v1/plugin-keys", js(map[string]string{"name": "example", "public_key": base64.StdEncoding.EncodeToString(pub)})); code != http.StatusCreated {
		t.Fatalf("trust add = %d %s", code, body)
	}
	install := map[string]string{"manifest": manifest, "key_id": plugin.Fingerprint(pub), "signature": plugin.Sign(priv, []byte(manifest), pluginTestDigest)}
	if code, body := req(admin, http.MethodPost, "/api/v1/plugins", js(install)); code != http.StatusCreated {
		t.Fatalf("install = %d %s", code, body)
	}

	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)

	_, body := req(readerCookie, http.MethodGet, "/api/v1/plugin-panels?slot=app-detail", "")
	if strings.TrimSpace(body) != "[]" {
		t.Fatalf("a plugin that is not enabled listed panels: %s", body)
	}
	if code, _ := req(admin, http.MethodPost, "/api/v1/plugins/deploy-panel/approve", `{"abilities":["read"]}`); code != http.StatusOK {
		t.Fatal("approve failed")
	}
	if code, _ := req(admin, http.MethodPost, "/api/v1/plugins/deploy-panel/enable", ""); code != http.StatusOK {
		t.Fatal("enable failed")
	}

	_, body = req(readerCookie, http.MethodGet, "/api/v1/plugin-panels?slot=app-detail", "")
	var entries []pluginPanelEntry
	if err := json.Unmarshal([]byte(body), &entries); err != nil || len(entries) != 1 || entries[0].Plugin != "deploy-panel" || entries[0].Panel.ID != "deploys" {
		t.Fatalf("app-detail panels = %s, %v", body, err)
	}
	_, body = req(readerCookie, http.MethodGet, "/api/v1/plugin-panels?slot=dashboard", "")
	if strings.TrimSpace(body) != "[]" {
		t.Fatalf("panel leaked into another slot: %s", body)
	}

	data := "/api/v1/plugins/deploy-panel/panels/deploys/data"
	if code, _ := req(readerCookie, http.MethodGet, "/api/v1/plugins/deploy-panel/panels/nope/data", ""); code != http.StatusNotFound {
		t.Fatalf("unknown panel = %d", code)
	}
	if code, _ := req(readerCookie, http.MethodGet, data+"?app_id=missing-app", ""); code != http.StatusNotFound {
		t.Fatalf("unknown app scope = %d", code)
	}
	if code, _ := req(readerCookie, http.MethodGet, data, ""); code != http.StatusServiceUnavailable {
		t.Fatalf("plugin that is not running = %d", code)
	}

	action := "/api/v1/plugins/deploy-panel/panels/deploys/actions/refresh"
	if rec := serveMobile(rt, httptest.NewRequest(http.MethodPost, action, nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous action = %d", rec.Code)
	}
	if rec := serveMobile(rt, httptest.NewRequest(http.MethodGet, data, nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous data = %d", rec.Code)
	}
	if code, _ := req(readerCookie, http.MethodPost, action, ""); code != http.StatusForbidden {
		t.Fatalf("read-only user pressing a button = %d", code)
	}
	if code, _ := req(admin, http.MethodPost, "/api/v1/plugins/deploy-panel/panels/deploys/actions/nuke", ""); code != http.StatusNotFound && code != http.StatusServiceUnavailable {
		t.Fatalf("undeclared action = %d", code)
	}
}
