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

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	provSecretA = "prov-secret-AAAA-do-not-leak" //nolint:gosec // fake fixture, not a real credential
	provSecretB = "prov-secret-BBBB-do-not-leak" //nolint:gosec // fake fixture, not a real credential
)

type providerHarness struct {
	t       *testing.T
	rt      *Router
	db      *store.DB
	secrets *fakeEmailSecretsStore
	cookie  *http.Cookie
	logs    *bytes.Buffer
	sender  *recordingEmailSender
}

type recordingEmailSender struct {
	only []string
}

func (s *recordingEmailSender) Send(ctx context.Context, _, _, _ string) error {
	s.only = append(s.only, email.OnlyProviderID(ctx))
	return nil
}

func newProviderHarness(t *testing.T) *providerHarness {
	t.Helper()
	db := openTestDB(t)
	logs := &bytes.Buffer{}
	secrets := newFakeEmailSecretsStore()
	sender := &recordingEmailSender{}
	rt := NewRouter(slog.New(slog.NewTextHandler(logs, nil)), testBrand(), db, WithEmailSecrets(secrets), WithEmailSender(sender))
	return &providerHarness{t: t, rt: rt, db: db, secrets: secrets, cookie: loginTestSession(t, rt, db), logs: logs, sender: sender}
}

func (h *providerHarness) do(method, target, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	rec := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(rec, authedRequest(h.t, h.cookie, method, target, body))
	return rec
}

func (h *providerHarness) decode(rec *httptest.ResponseRecorder, want int, into any) {
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

func (h *providerHarness) create(body string) emailProviderResource {
	h.t.Helper()
	var p emailProviderResource
	h.decode(h.do(http.MethodPost, "/api/v1/settings/email/providers", body), http.StatusCreated, &p)
	return p
}

func (h *providerHarness) list() []emailProviderResource {
	h.t.Helper()
	var out emailProviderListResponse
	h.decode(h.do(http.MethodGet, "/api/v1/settings/email/providers", ""), http.StatusOK, &out)
	return out.Providers
}

func (h *providerHarness) assertNoSecretLeak(bodies ...string) {
	h.t.Helper()
	for _, s := range append(bodies, h.logs.String()) {
		if strings.Contains(s, provSecretA) || strings.Contains(s, provSecretB) {
			h.t.Fatalf("a credential value leaked into output: %q", s)
		}
	}
}

const (
	resendBody = `{"name":"Resend","backend":"resend","resend_from":"a@example.com","resend_api_key":"` + provSecretA + `"}`
	smtpBody   = `{"name":"Relay","backend":"smtp","smtp_host":"smtp.example.com","smtp_port":587,"smtp_from":"b@example.com","smtp_password":"` + provSecretB + `"}`
)

func TestEmailProviders_CRUDAndOnePrimaryInvariant(t *testing.T) {
	h := newProviderHarness(t)

	first := h.create(resendBody)
	if first.Role != store.EmailRolePrimary || first.Enabled == nil || !*first.Enabled || !first.ResendAPIKeySet || first.ResendAPIKey != "" {
		t.Fatalf("first = %+v, want an enabled primary with only the *_set flag", first)
	}
	if got := h.secrets.values[store.EmailSecretResendAPIKey]; got != provSecretA {
		t.Errorf("primary secret not in the shared namespace: %q", got)
	}
	second := h.create(smtpBody)
	if second.Role != store.EmailRoleBackup || !second.SMTPPasswordSet {
		t.Fatalf("second = %+v, want a backup", second)
	}
	if got := h.secrets.values["email-settings/"+second.ID+"|"+store.EmailSecretSMTPPassword]; got != provSecretB {
		t.Errorf("backup secret not in its own namespace: %q", got)
	}
	if _, leaked := h.secrets.values[store.EmailSecretSMTPPassword]; leaked {
		t.Error("backup secret leaked into the shared namespace")
	}

	if rec := h.do(http.MethodPost, "/api/v1/settings/email/providers", resendBody); rec.Code != http.StatusConflict {
		t.Errorf("third provider status = %d, want 409", rec.Code)
	}

	got := h.list()
	if len(got) != 2 || got[0].ID != first.ID || got[1].ID != second.ID {
		t.Fatalf("list = %+v, want primary then backup", got)
	}
	var one emailProviderResource
	rec := h.do(http.MethodGet, "/api/v1/settings/email/providers/"+second.ID, "")
	h.decode(rec, http.StatusOK, &one)
	h.assertNoSecretLeak(rec.Body.String())

	update := `{"name":"Relay 2","backend":"smtp","smtp_host":"smtp2.example.com","smtp_port":2525,"smtp_from":"b@example.com","enabled":false}`
	var updated emailProviderResource
	rec = h.do(http.MethodPut, "/api/v1/settings/email/providers/"+second.ID, update)
	h.decode(rec, http.StatusOK, &updated)
	if updated.Role != store.EmailRoleBackup || updated.SMTPHost != "smtp2.example.com" || updated.Enabled == nil || *updated.Enabled || !updated.SMTPPasswordSet {
		t.Errorf("updated = %+v, want role kept, fields replaced, disabled, stored password kept", updated)
	}
	h.assertNoSecretLeak(rec.Body.String())

	if rec := h.do(http.MethodGet, "/api/v1/settings/email/providers/nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown get status = %d, want 404", rec.Code)
	}
	if rec := h.do(http.MethodDelete, "/api/v1/settings/email/providers/"+first.ID, ""); rec.Code != http.StatusConflict {
		t.Errorf("delete primary with backup status = %d, want 409", rec.Code)
	}
	if rec := h.do(http.MethodDelete, "/api/v1/settings/email/providers/"+second.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete backup status = %d, want 204", rec.Code)
	}
	if _, left := h.secrets.values["email-settings/"+second.ID+"|"+store.EmailSecretSMTPPassword]; left {
		t.Error("backup credentials survived its deletion")
	}
	if rec := h.do(http.MethodDelete, "/api/v1/settings/email/providers/"+first.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete primary status = %d, want 204", rec.Code)
	}
	if _, left := h.secrets.values[store.EmailSecretResendAPIKey]; left {
		t.Error("primary credentials survived its deletion")
	}
	if s, _ := h.db.GetEmailSettings(context.Background()); s.Backend != "" {
		t.Errorf("email_settings backend = %q after deleting the primary, want empty", s.Backend)
	}
}

