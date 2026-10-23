package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficexport"
)

const (
	texToken = "phc_super_secret_token_do_not_leak" //nolint:gosec // fake fixture, not a real credential
	texURL   = "https://hooks.example.com/in/SECRETPATH123"
	texAuth  = "Bearer authsecret456" //nolint:gosec // fake fixture, not a real credential
)

type fakeExporter struct {
	reloads int
	result  trafficexport.TestResult
	tested  []trafficexport.SinkSpec
}

func (f *fakeExporter) Reload(context.Context) error { f.reloads++; return nil }
func (f *fakeExporter) Status() []trafficexport.SinkStatus {
	return []trafficexport.SinkStatus{{ID: "x", Name: "n", Kind: "webhook", Enabled: true, Active: true, Dropped: 3}}
}
func (f *fakeExporter) Test(_ context.Context, s trafficexport.SinkSpec) trafficexport.TestResult {
	f.tested = append(f.tested, s)
	return f.result
}

type texHarness struct {
	t        *testing.T
	rt       *Router
	db       *store.DB
	secrets  *fakeEmailSecretsStore
	exporter *fakeExporter
	cookie   *http.Cookie
	logs     *bytes.Buffer
}

func newTexHarness(t *testing.T, withSecrets bool) *texHarness {
	t.Helper()
	db := openTestDB(t)
	logs := &bytes.Buffer{}
	h := &texHarness{t: t, db: db, logs: logs, secrets: newFakeEmailSecretsStore(), exporter: &fakeExporter{result: trafficexport.TestResult{OK: true}}}
	var opts []Option
	if withSecrets {
		opts = append(opts, WithTrafficExport(h.secrets, h.exporter))
	}
	h.rt = NewRouter(slog.New(slog.NewTextHandler(logs, nil)), testBrand(), db, opts...)
	h.cookie = loginTestSession(t, h.rt, db)
	return h
}

func (h *texHarness) do(cookie *http.Cookie, method, target, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, authedRequest(h.t, cookie, method, target, body))
	return rec
}

func (h *texHarness) mustDecode(rec *httptest.ResponseRecorder, want int, into any) {
	h.t.Helper()
	if rec.Code != want {
		h.t.Fatalf("status = %d, want %d, body = %s", rec.Code, want, rec.Body.String())
	}
	if into != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
			h.t.Fatalf("decode: %v", err)
		}
	}
}

func (h *texHarness) noLeak(bodies ...string) {
	h.t.Helper()
	for _, s := range append(bodies, h.logs.String()) {
		for _, secret := range []string{texToken, "SECRETPATH123", "authsecret456"} {
			if strings.Contains(s, secret) {
				h.t.Fatalf("credential %q leaked into output: %q", secret, s)
			}
		}
	}
}

const (
	texPostHogBody  = `{"name":"Product analytics","kind":"posthog","host":"eu","token":"` + texToken + `"}`
	texWebhookBody  = `{"name":"Warehouse","kind":"webhook","url":"` + texURL + `","auth_header":"` + texAuth + `","fields":["client_ip"]}`
	texTargetHostWH = "hooks.example.com"
)

func TestTrafficExport_CreateNeverReturnsOrLogsSecrets(t *testing.T) {
	h := newTexHarness(t, true)
	rec := h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", texWebhookBody)
	var got trafficExportSinkResource
	h.mustDecode(rec, http.StatusCreated, &got)
	if !got.URLSet || !got.AuthHeaderSet || got.URL != "" || got.AuthHeader != "" || got.Token != "" {
		t.Fatalf("response = %+v, want only the *_set flags", got)
	}
	if got.TargetHost != texTargetHostWH || got.IPTruncate == nil || !*got.IPTruncate || !got.IdentityEnabled {
		t.Errorf("response = %+v, want host only, truncation on by default and identity flagged", got)
	}
	h.noLeak(rec.Body.String())

	list := h.do(h.cookie, http.MethodGet, "/api/v1/settings/traffic-export/sinks", "")
	h.mustDecode(list, http.StatusOK, nil)
	h.noLeak(list.Body.String())
	if h.exporter.reloads != 1 {
		t.Errorf("reloads = %d, want 1", h.exporter.reloads)
	}
	if h.secrets.values[store.TrafficExportSecretsKey(got.ID)+"|url"] != texURL {
		t.Errorf("url not stored under the per-sink namespace: %v", h.secrets.values)
	}
}

