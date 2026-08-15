package api

import (
	"context"
	"encoding/json"
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
	want := ingressSettingsResource{}
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
	if got != (ingressSettingsResource{}) {
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
		{Domain: "web.example.com", ServiceName: "web"},
		{Domain: "www.example.com", ServiceName: "web"},
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
