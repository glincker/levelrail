package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/oidc"
)

type fakeOIDCJWKS struct {
	jwks oidc.JWKS
	err  error
}

func (f fakeOIDCJWKS) JWKS(context.Context) (oidc.JWKS, error) { return f.jwks, f.err }

type fakeOIDCRotator struct {
	res            oidc.RotationResult
	err            error
	gotRetireAfter time.Duration
}

func (f *fakeOIDCRotator) RotateKey(_ context.Context, retireAfter time.Duration) (oidc.RotationResult, error) {
	f.gotRetireAfter = retireAfter
	return f.res, f.err
}

func TestHandleOIDCJWKS_NotConfigured(t *testing.T) {
	rt, _ := newTestRouter(t)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleOIDCJWKS_Unauthenticated_ServesTheDocument(t *testing.T) {
	rt, _ := newTestRouter(t)
	want := oidc.JWKS{Keys: []oidc.JWK{{Kty: "EC", Crv: "P-256", X: "x", Y: "y", Kid: "abc", Use: "sig", Alg: "ES256"}}}
	rt.SetOIDCManager(fakeOIDCJWKS{jwks: want}, "https://cp.example.com", 0)

	rec := httptest.NewRecorder()
	// No auth cookie/token at all: this must work unauthenticated.
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got oidc.JWKS
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Keys) != 1 || got.Keys[0].Kid != "abc" {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestHandleOIDCJWKS_BuildError(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.SetOIDCManager(fakeOIDCJWKS{err: errors.New("key store unavailable")}, "https://cp.example.com", 0)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleGetPipelineOIDCInfo(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		rt, db := newTestRouter(t)
		cookie := loginTestSession(t, rt, db)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/pipelines/oidc", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		var got oidcInfoResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Configured {
			t.Errorf("got %+v, want not configured", got)
		}
	})

	t.Run("configured", func(t *testing.T) {
		rt, db := newTestRouter(t)
		rt.SetOIDCManager(fakeOIDCJWKS{jwks: oidc.JWKS{}}, "https://cp.example.com", 0)
		cookie := loginTestSession(t, rt, db)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/pipelines/oidc", ""))
		var got oidcInfoResource
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if !got.Configured || got.IssuerURL != "https://cp.example.com" || got.JWKSURL != "https://cp.example.com/.well-known/jwks.json" {
			t.Errorf("got %+v", got)
		}
	})
}

func TestHandleRotatePipelineOIDCKey_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/pipelines/oidc/rotate-key", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleRotatePipelineOIDCKey_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.SetOIDCRotator(&fakeOIDCRotator{})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/pipelines/oidc/rotate-key", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleRotatePipelineOIDCKey_Success(t *testing.T) {
	rt, db := newTestRouter(t)
	retireAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	fake := &fakeOIDCRotator{res: oidc.RotationResult{OldKID: "old1", NewKID: "new1", RetireAt: retireAt, RetiringCount: 1}}
	rt.SetOIDCRotator(fake)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/pipelines/oidc/rotate-key", `{"retire_after":"2h"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fake.gotRetireAfter != 2*time.Hour {
		t.Errorf("gotRetireAfter = %v, want 2h", fake.gotRetireAfter)
	}
	var got rotatePipelineOIDCKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OldKID != "old1" || got.NewKID != "new1" || got.RetiringCount != 1 || !got.RetireAt.Equal(retireAt) {
		t.Errorf("got %+v", got)
	}
}

func TestHandleRotatePipelineOIDCKey_InvalidRetireAfter(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetOIDCRotator(&fakeOIDCRotator{})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/pipelines/oidc/rotate-key", `{"retire_after":"not-a-duration"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleRotatePipelineOIDCKey_RotateError(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetOIDCRotator(&fakeOIDCRotator{err: errors.New("key store unavailable")})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/pipelines/oidc/rotate-key", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

func TestHandleGetPipelineOIDCInfo_RotationSupported(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.SetOIDCManager(fakeOIDCJWKS{}, "https://cp.example.com", 0)
	rt.SetOIDCRotator(&fakeOIDCRotator{})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/pipelines/oidc", ""))
	var got oidcInfoResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.RotationSupported {
		t.Error("expected RotationSupported = true")
	}
}

func TestHandleOIDCJWKS_RateLimited(t *testing.T) {
	rt, _ := newTestRouter(t)
	rt.SetOIDCManager(fakeOIDCJWKS{jwks: oidc.JWKS{Keys: []oidc.JWK{{Kid: "abc"}}}}, "https://cp.example.com", 1)

	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
		r.RemoteAddr = "203.0.113.5:1234"
		return r
	}
	rec1 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec1, req())
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", rec1.Code, http.StatusOK)
	}
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, req())
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", rec2.Code, http.StatusTooManyRequests)
	}
}
