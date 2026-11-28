package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

func TestRegistryTokenOwnLimiterAndLiveTTL(t *testing.T) {
	rt := &Router{codeLogin: &codeLoginState{redeemByIP: newAPIRateLimiter(1)}}
	l := rt.registryTokenLimiter()
	if l != rt.registryTokenLimiter() {
		t.Fatal("limiter must be reused")
	}
	if l == rt.codeLogin.redeemByIP {
		t.Fatal("registry token must not share the login bucket")
	}
	def := runtimesettings.ClampLimit(runtimesettings.KeyRegistryTokenRateLimit, 0)
	if def != 120 {
		t.Fatalf("default %d", def)
	}
	// One caller exhausts its own bucket without touching another job's.
	for i := 0; i < def; i++ {
		if ok, _ := l.allow("1.2.3.4|nonceA"); !ok {
			t.Fatalf("denied early at %d", i)
		}
	}
	if ok, _ := l.allow("1.2.3.4|nonceA"); ok {
		t.Error("over budget must be denied")
	}
	if ok, _ := l.allow("1.2.3.4|nonceB"); !ok {
		t.Error("another build must have its own bucket")
	}
}

func TestRegistryTokenHandlerFailsClosed(t *testing.T) {
	rt := &Router{}
	w := httptest.NewRecorder()
	rt.handleRegistryToken(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/token", nil))
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("disabled: %d", w.Code)
	}
	rt.registryToken = &build.RegistryTokenService{Service: "svc", BearerTTL: func() time.Duration { return time.Minute }}
	w = httptest.NewRecorder()
	rt.handleRegistryToken(w, httptest.NewRequest(http.MethodGet, "/api/v1/registry/token?service=svc", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials: %d", w.Code)
	}
}
