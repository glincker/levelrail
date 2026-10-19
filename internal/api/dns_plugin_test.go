package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/plugin"
	"github.com/GLINCKER/levelrail/internal/store"
)

const testCFToken = "cf-token-SECRET-xyz" //nolint:gosec // test fixture, not a credential

type tokenResolver struct{ token string }

func (r tokenResolver) Resolve(context.Context, string, string) (string, error) { return r.token, nil }

type fakeDNSPlugin struct {
	mu       sync.Mutex
	zones    []string
	records  []plugin.DNSRecord
	dropData bool
	failList error
	secrets  []string
}

func (f *fakeDNSPlugin) note(secret string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.secrets = append(f.secrets, secret)
}

func (f *fakeDNSPlugin) DNSListZones(_ context.Context, _, secret string) ([]string, error) {
	f.note(secret)
	return f.zones, f.failList
}

func (f *fakeDNSPlugin) DNSListRecords(_ context.Context, _, secret, _ string) ([]plugin.DNSRecord, error) {
	f.note(secret)
	return append([]plugin.DNSRecord{}, f.records...), nil
}

func (f *fakeDNSPlugin) DNSUpsert(_ context.Context, _, secret, _ string, rec plugin.DNSRecord) error {
	f.note(secret)
	if !f.dropData {
		f.records = append(f.records, rec)
	}
	return nil
}

func (f *fakeDNSPlugin) DNSDelete(_ context.Context, _, secret, _ string, rec plugin.DNSRecord) error {
	f.note(secret)
	kept := f.records[:0]
	for _, r := range f.records {
		if r.Name != rec.Name || r.Type != rec.Type || r.Value != rec.Value {
			kept = append(kept, r)
		}
	}
	f.records = kept
	return nil
}

func (f *fakeDNSPlugin) DNSVerify(_ context.Context, _, secret, _ string, rec plugin.DNSRecord) (bool, error) {
	f.note(secret)
	for _, r := range f.records {
		if r.Name == rec.Name && r.Type == rec.Type && r.Value == rec.Value {
			return true, nil
		}
	}
	return false, nil
}