func TestTrafficExport_DefaultsExportNoIdentity(t *testing.T) {
	h := newTexHarness(t, true)
	var got trafficExportSinkResource
	h.mustDecode(h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", texPostHogBody), http.StatusCreated, &got)
	if len(got.Fields) != 0 || got.IdentityEnabled || got.Enabled == nil || !*got.Enabled || got.Sample == nil || *got.Sample != 1 {
		t.Fatalf("got = %+v, want no opt-in fields, enabled, sample 1", got)
	}
	if got.TargetHost != "eu.i.posthog.com" {
		t.Errorf("target host = %q", got.TargetHost)
	}
}

func TestTrafficExport_UpdateKeepsSecretsAndRejectsKindChange(t *testing.T) {
	h := newTexHarness(t, true)
	var created trafficExportSinkResource
	h.mustDecode(h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", texWebhookBody), http.StatusCreated, &created)
	path := "/api/v1/settings/traffic-export/sinks/" + created.ID

	var updated trafficExportSinkResource
	h.mustDecode(h.do(h.cookie, http.MethodPut, path, `{"name":"Warehouse 2","fields":[],"enabled":false}`), http.StatusOK, &updated)
	if updated.Name != "Warehouse 2" || len(updated.Fields) != 0 || updated.IdentityEnabled || !updated.URLSet || !updated.AuthHeaderSet {
		t.Fatalf("updated = %+v, want fields cleared and credentials kept", updated)
	}
	if h.secrets.values[store.TrafficExportSecretsKey(created.ID)+"|url"] != texURL {
		t.Error("an update without a url must keep the stored one")
	}
	if rec := h.do(h.cookie, http.MethodPut, path, `{"kind":"posthog"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("kind change status = %d, want 400", rec.Code)
	}
}

func TestTrafficExport_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing name", `{"kind":"posthog","token":"t"}`, http.StatusBadRequest},
		{"unknown kind", `{"name":"a","kind":"carrier-pigeon"}`, http.StatusBadRequest},
		{"unknown field", `{"name":"a","kind":"posthog","token":"t","fields":["cookies"]}`, http.StatusBadRequest},
		{"sample above one", `{"name":"a","kind":"posthog","token":"t","sample":1.5}`, http.StatusBadRequest},
		{"posthog without token", `{"name":"a","kind":"posthog"}`, http.StatusBadRequest},
		{"webhook url must be http", `{"name":"a","kind":"webhook","url":"ftp://example.com/x"}`, http.StatusBadRequest},
		{"loopback target blocked", `{"name":"a","kind":"webhook","url":"http://127.0.0.1:9/x"}`, http.StatusBadRequest},
		{"token on a webhook", `{"name":"a","kind":"webhook","url":"https://example.com/x","token":"t"}`, http.StatusBadRequest},
		{"valid", `{"name":"a","kind":"webhook","url":"https://example.com/x"}`, http.StatusCreated},
		{"duplicate name", `{"name":"a","kind":"webhook","url":"https://example.com/y"}`, http.StatusConflict},
	}
	h := newTexHarness(t, true)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", tt.body)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestTrafficExport_ErrorsDoNotEchoSecretURL(t *testing.T) {
	h := newTexHarness(t, true)
	rec := h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", `{"name":"a","kind":"webhook","url":"http://127.0.0.1/SECRETPATH123"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	h.noLeak(rec.Body.String())
}

func TestTrafficExport_DeleteRemovesSecretsAndReloads(t *testing.T) {
	h := newTexHarness(t, true)
	var created trafficExportSinkResource
	h.mustDecode(h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", texPostHogBody), http.StatusCreated, &created)
	rec := h.do(h.cookie, http.MethodDelete, "/api/v1/settings/traffic-export/sinks/"+created.ID, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(h.secrets.values) != 0 {
		t.Errorf("secrets left behind: %v", h.secrets.values)
	}
	if rec := h.do(h.cookie, http.MethodDelete, "/api/v1/settings/traffic-export/sinks/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Errorf("second delete status = %d, want 404", rec.Code)
	}
}

func TestTrafficExport_TestDeliveryAndStatus(t *testing.T) {
	h := newTexHarness(t, true)
	var created trafficExportSinkResource
	h.mustDecode(h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks", texWebhookBody), http.StatusCreated, &created)

	h.exporter.result = trafficexport.TestResult{ErrorKind: trafficexport.KindAuth}
	var res trafficexport.TestResult
	rec := h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks/"+created.ID+"/test", "")
	h.mustDecode(rec, http.StatusOK, &res)
	if res.OK || res.ErrorKind != trafficexport.KindAuth {
		t.Fatalf("result = %+v, want a failed delivery with kind auth", res)
	}
	if len(h.exporter.tested) != 1 || h.exporter.tested[0].Secrets[trafficexport.SecretURL] != texURL {
		t.Errorf("the test did not get the stored url: %+v", h.exporter.tested)
	}
	h.noLeak(rec.Body.String())

	var st trafficExportStatusResponse
	h.mustDecode(h.do(h.cookie, http.MethodGet, "/api/v1/settings/traffic-export/status", ""), http.StatusOK, &st)
	if len(st.Sinks) != 1 || st.Sinks[0].Dropped != 3 {
		t.Errorf("status = %+v", st)
	}
	if rec := h.do(h.cookie, http.MethodPost, "/api/v1/settings/traffic-export/sinks/missing/test", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown sink test status = %d, want 404", rec.Code)
	}
}

func TestTrafficExport_WithoutMasterKey(t *testing.T) {
	h := newTexHarness(t, false)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/settings/traffic-export/sinks", texPostHogBody},
		{http.MethodGet, "/api/v1/settings/traffic-export/status", ""},
	} {
		if rec := h.do(h.cookie, c.method, c.path, c.body); rec.Code != http.StatusNotImplemented {
			t.Errorf("%s %s = %d, want 501", c.method, c.path, rec.Code)
		}
	}
	if rec := h.do(h.cookie, http.MethodGet, "/api/v1/settings/traffic-export/sinks", ""); rec.Code != http.StatusOK {
		t.Errorf("list without a master key = %d, want 200", rec.Code)
	}
}

func TestTrafficExport_ReadIsNotRoot(t *testing.T) {
	h := newTexHarness(t, true)
	viewer := store.User{ID: "user_viewer", Email: "viewer@example.com", DisplayName: "V", Abilities: []string{AbilityRead}, CreatedAt: time.Now()}
	if err := h.db.CreateUser(context.Background(), viewer); err != nil {
		t.Fatal(err)
	}
	vc := sessionCookieForTest(t, h.rt, viewer.ID)
	if rec := h.do(vc, http.MethodGet, "/api/v1/settings/traffic-export/sinks", ""); rec.Code != http.StatusOK {
		t.Errorf("viewer list = %d, want 200", rec.Code)
	}
	if rec := h.do(vc, http.MethodGet, "/api/v1/settings/traffic-export/status", ""); rec.Code != http.StatusOK {
		t.Errorf("viewer status = %d, want 200", rec.Code)
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/settings/traffic-export/sinks", texPostHogBody},
		{http.MethodPut, "/api/v1/settings/traffic-export/sinks/x", `{}`},
		{http.MethodDelete, "/api/v1/settings/traffic-export/sinks/x", ""},
		{http.MethodPost, "/api/v1/settings/traffic-export/sinks/x/test", ""},
	} {
		if rec := h.do(vc, c.method, c.path, c.body); rec.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s = %d, want 403", c.method, c.path, rec.Code)
		}
	}
}
