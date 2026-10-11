package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestValidateIngressSettingsRequest covers the one real business rule
// this endpoint owns: a syntactically valid, non-empty ACME email is
// required whenever acme_enabled is true, and is not required (or even
// checked) otherwise.
func TestValidateIngressSettingsRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     ingressSettingsResource
		wantErr error
	}{
		{
			name: "acme disabled, no email needed",
			req:  ingressSettingsResource{ACMEEnabled: false},
		},
		{
			name: "acme disabled, malformed email still accepted (unchecked)",
			req:  ingressSettingsResource{ACMEEnabled: false, ACMEEmail: "not-an-email"},
		},
		{
			name:    "acme enabled, empty email rejected",
			req:     ingressSettingsResource{ACMEEnabled: true},
			wantErr: errACMEEmailRequired,
		},
		{
			name:    "acme enabled, whitespace-only email rejected",
			req:     ingressSettingsResource{ACMEEnabled: true, ACMEEmail: "   "},
			wantErr: errACMEEmailRequired,
		},
		{
			name:    "acme enabled, malformed email rejected",
			req:     ingressSettingsResource{ACMEEnabled: true, ACMEEmail: "not-an-email"},
			wantErr: errACMEEmailInvalid,
		},
		{
			name:    "acme enabled, bare @ rejected",
			req:     ingressSettingsResource{ACMEEnabled: true, ACMEEmail: "@"},
			wantErr: errACMEEmailInvalid,
		},
		{
			// ParseAddress alone accepts this; see
			// validateIngressSettingsRequest's own doc comment for why
			// it must still be rejected.
			name:    "acme enabled, display-name email rejected (breaks Caddy's mailto: contact URI)",
			req:     ingressSettingsResource{ACMEEnabled: true, ACMEEmail: "Ops <ops@example.com>"},
			wantErr: errACMEEmailInvalid,
		},
		{
			name: "acme enabled, valid email accepted",
			req:  ingressSettingsResource{ACMEEnabled: true, ACMEEmail: "ops@example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIngressSettingsRequest(tt.req)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("validateIngressSettingsRequest() error = %v, want nil", err)
				}
				return
			}
			if err != tt.wantErr {
				t.Fatalf("validateIngressSettingsRequest() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleGetIngressSettings_Default(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got ingressSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := ingressSettingsResource{FallbackDomainsEnabled: true}
	if got != want {
		t.Errorf("GET /settings/ingress = %+v, want the seeded default %+v", got, want)
	}
}

func TestHandleUpdateIngressSettings_RejectsEnabledWithoutEmail(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{"acme_enabled":true}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}

	// The row must be untouched by a rejected request: GET afterwards
	// still reports the seeded default, not a half-applied update.
	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	var got ingressSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ACMEEnabled {
		t.Errorf("ACMEEnabled = true after a rejected update, want the row left unchanged")
	}
}

func TestHandleUpdateIngressSettings_AcceptsValidConfig(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"primary_domain": "dashboard.example.com",
		"acme_enabled": true,
		"acme_email": "ops@example.com",
		"acme_directory_url": "https://acme-staging-v02.api.letsencrypt.org/directory"
	}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	want := ingressSettingsResource{
		PrimaryDomain:    "dashboard.example.com",
		ACMEEnabled:      true,
		ACMEEmail:        "ops@example.com",
		ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory",

		FallbackDomainsEnabled: true,
	}
	var got ingressSettingsResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != want {
		t.Errorf("PUT response = %+v, want %+v", got, want)
	}

	// GET afterwards reflects the same persisted state, proving the
	// update actually reached the store, not just the response echo.
	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	var reread ingressSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &reread); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if reread != want {
		t.Errorf("GET after PUT = %+v, want %+v", reread, want)
	}
}

// TestHandleUpdateIngressSettings_HSTSRoundTrip covers hsts_enabled's
// default (false, same seeded-default shape every other ingress setting
// field has) and that a PUT persists it for a later GET to see, matching
// TestHandleUpdateIngressSettings_AcceptsValidConfig's own round-trip
// shape for the pre-existing fields.
func TestHandleUpdateIngressSettings_HSTSRoundTrip(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	getRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	var initial ingressSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if initial.HSTSEnabled {
		t.Errorf("HSTSEnabled = true on a fresh control plane, want the seeded default false")
	}

	putRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(putRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", `{"hsts_enabled":true}`))
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body = %s", putRec.Code, http.StatusOK, putRec.Body.String())
	}
	var putGot ingressSettingsResource
	if err := json.Unmarshal(putRec.Body.Bytes(), &putGot); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !putGot.HSTSEnabled {
		t.Errorf("PUT response HSTSEnabled = false, want true")
	}

	rereadRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rereadRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	var reread ingressSettingsResource
	if err := json.Unmarshal(rereadRec.Body.Bytes(), &reread); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !reread.HSTSEnabled {
		t.Errorf("GET after PUT HSTSEnabled = false, want the persisted true")
	}
}

