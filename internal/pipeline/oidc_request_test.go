package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOIDCTokenRequestHandler_MintsForAllowedAudience(t *testing.T) {
	var gotReq OIDCTokenRequest
	e := &Engine{
		cfg: Config{OIDCIssuer: func(_ context.Context, req OIDCTokenRequest) (string, error) {
			gotReq = req
			return "minted-token", nil
		}},
		oidcRequests: newOIDCRequestRegistry(),
	}
	e.oidcRequests.register("tok123", []string{"aws", "gcp"}, OIDCTokenRequest{Subject: "repo:web:ref:main:job:deploy", Repo: "web"})

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=gcp", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got oidcTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token != "minted-token" {
		t.Errorf("token = %q", got.Token)
	}
	if gotReq.Audience != "gcp" || gotReq.Repo != "web" {
		t.Errorf("OIDCTokenRequest = %+v", gotReq)
	}
}

func TestOIDCTokenRequestHandler_RejectsDisallowedAudience(t *testing.T) {
	e := &Engine{
		cfg:          Config{OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) { return "tok", nil }},
		oidcRequests: newOIDCRequestRegistry(),
	}
	e.oidcRequests.register("tok123", []string{"aws"}, OIDCTokenRequest{})

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=gcp", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestOIDCTokenRequestHandler_RejectsUnknownToken(t *testing.T) {
	e := &Engine{
		cfg:          Config{OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) { return "tok", nil }},
		oidcRequests: newOIDCRequestRegistry(),
	}

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=aws", nil)
	req.Header.Set("Authorization", "Bearer does-not-exist")
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOIDCTokenRequestHandler_RejectsMissingBearer(t *testing.T) {
	e := &Engine{
		cfg:          Config{OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) { return "tok", nil }},
		oidcRequests: newOIDCRequestRegistry(),
	}

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=aws", nil)
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOIDCTokenRequestHandler_NotConfigured(t *testing.T) {
	e := &Engine{oidcRequests: newOIDCRequestRegistry()}

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=aws", nil)
	req.Header.Set("Authorization", "Bearer x")
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestOIDCTokenRequestHandler_IssuerErrorPropagates(t *testing.T) {
	e := &Engine{
		cfg: Config{OIDCIssuer: func(context.Context, OIDCTokenRequest) (string, error) {
			return "", errors.New("signing key unavailable")
		}},
		oidcRequests: newOIDCRequestRegistry(),
	}
	e.oidcRequests.register("tok123", []string{"aws"}, OIDCTokenRequest{})

	req := httptest.NewRequest(http.MethodGet, "/oidc/token?audience=aws", nil)
	req.Header.Set("Authorization", "Bearer tok123")
	rec := httptest.NewRecorder()
	e.OIDCTokenRequestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestOIDCRequestRegistry_UnregisterRemovesEntry(t *testing.T) {
	r := newOIDCRequestRegistry()
	r.register("tok", []string{"aws"}, OIDCTokenRequest{})
	if _, ok := r.lookup("tok"); !ok {
		t.Fatal("expected entry to be registered")
	}
	r.unregister("tok")
	if _, ok := r.lookup("tok"); ok {
		t.Error("expected entry to be gone after unregister")
	}
}

func TestOidcAllowedAudiences(t *testing.T) {
	tests := []struct {
		name string
		jd   *Job
		want []string
	}{
		{"nil oidc", &Job{}, nil},
		{"audience only", &Job{OIDC: &OIDCRequest{Audience: "aws"}}, []string{"aws"}},
		{"audiences only", &Job{OIDC: &OIDCRequest{Audiences: []string{"aws", "gcp"}}}, []string{"aws", "gcp"}},
		{"both, deduped", &Job{OIDC: &OIDCRequest{Audience: "aws", Audiences: []string{"aws", "gcp"}}}, []string{"aws", "gcp"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oidcAllowedAudiences(tt.jd)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