func TestEmailProviders_Validation(t *testing.T) {
	h := newProviderHarness(t)
	cases := map[string]string{
		"missing backend":   `{"name":"x"}`,
		"unknown backend":   `{"backend":"carrier-pigeon"}`,
		"smtp without host": `{"backend":"smtp","smtp_port":587,"smtp_from":"a@example.com"}`,
		"smtp without from": `{"backend":"smtp","smtp_host":"h","smtp_port":587}`,
		"bad tls mode":      `{"backend":"smtp","smtp_host":"h","smtp_port":587,"smtp_from":"a@example.com","tls_mode":"rot13"}`,
		"bad ses region":    `{"backend":"ses","ses_region":"ses.example.com","ses_access_key_id":"AK","ses_from":"a@example.com"}`,
		"long name":         `{"backend":"resend","resend_from":"a@example.com","name":"` + strings.Repeat("n", 200) + `"}`,
	}
	for name, body := range cases {
		rec := h.do(http.MethodPost, "/api/v1/settings/email/providers", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400, body = %s", name, rec.Code, rec.Body.String())
		}
	}
	if got := h.list(); len(got) != 0 {
		t.Errorf("rejected creates left %d providers", len(got))
	}
	if rec := h.do(http.MethodPost, "/api/v1/settings/email/providers", `{not json`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad json status = %d, want 400", rec.Code)
	}
}

func TestEmailProviders_PromoteSwapsRolesAndCredentials(t *testing.T) {
	h := newProviderHarness(t)
	primary := h.create(resendBody)
	backup := h.create(smtpBody)

	rec := h.do(http.MethodPost, "/api/v1/settings/email/providers/"+backup.ID+"/promote", "")
	var out emailProviderListResponse
	h.decode(rec, http.StatusOK, &out)
	if len(out.Providers) != 2 || out.Providers[0].ID != backup.ID || out.Providers[0].Role != store.EmailRolePrimary || out.Providers[1].ID != primary.ID {
		t.Fatalf("after promote = %+v", out.Providers)
	}
	if !out.Providers[0].SMTPPasswordSet || !out.Providers[1].ResendAPIKeySet {
		t.Errorf("credential flags did not follow the roles: %+v", out.Providers)
	}
	h.assertNoSecretLeak(rec.Body.String())

	if got := h.secrets.values[store.EmailSecretSMTPPassword]; got != provSecretB {
		t.Errorf("new primary's credential not in the shared namespace: %q", got)
	}
	if _, stale := h.secrets.values[store.EmailSecretResendAPIKey]; stale {
		t.Error("old primary's credential still in the shared namespace")
	}
	if got := h.secrets.values["email-settings/"+primary.ID+"|"+store.EmailSecretResendAPIKey]; got != provSecretA {
		t.Errorf("old primary's credential not in its own namespace: %q", got)
	}
	if _, stale := h.secrets.values["email-settings/"+backup.ID+"|"+store.EmailSecretSMTPPassword]; stale {
		t.Error("promoted provider's old namespace was not cleared")
	}
	if s, _ := h.db.GetEmailSettings(context.Background()); s.Backend != store.EmailBackendSMTP || s.SMTPHost != "smtp.example.com" {
		t.Errorf("email_settings not mirrored to the new primary: %+v", s)
	}

	if rec := h.do(http.MethodPost, "/api/v1/settings/email/providers/"+backup.ID+"/promote", ""); rec.Code != http.StatusConflict {
		t.Errorf("promote the primary status = %d, want 409", rec.Code)
	}
	if rec := h.do(http.MethodPost, "/api/v1/settings/email/providers/nope/promote", ""); rec.Code != http.StatusNotFound {
		t.Errorf("promote unknown status = %d, want 404", rec.Code)
	}
}

