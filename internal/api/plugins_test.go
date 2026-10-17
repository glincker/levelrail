package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/store"
)

const pluginTestDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000001"

type idleContainers struct{}

func (idleContainers) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return nil, nil
}

func (idleContainers) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return nil, nil
}

func (idleContainers) Create(context.Context, docker.ContainerSpec) (string, error) {
	return "c1", nil
}
func (idleContainers) Start(context.Context, string) error                   { return nil }
func (idleContainers) Stop(context.Context, string, time.Duration) error     { return nil }
func (idleContainers) Remove(context.Context, string, bool) error            { return nil }
func (idleContainers) EnsureNetwork(context.Context, string) (string, error) { return "n", nil }

func pluginRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	experimental.Set(experimental.Plugins)
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	mgr := plugin.NewManager(plugin.NewSQLStore(db), idleContainers{}, PluginAuditor{DB: db}, plugin.Config{Namespace: "ns", DataDir: t.TempDir()}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rt.SetPluginManager(mgr)
	return rt, db, admin
}

func pluginManifest(name string) string {
	return "api_version: 1\nname: " + name + "\nversion: 1.0.0\nkind: notification_channel\npublisher: Example Org\n" +
		"image: registry.example.com/team/" + name + "@" + pluginTestDigest + "\nabilities: [read]\noutbound_hosts: [hooks.example.com]\n"
}

func TestPluginsAreAdminOnly(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)
	calls := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/plugins", ""},
		{http.MethodGet, "/api/v1/plugins/example", ""},
		{http.MethodPost, "/api/v1/plugins", `{}`},
		{http.MethodPost, "/api/v1/plugins/example/approve", `{"abilities":["read"]}`},
		{http.MethodPost, "/api/v1/plugins/example/enable", ""},
		{http.MethodPost, "/api/v1/plugins/example/disable", ""},
		{http.MethodDelete, "/api/v1/plugins/example", ""},
		{http.MethodGet, "/api/v1/plugin-keys", ""},
		{http.MethodPost, "/api/v1/plugin-keys", `{}`},
		{http.MethodDelete, "/api/v1/plugin-keys/ed25519:x", ""},
	}
	for _, c := range calls {
		anon := serveMobile(rt, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body)))
		if anon.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous = %d, want 401", c.method, c.path, anon.Code)
		}
		if rec := serveMobile(rt, authedRequest(t, readerCookie, c.method, c.path, c.body)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a non-admin = %d, want 403", c.method, c.path, rec.Code)
		}
		if rec := serveMobile(rt, authedRequest(t, admin, c.method, c.path, c.body)); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s as admin = %d", c.method, c.path, rec.Code)
		}
	}
}

func TestPluginRoutesFollowTheFlagAndRuntime(t *testing.T) {
	rt, _, admin := pluginRouter(t)
	experimental.Set()
	rec := serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/plugins", ""))
	if rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), ExperimentalDisabledCode) {
		t.Fatalf("flag off = %d %s", rec.Code, rec.Body.String())
	}
	rec = serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/plugin-keys", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("keys with flag off = %d", rec.Code)
	}
	experimental.Set(experimental.Plugins)
	rt.SetPluginManager(nil)
	if rec := serveMobile(rt, authedRequest(t, admin, http.MethodGet, "/api/v1/plugins", "")); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no runtime = %d", rec.Code)
	}
}

