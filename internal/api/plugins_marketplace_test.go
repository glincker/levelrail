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
	"time"

	"github.com/GLINCKER/levelrail/internal/plugin"
)

func TestPluginMarketplaceRoutesInstallInOneConfirmedStep(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	rootPub, rootPriv, _ := ed25519.GenerateKey(rand.Reader)
	pubPub, pubPriv, _ := ed25519.GenerateKey(rand.Reader)
	manifest := pluginManifestV("example-notify", "1.0.0", pluginTestDigest, "[read]")
	const manifestURL = "https://plugins.example.com/p/plugin.yaml"
	entry, err := plugin.BuildEntry([]byte(manifest), manifestURL, plugin.Sign(pubPriv, []byte(manifest), pluginTestDigest), pubPub, plugin.Entry{Icon: "slack"})
	if err != nil {
		t.Fatal(err)
	}
	index, err := plugin.SignIndex(rootPriv, plugin.Index{
		Version: 1, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Sequence: 1,
		PublisherKeys: []plugin.IndexKey{{KeyID: entry.PublisherKeyID, Name: "Example Org", PublicKey: base64.StdEncoding.EncodeToString(pubPub)}},
		Plugins:       []plugin.Entry{entry},
	})
	if err != nil {
		t.Fatal(err)
	}
	mk, err := plugin.NewMarketplace(plugin.NewSQLMarketStore(db), rt.plugins, "https://index.example.com/index.json", base64.StdEncoding.EncodeToString(rootPub), "0.13.0")
	if err != nil {
		t.Fatal(err)
	}
	mk.SetFetchers(
		func(context.Context, string, int64, string) (plugin.IndexFetchResult, error) {
			return plugin.IndexFetchResult{Body: index}, nil
		},
		func(context.Context, string, int64) ([]byte, error) { return []byte(manifest), nil },
	)

	call := func(method, path, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, admin, method, path, body))
		return rec.Code, rec.Body.String()
	}
	if code, _ := call(http.MethodGet, "/api/v1/plugin-marketplace", ""); code != http.StatusServiceUnavailable {
		t.Fatalf("before the service is set = %d, want 503", code)
	}
	rt.SetPluginMarketplace(mk)

	code, body := call(http.MethodGet, "/api/v1/plugin-marketplace", "")
	var cat plugin.Catalog
	if err := json.Unmarshal([]byte(body), &cat); err != nil || code != http.StatusOK || len(cat.Entries) != 1 || cat.Sources[0].Status != "ok" {
		t.Fatalf("catalog = %d %s", code, body)
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if code, body := call(http.MethodPost, "/api/v1/plugin-marketplace/preview", js(map[string]string{"source_id": "default", "id": "example-notify"})); code != http.StatusOK || !strings.Contains(body, `"key_trusted":false`) {
		t.Fatalf("preview = %d %s", code, body)
	}
	req := plugin.MarketInstallRequest{SourceID: "default", ID: "example-notify", Version: "1.0.0", Digest: entry.Digest, Abilities: []string{"read"}}
	if code, body := call(http.MethodPost, "/api/v1/plugin-marketplace/install", js(req)); code != http.StatusBadRequest {
		t.Fatalf("install without confirming the key = %d %s, want 400", code, body)
	}
	req.Abilities = []string{"read", "read:sensitive"}
	req.TrustKeyID = entry.PublisherKeyID
	if code, body := call(http.MethodPost, "/api/v1/plugin-marketplace/install", js(req)); code != http.StatusConflict {
		t.Fatalf("install with abilities the sheet did not show = %d %s, want 409", code, body)
	}
	req.Abilities = []string{"read"}
	if code, body := call(http.MethodPost, "/api/v1/plugin-marketplace/install", js(req)); code != http.StatusCreated || !strings.Contains(body, `"state":"enabled"`) || !strings.Contains(body, `"trust_added":true`) {
		t.Fatalf("install = %d %s", code, body)
	}
	if code, body := call(http.MethodGet, "/api/v1/plugin-keys", ""); code != http.StatusOK || !strings.Contains(body, "added from marketplace") {
		t.Fatalf("keys = %d %s", code, body)
	}
	if code, _ := call(http.MethodPost, "/api/v1/plugin-marketplace/sources", js(map[string]string{"url": "http://127.0.0.1/i.json", "root_public_key": base64.StdEncoding.EncodeToString(rootPub)})); code != http.StatusBadRequest {
		t.Fatalf("private source = %d, want 400", code)
	}
	if code, _ := call(http.MethodDelete, "/api/v1/plugin-marketplace/sources/default", ""); code != http.StatusBadRequest {
		t.Fatalf("remove default source = %d, want 400", code)
	}
}
