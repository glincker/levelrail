package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

const pluginTestDigest2 = "sha256:0000000000000000000000000000000000000000000000000000000000000002"

func pluginManifestV(name, version, digest, abilities string) string {
	return "api_version: 1\nname: " + name + "\nversion: " + version + "\nkind: notification_channel\npublisher: Example Org\n" +
		"image: registry.example.com/team/" + name + "@" + digest + "\nabilities: " + abilities + "\noutbound_hosts: []\n" +
		"contributes:\n  panels:\n    - id: info\n      title: Delivery\n      rows:\n        - {label: Target, value: Example chat}\n"
}

func TestPluginInspectInstallFromURLAndUpgrade(t *testing.T) {
	rt, _, admin := pluginRouter(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	req := func(method, path, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, admin, method, path, body))
		return rec.Code, rec.Body.String()
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if code, body := req(http.MethodPost, "/api/v1/plugin-keys", js(map[string]string{"name": "example", "public_key": base64.StdEncoding.EncodeToString(pub)})); code != http.StatusCreated {
		t.Fatalf("trust key = %d %s", code, body)
	}

	v1 := pluginManifestV("example-notify", "1.0.0", pluginTestDigest, "[read]")
	files := map[string]string{
		"https://plugins.example.com/p/plugin.yaml":     v1,
		"https://plugins.example.com/p/plugin.yaml.sig": plugin.Sign(priv, []byte(v1), pluginTestDigest) + "\n",
		"https://plugins.example.com/u/plugin.yaml":     v1,
	}
	rt.pluginFetch = func(ctx context.Context, u string, limit int64) ([]byte, error) {
		if !strings.HasPrefix(u, "https://") {
			return plugin.Fetch(ctx, u, limit)
		}
		if body, ok := files[u]; ok {
			return []byte(body), nil
		}
		return nil, plugin.ErrSourceNotFound
	}

	code, body := req(http.MethodPost, "/api/v1/plugins/inspect", js(map[string]string{"manifest_url": "https://plugins.example.com/p/plugin.yaml"}))
	var pv plugin.Preview
	if err := json.Unmarshal([]byte(body), &pv); err != nil || code != http.StatusOK || pv.Action != "install" || !pv.Verification.Installable || pv.Verification.Status != plugin.VerifyVerified {
		t.Fatalf("inspect = %d %s", code, body)
	}
	if len(pv.Manifest.Contributes.Panels) != 1 {
		t.Fatalf("preview lost the contributed panel: %s", body)
	}
	if code, body := req(http.MethodGet, "/api/v1/plugins", ""); code != http.StatusOK || strings.Contains(body, "example-notify") {
		t.Fatalf("inspect must not install anything: %d %s", code, body)
	}

	code, body = req(http.MethodPost, "/api/v1/plugins/inspect", js(map[string]string{"manifest_url": "https://plugins.example.com/u/plugin.yaml"}))
	if err := json.Unmarshal([]byte(body), &pv); err != nil || code != http.StatusOK || pv.Verification.Installable || pv.Verification.Status != plugin.VerifyUnsignedBlock {
		t.Fatalf("unsigned inspect = %d %s", code, body)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins", js(map[string]string{"manifest_url": "https://plugins.example.com/u/plugin.yaml"})); code != http.StatusUnprocessableEntity {
		t.Fatalf("unsigned install = %d, want 422", code)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins", js(map[string]string{"manifest_url": "http://plugins.example.com/p/plugin.yaml"})); code != http.StatusBadRequest {
		t.Fatalf("non https install = %d, want 400", code)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins", js(map[string]any{"manifest": v1, "manifest_url": "https://plugins.example.com/p/plugin.yaml"})); code != http.StatusBadRequest {
		t.Fatalf("both sources = %d, want 400", code)
	}

	if code, body := req(http.MethodPost, "/api/v1/plugins", js(map[string]any{"manifest_url": "https://plugins.example.com/p/plugin.yaml", "approve_abilities": []string{"read", "read:sensitive"}})); code != http.StatusBadRequest {
		t.Fatalf("approval beyond the request = %d %s, want 400", code, body)
	}
	code, body = req(http.MethodPost, "/api/v1/plugins", js(map[string]any{"manifest_url": "https://plugins.example.com/p/plugin.yaml", "approve_abilities": []string{"read"}}))
	var got pluginResource
	if err := json.Unmarshal([]byte(body), &got); err != nil || code != http.StatusCreated || got.ApprovedAt == "" || got.Verification != plugin.VerifyVerified || len(got.Panels) != 1 {
		t.Fatalf("install with approval = %d %s", code, body)
	}

	v2 := pluginManifestV("example-notify", "2.0.0", pluginTestDigest2, "[read, read:sensitive]")
	upgrade := map[string]string{"manifest": v2, "signature": plugin.Sign(priv, []byte(v2), pluginTestDigest2)}
	code, body = req(http.MethodPost, "/api/v1/plugins/inspect", js(upgrade))
	if err := json.Unmarshal([]byte(body), &pv); err != nil || code != http.StatusOK || pv.Action != "upgrade" || !pv.ReapprovalRequired || len(pv.AddedAbilities) != 1 {
		t.Fatalf("upgrade inspect = %d %s", code, body)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/other/upgrade", js(upgrade)); code != http.StatusBadRequest {
		t.Fatalf("upgrade under the wrong name = %d, want 400", code)
	}
	code, body = req(http.MethodPost, "/api/v1/plugins/example-notify/upgrade", js(upgrade))
	if err := json.Unmarshal([]byte(body), &got); err != nil || code != http.StatusOK || got.Version != "2.0.0" || got.ApprovedAt != "" || got.Fix != plugin.FixApprove {
		t.Fatalf("widening upgrade = %d %s, want approval cleared and the approve fix", code, body)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/example-notify/restart", ""); code != http.StatusConflict {
		t.Fatalf("restart of a stopped plugin = %d, want 409", code)
	}
}