func TestPluginInstallApproveEnableFlow(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	manifest := pluginManifest("example-notify")
	req := func(method, path, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, admin, method, path, body))
		return rec.Code, rec.Body.String()
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	install := map[string]string{"manifest": manifest, "key_id": plugin.Fingerprint(pub), "signature": plugin.Sign(priv, []byte(manifest), pluginTestDigest)}

	if code, body := req(http.MethodPost, "/api/v1/plugins", js(install)); code != http.StatusUnprocessableEntity {
		t.Fatalf("install with an empty trust list = %d %s", code, body)
	}
	if code, body := req(http.MethodPost, "/api/v1/plugin-keys", js(map[string]string{"name": "example", "public_key": base64.StdEncoding.EncodeToString(pub)})); code != http.StatusCreated {
		t.Fatalf("trust add = %d %s", code, body)
	}
	if code, body := req(http.MethodPost, "/api/v1/plugin-keys", js(map[string]string{"name": "bad", "public_key": "AAAA"})); code != http.StatusBadRequest {
		t.Fatalf("bad key = %d %s", code, body)
	}
	tampered := map[string]string{"manifest": strings.Replace(manifest, "1.0.0", "9.9.9", 1), "key_id": install["key_id"], "signature": install["signature"]}
	if code, _ := req(http.MethodPost, "/api/v1/plugins", js(tampered)); code != http.StatusUnprocessableEntity {
		t.Fatalf("tampered manifest = %d", code)
	}
	code, body := req(http.MethodPost, "/api/v1/plugins", js(install))
	if code != http.StatusCreated {
		t.Fatalf("install = %d %s", code, body)
	}
	var got pluginResource
	if err := json.Unmarshal([]byte(body), &got); err != nil || got.State != plugin.StateInstalled || len(got.ApprovedAbilities) != 0 || got.Running {
		t.Fatalf("installed plugin = %+v, %v", got, err)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins", js(install)); code != http.StatusConflict {
		t.Fatalf("duplicate install = %d", code)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/example-notify/enable", ""); code != http.StatusConflict {
		t.Fatalf("enable before approval = %d", code)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/example-notify/approve", `{"abilities":["root"]}`); code != http.StatusBadRequest {
		t.Fatalf("approve root = %d", code)
	}
	if code, body := req(http.MethodPost, "/api/v1/plugins/example-notify/approve", `{"abilities":["read"]}`); code != http.StatusOK {
		t.Fatalf("approve = %d %s", code, body)
	}
	if code, body := req(http.MethodPost, "/api/v1/plugins/example-notify/enable", ""); code != http.StatusOK || !strings.Contains(body, `"state":"enabled"`) {
		t.Fatalf("enable = %d %s", code, body)
	}
	if code, _ := req(http.MethodGet, "/api/v1/plugins/missing", ""); code != http.StatusNotFound {
		t.Fatalf("unknown plugin = %d", code)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/example-notify/disable", ""); code != http.StatusOK {
		t.Fatalf("disable = %d", code)
	}
	if code, _ := req(http.MethodDelete, "/api/v1/plugins/example-notify", ""); code != http.StatusNoContent {
		t.Fatalf("remove = %d", code)
	}

	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{ActionPrefixes: []string{"plugin."}})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Action] = true
	}
	for _, want := range []string{plugin.AuditTrustAdd, plugin.AuditInstall, plugin.AuditApprove, plugin.AuditEnable, plugin.AuditDisable, plugin.AuditRemove} {
		if !seen[want] {
			t.Errorf("no audit entry for %s (saw %v)", want, seen)
		}
	}
}

func TestPluginChannelKindValidation(t *testing.T) {
	rt, _, _ := pluginRouter(t)
	if err := rt.validateChannelDestination(context.Background(), "plugin:example-notify", "tok"); err == nil {
		t.Fatal("a channel for a plugin that is not installed was accepted")
	}
	if _, err := validateNotifyKind("plugin:Bad_Name"); err == nil {
		t.Fatal("malformed plugin kind accepted")
	}
	if k, err := validateNotifyKind("plugin:example-notify"); err != nil || !alerting.IsPluginKind(k) {
		t.Fatalf("well formed plugin kind = %q, %v", k, err)
	}
	experimental.Set()
	if _, err := validateNotifyKind("plugin:example-notify"); err == nil {
		t.Fatal("plugin kind accepted while the feature is off")
	}
}