func pluginDNSRouter(t *testing.T, fake *fakeDNSPlugin, choose bool) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.publicHost = "203.0.113.5"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.cloudflareDNSTokenResolver = tokenResolver{testCFToken}
	if err := db.UpdateCloudflareDNSSettings(context.Background(), store.CloudflareDNSSettings{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	rt.domainAuto.pluginDNS = fake
	if choose {
		if err := db.SetDomainAutomationRaw(context.Background(), `{"dns_provider_plugin":"cloudflare-dns"}`); err != nil {
			t.Fatal(err)
		}
	}
	return rt, db
}

func TestPluginChosenRoutesDNSThroughPluginWithPerCallToken(t *testing.T) {
	fake := &fakeDNSPlugin{zones: []string{"example.com."}}
	rt, _ := pluginDNSRouter(t, fake, true)
	res, undo := rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsrecords.OutcomeCreated || res.Zone != "example.com." {
		t.Fatalf("result = %+v", res)
	}
	if len(fake.records) != 1 || fake.records[0].Name != "app" || fake.records[0].Value != "203.0.113.5" {
		t.Fatalf("records = %+v", fake.records)
	}
	if len(undo) != 1 || undo[0].Kind != undoKindDNSRecord {
		t.Fatalf("undo = %+v", undo)
	}
	for _, s := range fake.secrets {
		if s != testCFToken {
			t.Fatalf("every plugin call must carry the stored token: %v", fake.secrets)
		}
	}
}

func TestPluginNotChosenKeepsBuiltInPath(t *testing.T) {
	fake := &fakeDNSPlugin{zones: []string{"example.com."}}
	rt, _ := pluginDNSRouter(t, fake, false)
	if tgt, err := rt.pluginDNSTarget(context.Background(), "app.example.com"); tgt != nil || err != nil {
		t.Fatalf("no plugin chosen must leave the built-in path alone: %v %v", tgt, err)
	}
	if len(fake.secrets) != 0 {
		t.Fatal("the plugin was called although none was chosen")
	}
}

func TestPluginHalfSuccessIsReportedAndUndoable(t *testing.T) {
	fake := &fakeDNSPlugin{zones: []string{"example.com."}, dropData: true}
	rt, _ := pluginDNSRouter(t, fake, true)
	res, undo := rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsResultError || !strings.Contains(res.Message, "could not confirm") {
		t.Fatalf("result = %+v", res)
	}
	if len(undo) != 1 || undo[0].Kind != undoKindDNSRecord || undo[0].Name != "app" {
		t.Fatalf("a record the plugin accepted must stay undoable: %+v", undo)
	}
}

func TestPluginZoneMissAndProviderErrorsAreExplained(t *testing.T) {
	fake := &fakeDNSPlugin{zones: []string{"other.org."}}
	rt, _ := pluginDNSRouter(t, fake, true)
	res, _ := rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsResultSkipped || !strings.Contains(res.Message, "does not manage a zone") {
		t.Fatalf("result = %+v", res)
	}
	fake.failList = errors.New("plugin: not running")
	res, _ = rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsResultError {
		t.Fatalf("result = %+v", res)
	}
}

func TestPluginWithoutCredentialNeverCallsPlugin(t *testing.T) {
	fake := &fakeDNSPlugin{zones: []string{"example.com."}}
	rt, _ := pluginDNSRouter(t, fake, true)
	rt.cloudflareDNSTokenResolver = tokenResolver{""}
	res, _ := rt.applyDomainDNS(context.Background(), nil, "web", "app.example.com", dnsOpts{Mode: dnsModeAuto, CanWrite: true})
	if res.DNS != dnsResultError || len(fake.secrets) != 0 {
		t.Fatalf("result = %+v secrets = %v", res, fake.secrets)
	}
}

func dnsPluginManifest(name, kind string) string {
	return "api_version: 1\nname: " + name + "\nversion: 1.0.0\nkind: " + kind + "\npublisher: Example Org\n" +
		"image: registry.example.com/team/" + name + "@" + pluginTestDigest + "\nabilities: [read]\noutbound_hosts: [api.cloudflare.com]\n"
}

func TestDNSProviderPluginSettingAndTestEndpoint(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	req := func(method, path, body string) (int, string) {
		rec := serveMobile(rt, authedRequest(t, admin, method, path, body))
		return rec.Code, rec.Body.String()
	}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if code, body := req(http.MethodPost, "/api/v1/plugin-keys", js(map[string]string{"name": "ex", "public_key": base64.StdEncoding.EncodeToString(pub)})); code != http.StatusCreated {
		t.Fatalf("trust add = %d %s", code, body)
	}
	install := func(name, kind string) {
		t.Helper()
		m := dnsPluginManifest(name, kind)
		if code, body := req(http.MethodPost, "/api/v1/plugins", js(map[string]string{"manifest": m, "key_id": plugin.Fingerprint(pub), "signature": plugin.Sign(priv, []byte(m), pluginTestDigest)})); code != http.StatusCreated {
			t.Fatalf("install %s = %d %s", name, code, body)
		}
	}
	install("cloudflare-dns", "dns_provider")
	install("chat-notify", "notification_channel")

	if code, body := req(http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":"chat-notify"}`); code != http.StatusBadRequest {
		t.Fatalf("a notification plugin must not be selectable: %d %s", code, body)
	}
	if code, body := req(http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":"missing"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown plugin: %d %s", code, body)
	}
	if code, body := req(http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":"Bad Name"}`); code != http.StatusBadRequest {
		t.Fatalf("malformed name: %d %s", code, body)
	}
	if code, body := req(http.MethodGet, "/api/v1/settings/dns-provider-plugin", ""); code != http.StatusOK || !strings.Contains(body, `"plugin":""`) {
		t.Fatalf("default must be the built-in client: %d %s", code, body)
	}
	if code, body := req(http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":"cloudflare-dns"}`); code != http.StatusOK || !strings.Contains(body, `"plugin":"cloudflare-dns"`) {
		t.Fatalf("select = %d %s", code, body)
	}
	raw, _ := db.GetDomainAutomationRaw(context.Background())
	if !strings.Contains(raw, "cloudflare-dns") {
		t.Fatalf("stored = %s", raw)
	}
	if code, body := req(http.MethodPut, "/api/v1/settings/domain-automation", `{"force_https":false}`); code != http.StatusOK || !strings.Contains(body, `"dns_provider_plugin":"cloudflare-dns"`) {
		t.Fatalf("saving the policy must not drop the plugin choice: %d %s", code, body)
	}
	if code, _ := req(http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":""}`); code != http.StatusOK {
		t.Fatalf("clear = %d", code)
	}

	if code, body := req(http.MethodPost, "/api/v1/plugins/chat-notify/test", ""); code != http.StatusBadRequest {
		t.Fatalf("test on a notification plugin = %d %s", code, body)
	}
	if code, _ := req(http.MethodPost, "/api/v1/plugins/missing/test", ""); code != http.StatusNotFound {
		t.Fatalf("test on a missing plugin = %d", code)
	}
	code, body := req(http.MethodPost, "/api/v1/plugins/cloudflare-dns/test", "")
	if code != http.StatusOK || !strings.Contains(body, `"ok":false`) {
		t.Fatalf("test without a credential = %d %s", code, body)
	}
}

func TestDNSProviderPluginRoutesAuthz(t *testing.T) {
	rt, db, admin := pluginRouter(t)
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})
	readerCookie := sessionCookieForTest(t, rt, reader.ID)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/settings/dns-provider-plugin", `{"plugin":""}`},
		{http.MethodPost, "/api/v1/plugins/x/test", ""},
	} {
		if rec := serveMobile(rt, httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s anonymous = %d, want 401", c.method, c.path, rec.Code)
		}
		if rec := serveMobile(rt, authedRequest(t, readerCookie, c.method, c.path, c.body)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a non-root user = %d, want 403", c.method, c.path, rec.Code)
		}
		if rec := serveMobile(rt, authedRequest(t, admin, c.method, c.path, c.body)); rec.Code == http.StatusForbidden || rec.Code == http.StatusUnauthorized {
			t.Errorf("%s %s as admin = %d", c.method, c.path, rec.Code)
		}
	}
	if rec := serveMobile(rt, authedRequest(t, readerCookie, http.MethodGet, "/api/v1/settings/dns-provider-plugin", "")); rec.Code != http.StatusOK {
		t.Errorf("reading the setting needs only read, got %d", rec.Code)
	}
}