func TestEmailProviders_ShimKeepsPrimaryInSync(t *testing.T) {
	h := newProviderHarness(t)

	shimPut := `{"backend":"smtp","from_address":"ops@example.com","smtp_host":"smtp.example.com","smtp_port":587,"smtp_password":"` + provSecretA + `"}`
	rec := h.do(http.MethodPut, "/api/v1/settings/email", shimPut)
	h.decode(rec, http.StatusOK, nil)
	h.assertNoSecretLeak(rec.Body.String())

	got := h.list()
	if len(got) != 1 || got[0].Role != store.EmailRolePrimary || got[0].Backend != "smtp" || got[0].SMTPHost != "smtp.example.com" || !got[0].SMTPPasswordSet {
		t.Fatalf("shim write did not produce a primary provider: %+v", got)
	}
	var settings emailSettingsResource
	h.decode(h.do(http.MethodGet, "/api/v1/settings/email", ""), http.StatusOK, &settings)
	if settings.Backend != "smtp" || settings.FromAddress != "ops@example.com" || !settings.SMTPPasswordSet {
		t.Errorf("shim read = %+v", settings)
	}

	h.create(resendBody)
	rec = h.do(http.MethodPut, "/api/v1/settings/email", `{"backend":""}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("clearing the primary under a backup status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}

	pid := got[0].ID
	update := `{"backend":"smtp","smtp_host":"moved.example.com","smtp_port":465,"smtp_from":"ops@example.com"}`
	h.decode(h.do(http.MethodPut, "/api/v1/settings/email/providers/"+pid, update), http.StatusOK, nil)
	h.decode(h.do(http.MethodGet, "/api/v1/settings/email", ""), http.StatusOK, &settings)
	if settings.SMTPHost != "moved.example.com" || settings.SMTPPort != 465 {
		t.Errorf("provider write not visible through the shim: %+v", settings)
	}

	// Remove the backup, then the shim may clear the primary.
	all := h.list()
	h.decode(h.do(http.MethodDelete, "/api/v1/settings/email/providers/"+all[1].ID, ""), http.StatusNoContent, nil)
	h.decode(h.do(http.MethodPut, "/api/v1/settings/email", `{"backend":""}`), http.StatusOK, nil)
	if left := h.list(); len(left) != 0 {
		t.Errorf("clearing the backend left providers: %+v", left)
	}
}

func TestEmailProviders_ShimAdoptsLegacyBackendOnFirstProviderCreate(t *testing.T) {
	h := newProviderHarness(t)
	if err := h.db.UpdateEmailSettings(context.Background(), store.EmailSettings{
		Backend: store.EmailBackendResend, ResendFrom: "legacy@example.com", SESSMTPPort: 587, MailgunRegion: "us",
	}); err != nil {
		t.Fatal(err)
	}
	b := h.create(smtpBody)
	got := h.list()
	if b.Role != store.EmailRoleBackup || len(got) != 2 || got[0].Backend != "resend" {
		t.Errorf("legacy backend not adopted as primary: %+v", got)
	}
}

func TestEmailProviders_TestSendIsAimedAtOneProvider(t *testing.T) {
	h := newProviderHarness(t)
	h.create(resendBody)
	backup := h.create(smtpBody)

	rec := h.do(http.MethodPost, "/api/v1/settings/email/providers/"+backup.ID+"/test", `{"to":"me@example.com"}`)
	var out testEmailResponse
	h.decode(rec, http.StatusOK, &out)
	if out.ProviderID != backup.ID || len(h.sender.only) != 1 || h.sender.only[0] != backup.ID {
		t.Errorf("test send not pinned to the provider: resp %+v, sender saw %v", out, h.sender.only)
	}
	if rec := h.do(http.MethodPost, "/api/v1/settings/email/providers/nope/test", `{"to":"me@example.com"}`); rec.Code != http.StatusNotFound {
		t.Errorf("unknown provider test status = %d, want 404", rec.Code)
	}
}

func TestEmailProviders_AuthorizationTiers(t *testing.T) {
	h := newProviderHarness(t)
	p := h.create(resendBody)

	tokens := map[string][]string{"read": {AbilityRead}, "write": {AbilityWrite}, "root": {AbilityRoot}}
	plain := map[string]string{}
	for name, abilities := range tokens {
		plain[name] = "tok-" + name + "-plaintext"
		if err := h.db.SaveAPIToken(context.Background(), store.APIToken{
			ID: "tok_" + name, Name: name, TokenHash: hashToken(plain[name]), Abilities: abilities, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	call := func(tok, method, target, body string) int {
		rec := httptest.NewRecorder()
		h.rt.Handler().ServeHTTP(rec, bearerRequest(method, target, body, plain[tok]))
		return rec.Code
	}
	base := "/api/v1/settings/email/providers"
	for _, tok := range []string{"read", "root"} {
		if code := call(tok, http.MethodGet, base, ""); code != http.StatusOK {
			t.Errorf("%s token list = %d, want 200", tok, code)
		}
		if code := call(tok, http.MethodGet, base+"/"+p.ID, ""); code != http.StatusOK {
			t.Errorf("%s token get = %d, want 200", tok, code)
		}
	}
	writes := []struct{ method, path, body string }{
		{http.MethodPost, base, resendBody},
		{http.MethodPut, base + "/" + p.ID, resendBody},
		{http.MethodDelete, base + "/" + p.ID, ""},
		{http.MethodPost, base + "/" + p.ID + "/test", `{"to":"me@example.com"}`},
		{http.MethodPost, base + "/" + p.ID + "/promote", ""},
	}
	for _, w := range writes {
		for _, tok := range []string{"read", "write"} {
			if code := call(tok, w.method, w.path, w.body); code != http.StatusForbidden {
				t.Errorf("%s token %s %s = %d, want 403", tok, w.method, w.path, code)
			}
		}
	}
	if code := call("root", http.MethodPost, base+"/"+p.ID+"/test", `{"to":"me@example.com"}`); code != http.StatusOK {
		t.Errorf("root token test = %d, want 200", code)
	}
	if code := h.do(http.MethodGet, base, "").Code; code != http.StatusOK {
		t.Errorf("session list = %d", code)
	}
	unauth := httptest.NewRecorder()
	h.rt.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, base, nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated list = %d, want 401", unauth.Code)
	}
}

func TestEmailProviders_AuditNamesFieldsNeverValues(t *testing.T) {
	h := newProviderHarness(t)
	p := h.create(resendBody)

	entries, err := h.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: store.AuditActionEmailSecretsChanged})
	if err != nil || len(entries) != 1 {
		t.Fatalf("audit entries = %d, err = %v, want one secrets-changed entry", len(entries), err)
	}
	if !strings.Contains(entries[0].Detail, store.EmailSecretResendAPIKey) || !strings.Contains(entries[0].Detail, p.ID) {
		t.Errorf("detail = %q, want provider id and field name", entries[0].Detail)
	}
	h.assertNoSecretLeak(entries[0].Detail, entries[0].Path)

	// An update that carries no credential writes no secrets audit entry.
	h.decode(h.do(http.MethodPut, "/api/v1/settings/email/providers/"+p.ID, `{"backend":"resend","resend_from":"z@example.com"}`), http.StatusOK, nil)
	entries, _ = h.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{Action: store.AuditActionEmailSecretsChanged})
	if len(entries) != 1 {
		t.Errorf("credential-free update added audit entries: %d", len(entries))
	}
}

func TestEmailProviders_NoMasterKey(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/settings/email/providers", resendBody))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("create without a secrets store = %d, want 501", rec.Code)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/email/providers", ""))
	if rec.Code != http.StatusOK {
		t.Errorf("list without a secrets store = %d, want 200", rec.Code)
	}
}

type emptyDNS struct{}

func (emptyDNS) LookupTXT(context.Context, string) ([]string, error) { return nil, nil }
func (emptyDNS) LookupCNAME(context.Context, string) (string, error) { return "", nil }

func TestEmailProviders_DomainCheckCanTargetOneProvider(t *testing.T) {
	h := newProviderHarness(t)
	h.rt.emailDNS = emptyDNS{}
	h.create(resendBody)
	backup := h.create(smtpBody)

	var out struct {
		Domain   string `json:"domain"`
		Provider string `json:"provider"`
	}
	h.decode(h.do(http.MethodGet, "/api/v1/settings/email/domain-check?provider="+backup.ID, ""), http.StatusOK, &out)
	if out.Domain != "example.com" || out.Provider != "smtp" {
		t.Errorf("domain check = %+v, want example.com for smtp", out)
	}
	if rec := h.do(http.MethodGet, "/api/v1/settings/email/domain-check?provider=nope", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown provider status = %d, want 404", rec.Code)
	}
}