// TestHSTSDBOverride_TogglesHeader covers the actual point of
// hsts_enabled: enabling it from the dashboard, with APP_ENABLE_HSTS
// left unset, makes Strict-Transport-Security appear on the very next
// request, no restart needed (Router.hstsDBEnabled's own doc comment).
func TestHSTSDBOverride_TogglesHeader(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	before := httptest.NewRecorder()
	rt.Handler().ServeHTTP(before, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	if got := before.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q before enabling hsts_enabled, want unset", got)
	}

	putRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(putRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", `{"hsts_enabled":true}`))
	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want %d, body = %s", putRec.Code, http.StatusOK, putRec.Body.String())
	}

	after := httptest.NewRecorder()
	rt.Handler().ServeHTTP(after, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	got := after.Header().Get("Strict-Transport-Security")
	if got == "" {
		t.Fatal("Strict-Transport-Security unset after enabling hsts_enabled, want it set without a restart")
	}
	if !strings.Contains(got, "includeSubDomains") {
		t.Errorf("Strict-Transport-Security = %q, want includeSubDomains", got)
	}

	disableRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(disableRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", `{"hsts_enabled":false}`))
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want %d, body = %s", disableRec.Code, http.StatusOK, disableRec.Body.String())
	}

	afterDisable := httptest.NewRecorder()
	rt.Handler().ServeHTTP(afterDisable, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
	if got := afterDisable.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q after disabling hsts_enabled, want unset", got)
	}
}

// TestHandleUpdateIngressSettings_PrimaryDomainConflict covers the
// collision this endpoint didn't check before: setting PrimaryDomain to
// a domain an app already owns must be rejected with 409, naming the
// conflicting app, the same shape apps domains add's own
// store.ErrDomainTaken already gives a per-app domain conflict.
func TestHandleUpdateIngressSettings_PrimaryDomainConflict(t *testing.T) {
	tests := []struct {
		name          string
		seedDomain    string
		primaryDomain string
		wantConflict  bool
	}{
		{
			name:          "exact match conflicts",
			seedDomain:    "app.example.com",
			primaryDomain: "app.example.com",
			wantConflict:  true,
		},
		{
			name:          "case-insensitive match conflicts",
			seedDomain:    "App.Example.com",
			primaryDomain: "app.example.com",
			wantConflict:  true,
		},
		{
			name:          "different domain does not conflict",
			seedDomain:    "app.example.com",
			primaryDomain: "dashboard.example.com",
			wantConflict:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)

			if err := db.SaveDesiredService(context.Background(), store.DesiredService{
				Name: "web", Image: "img:v1", Port: 80,
				Domains: []string{tt.seedDomain},
			}); err != nil {
				t.Fatalf("seed service: %v", err)
			}

			body := `{"primary_domain":"` + tt.primaryDomain + `"}`
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", body))

			if tt.wantConflict {
				if rec.Code != http.StatusConflict {
					t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusConflict, rec.Body.String())
				}
				if !strings.Contains(rec.Body.String(), "web") {
					t.Errorf("conflict body = %q, want it to name the owning app %q", rec.Body.String(), "web")
				}

				// The row must be untouched by a rejected request.
				getRec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(getRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress", ""))
				var got ingressSettingsResource
				if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				if got.PrimaryDomain != "" {
					t.Errorf("PrimaryDomain = %q after a rejected update, want left unchanged", got.PrimaryDomain)
				}
				return
			}

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateIngressSettings_CanDisableAndClear(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	enableBody := `{"acme_enabled":true,"acme_email":"ops@example.com","primary_domain":"dashboard.example.com"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", enableBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable status = %d, body = %s", rec.Code, rec.Body.String())
	}

	disableRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(disableRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", `{}`))
	if disableRec.Code != http.StatusOK {
		t.Fatalf("disable status = %d, body = %s", disableRec.Code, disableRec.Body.String())
	}

	var got ingressSettingsResource
	if err := json.Unmarshal(disableRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got != (ingressSettingsResource{FallbackDomainsEnabled: true}) {
		t.Errorf("after disable = %+v, want zero value", got)
	}
}

// TestHandleUpdateIngressSettings_PlainWriteToken_Forbidden proves PUT
// /settings/ingress really sits behind AbilityRoot, not just AbilityWrite
// or AbilityRead: an ACME on/off toggle changes what every currently-
// routed host's certificate automation does on the control plane's very
// next ingress reconcile, the same "real infrastructure, not an ordinary
// per-app write" blast radius TestHandleSystemPrune_PlainWriteToken_Forbidden
// and TestHandleSetAppNode_PlainWriteToken_Forbidden already draw this
// exact line for. A token scoped only to AbilityWrite must be rejected
// with 403, not silently accepted because the route only checked for
// "any authenticated caller."
func TestHandleUpdateIngressSettings_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	body := `{"acme_enabled":true,"acme_email":"ops@example.com"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ingress", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the ACME toggle", rec.Code, http.StatusForbidden)
	}

	// The row must be untouched by a rejected request.
	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/settings/ingress", nil)
	getReq.Header.Set("Authorization", "Bearer "+plaintext)
	rt.Handler().ServeHTTP(getRec, getReq)
	var got ingressSettingsResource
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ACMEEnabled {
		t.Errorf("ACMEEnabled = true after a forbidden update, want the row left unchanged")
	}
}

func TestHandleUpdateIngressSettings_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/ingress", nil)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

// TestHandleCheckIngressDomain covers handleCheckIngressDomain's own
// cases on top of runDomainCheck (already covered exhaustively by
// domain_check_test.go's TestHandleCheckDomain_* suite against the
// per-app route): no primary domain configured yet, a configured domain
// that resolves, one that doesn't, and inferred vs explicit
// APP_PUBLIC_HOST.
func TestHandleCheckIngressDomain(t *testing.T) {
	tests := []struct {
		name           string
		primaryDomain  string
		publicHost     string
		requestHost    string
		lookup         func(t *testing.T) lookupHostFunc
		wantConfigured bool
		wantStatus     string
		wantInferred   bool
	}{
		{
			name:           "no primary domain configured",
			primaryDomain:  "",
			publicHost:     "203.0.113.10",
			wantConfigured: false,
		},
		{
			name:          "configured domain resolves",
			primaryDomain: "dashboard.example.com",
			publicHost:    "203.0.113.10",
			lookup: func(t *testing.T) lookupHostFunc {
				t.Helper()
				return func(_ context.Context, host string) ([]string, error) {
					if host != "dashboard.example.com" {
						t.Errorf("lookupHost called with %q, want %q", host, "dashboard.example.com")
					}
					return []string{"203.0.113.10"}, nil
				}
			},
			wantConfigured: true,
			wantStatus:     domainCheckStatusConnected,
			wantInferred:   false,
		},
		{
			name:          "configured domain does not resolve",
			primaryDomain: "dashboard.example.com",
			publicHost:    "203.0.113.10",
			lookup: func(t *testing.T) lookupHostFunc {
				t.Helper()
				return func(_ context.Context, _ string) ([]string, error) {
					return nil, errors.New("no such host")
				}
			},
			wantConfigured: true,
			wantStatus:     domainCheckStatusNotResolving,
			wantInferred:   false,
		},
		{
			name:          "host inferred when APP_PUBLIC_HOST unset",
			primaryDomain: "dashboard.example.com",
			publicHost:    "",
			requestHost:   "203.0.113.10:8443",
			lookup: func(t *testing.T) lookupHostFunc {
				t.Helper()
				return func(_ context.Context, _ string) ([]string, error) {
					return []string{"203.0.113.10"}, nil
				}
			},
			wantConfigured: true,
			wantStatus:     domainCheckStatusConnected,
			wantInferred:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(_ context.Context, _ string) ([]string, error) { return nil, errors.New("no such host") }
			if tt.lookup != nil {
				lookup = tt.lookup(t)
			}
			rt, db := newTestRouterWithLookupHost(t, tt.publicHost, lookup)
			cookie := loginTestSession(t, rt, db)

			if tt.primaryDomain != "" {
				putBody := `{"primary_domain":"` + tt.primaryDomain + `"}`
				putRec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(putRec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/ingress", putBody))
				if putRec.Code != http.StatusOK {
					t.Fatalf("seed primary_domain: status = %d, body = %s", putRec.Code, putRec.Body.String())
				}
			}

			req := authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/ingress/check", "")
			if tt.requestHost != "" {
				req.Host = tt.requestHost
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			var got ingressDomainCheckResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got.Configured != tt.wantConfigured {
				t.Errorf("configured = %v, want %v", got.Configured, tt.wantConfigured)
			}
			if !tt.wantConfigured {
				return
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.HostInferred != tt.wantInferred {
				t.Errorf("host_inferred = %v, want %v", got.HostInferred, tt.wantInferred)
			}
			if got.Domain != tt.primaryDomain {
				t.Errorf("domain = %q, want %q", got.Domain, tt.primaryDomain)
			}
		})
	}
}

func TestHandleCheckIngressDomain_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/ingress/check", nil)
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

// requestWithWriteScopedToken seeds a plain AbilityWrite-only API token
// and issues req with it as the bearer credential, for tests proving a
// route needs more than AbilityWrite.
func requestWithWriteScopedToken(t *testing.T, db *store.DB, req *http.Request) *http.Request {
	t.Helper()
	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_write_check", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+plaintext)
	return req
}

// TestHandleCheckIngressDomain_PlainWriteToken_Forbidden proves the
// route really sits behind AbilityRoot, matching PUT
// /api/v1/settings/ingress's own precedent: see that route's forbidden
// test for why AbilityWrite alone must not be enough here either.
func TestHandleCheckIngressDomain_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithLookupHost(t, "203.0.113.10", func(_ context.Context, _ string) ([]string, error) {
		return []string{"203.0.113.10"}, nil
	})

	req := requestWithWriteScopedToken(t, db, httptest.NewRequest(http.MethodGet, "/api/v1/settings/ingress/check", nil))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach the platform DNS check", rec.Code, http.StatusForbidden)
	}
}

func TestHandleListDomains(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if err := db.SaveDesiredService(context.Background(), store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"web.example.com", "www.example.com"},
	}); err != nil {
		t.Fatalf("seed service: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/domains", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []domainResource{
		{Domain: "web.example.com", ServiceName: "web", ReachableReason: reachableReasonStopped},
		{Domain: "www.example.com", ServiceName: "web", ReachableReason: reachableReasonStopped},
	}
	if len(got) != len(want) {
		t.Fatalf("GET /domains = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("GET /domains[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestHandleListDomains_StatusFlags proves the four read-only status
// flags (waf_enabled, has_redirect, maintenance_enabled, has_basic_auth)
// round-trip correctly per domain, each sourced from its own table via
// a bulk List* read rather than a per-domain lookup, and that a domain
// with none of them configured reports all four false.
func TestHandleListDomains_StatusFlags(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"configured.example.com", "plain.example.com"},
	}); err != nil {
		t.Fatalf("seed service: %v", err)
	}

	if err := db.SetDomainWAF(ctx, "configured.example.com", true, store.DomainWAFModeBlock, 0, 0); err != nil {
		t.Fatalf("set domain waf: %v", err)
	}
	if err := db.SetDomainRedirect(ctx, "configured.example.com", "https://target.example.com", store.DomainRedirectPermanent); err != nil {
		t.Fatalf("set domain redirect: %v", err)
	}
	if err := db.SetDomainMaintenance(ctx, "configured.example.com"); err != nil {
		t.Fatalf("set domain maintenance: %v", err)
	}
	if err := db.SetDomainBasicAuth(ctx, "configured.example.com", "admin"); err != nil {
		t.Fatalf("set domain basic auth: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/domains", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]domainResource{
		"configured.example.com": {
			Domain: "configured.example.com", ServiceName: "web",
			WAFEnabled: true, HasRedirect: true, MaintenanceEnabled: true, HasBasicAuth: true,
			ReachableReason: reachableReasonStopped,
		},
		"plain.example.com": {
			Domain: "plain.example.com", ServiceName: "web", ReachableReason: reachableReasonStopped,
		},
	}
	if len(got) != len(want) {
		t.Fatalf("GET /domains = %+v, want %d rows", got, len(want))
	}
	for _, row := range got {
		if row != want[row.Domain] {
			t.Errorf("GET /domains[%q] = %+v, want %+v", row.Domain, row, want[row.Domain])
		}
	}
}

func TestHandleListDomains_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/domains", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []domainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("GET /domains = %+v, want empty for a control plane with no domains claimed", got)
	}
}

func TestFirstAdminAddress(t *testing.T) {
	for name, tc := range map[string]struct{ username, want string }{
		"address":        {"owner@example.com", "owner@example.com"},
		"plain username": {"admin", ""},
	} {
		t.Run(name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			if err := BootstrapAdmin(context.Background(), db, tc.username, testAdminPassword); err != nil {
				t.Fatalf("bootstrap: %v", err)
			}
			if got := rt.firstAdminAddress(context.Background()); got != tc.want {
				t.Errorf("firstAdminAddress = %q, want %q", got, tc.want)
			}
		})
	}
}
